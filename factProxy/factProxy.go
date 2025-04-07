// factProxy.go
package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"io"
	"log"
	"net"
	"os"
)

const (
	startPort  = 20000
	endPort    = 20018
	serverHost = "127.0.0.1"
	serverPort = 20000
	serverAddr = serverHost + ":20000"
	protoUDP   = 0x01
)

var verbose = true

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

func forwardToServer(port int, data []byte, addr string, listener *net.UDPConn) {
	if verbose {
		log.Printf("[HEX IN] UDP %d ← %s:\n%s", port, addr, hex.Dump(data))
	}

	compressed, err := compress(data)
	if err != nil {
		log.Printf("[ERR] Compression failed (%d): %v", port, err)
		return
	}

	packet := new(bytes.Buffer)
	packet.WriteByte(protoUDP)
	binary.Write(packet, binary.BigEndian, uint16(port-10000))
	packet.Write(compressed)

	conn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		log.Printf("[ERR] Failed to connect to server: %v", err)
		return
	}
	defer conn.Close()

	log.Printf("[OUTGOING] → %s (UDP %d) (%d → %d bytes)", serverAddr, port, len(data), len(compressed))
	conn.Write(packet.Bytes())

	// Read response
	reply := make([]byte, 65536)
	n, err := conn.Read(reply)
	if err != nil {
		log.Printf("[ERR] Failed to read reply: %v", err)
		return
	}
	if n < 3 {
		log.Printf("[WARN] Short reply")
		return
	}

	respPort := int(binary.BigEndian.Uint16(reply[1:3])) + 10000 // add offset back
	respCompressed := reply[3:n]
	respData, err := decompress(respCompressed)
	if err != nil {
		log.Printf("[ERR] Failed to decompress reply: %v", err)
		return
	}

	log.Printf("[REPLY] ← %s (UDP %d) (%d → %d bytes)", serverAddr, respPort, len(respCompressed), len(respData))

	// Send response using the original listening socket
	udpAddr, _ := net.ResolveUDPAddr("udp", addr)
	_, err = listener.WriteToUDP(respData, udpAddr)
	if err != nil {
		log.Printf("[ERR] UDP write to %s failed: %v", addr, err)
	} else {
		log.Printf("[SEND] UDP %d → %s (%d bytes)", port, addr, len(respData))
	}
}

func handleUDP(port int) {
	addr := net.UDPAddr{Port: port}
	conn, err := net.ListenUDP("udp", &addr)
	if err != nil {
		log.Fatalf("[FATAL] Failed to bind UDP port %d: %v", port, err)
	}
	log.Printf("[LISTEN] UDP %d", port)
	defer conn.Close()

	buf := make([]byte, 4096)
	for {
		n, clientAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("[ERR] UDP read: %v", err)
			continue
		}
		log.Printf("[RECV] UDP %d ← %s (%d bytes)", port, clientAddr.String(), n)
		go forwardToServer(port, buf[:n], clientAddr.String(), conn)
	}
}

func main() {
	flag.BoolVar(&verbose, "verbose", true, "Enable verbose logging (on by default)")
	flag.Parse()
	log.SetOutput(os.Stdout)
	log.Printf("[START] factProxy targeting %s", serverAddr)

	for port := startPort; port <= endPort; port++ {
		go handleUDP(port)
	}

	select {} // block forever
}
