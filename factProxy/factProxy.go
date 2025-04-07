// factProxy.go (with persistent TCP connection per port)
package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"
)

const (
	startPort  = 20000
	endPort    = 20018
	serverHost = "127.0.0.1"
	serverPort = 30000
	protoUDP   = 0x01
)

var (
	verbose    = true
	serverAddr = serverHost + ":" + fmt.Sprint(serverPort)
)

func compress(data []byte) ([]byte, error) {
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)
	w, err := zipWriter.Create("data")
	if err != nil {
		return nil, err
	}
	_, err = w.Write(data)
	if err != nil {
		return nil, err
	}
	zipWriter.Close()
	return buf.Bytes(), nil
}

func decompress(data []byte) ([]byte, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	for _, f := range r.File {
		rc, _ := f.Open()
		io.Copy(&out, rc)
		rc.Close()
	}
	return out.Bytes(), nil
}

func startProxy(port int) {
	addr := net.UDPAddr{Port: port}
	udpConn, err := net.ListenUDP("udp", &addr)
	if err != nil {
		log.Fatalf("[FATAL] Failed to bind UDP port %d: %v", port, err)
	}
	log.Printf("[LISTEN] UDP %d", port)
	defer udpConn.Close()

	tcpConn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		log.Fatalf("[FATAL] TCP dial to %s failed: %v", serverAddr, err)
	}
	log.Printf("[CONNECT] TCP to %s for port %d", serverAddr, port)
	defer tcpConn.Close()

	// Response reader goroutine
	go func() {
		replyBuf := make([]byte, 65536)
		for {
			n, err := tcpConn.Read(replyBuf)
			if err != nil {
				log.Printf("[ERR] TCP read failed for port %d: %v", port, err)
				return
			}
			if n < 3 {
				continue
			}
			respCompressed := replyBuf[3:n]
			respData, err := decompress(respCompressed)
			if err != nil {
				log.Printf("[ERR] Decompress reply on port %d: %v", port, err)
				continue
			}
			udpAddr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port}
			udpConn.WriteToUDP(respData, udpAddr)
			log.Printf("[REPLY] TCP → UDP %d (%d bytes)", port, len(respData))
		}
	}()

	buf := make([]byte, 4096)
	for {
		n, clientAddr, err := udpConn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("[ERR] UDP read on port %d: %v", port, err)
			continue
		}
		log.Printf("[RECV] UDP %d ← %s (%d bytes)", port, clientAddr.String(), n)
		if verbose {
			log.Printf("[HEX IN] UDP %d ← %s:\n%s", port, clientAddr.String(), hex.Dump(buf[:n]))
		}

		compressed, err := compress(buf[:n])
		if err != nil {
			log.Printf("[ERR] Compression failed on port %d: %v", port, err)
			continue
		}

		packet := new(bytes.Buffer)
		packet.WriteByte(protoUDP)
		binary.Write(packet, binary.BigEndian, uint16(port-10000))
		packet.Write(compressed)

		_, err = tcpConn.Write(packet.Bytes())
		if err != nil {
			log.Printf("[ERR] TCP write failed for port %d: %v", port, err)
			return
		}
		log.Printf("[FORWARD] UDP %d → TCP (%d → %d bytes)", port, n, len(compressed))
	}
}

func main() {
	flag.BoolVar(&verbose, "verbose", true, "Enable verbose logging (on by default)")
	flag.Parse()
	log.SetOutput(os.Stdout)
	log.Printf("[START] factProxy targeting %s", serverAddr)

	var wg sync.WaitGroup
	for port := startPort; port <= endPort; port++ {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			startProxy(p)
		}(port)
	}
	wg.Wait()
}
