// factServer.go
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
	"time"
)

const (
	listenPort = 20000
	protoUDP   = 0x01
	startPort  = 10000
	endPort    = 10018
)

var verbose = true

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

func compress(data []byte) ([]byte, error) {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	w, err := zw.Create("data")
	if err != nil {
		return nil, err
	}
	w.Write(data)
	zw.Close()
	return buf.Bytes(), nil
}

func protoStr(p byte) string {
	switch p {
	case protoUDP:
		return "UDP"
	default:
		return "UNKNOWN"
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
	client := conn.RemoteAddr().String()
	log.Printf("[CONNECT] From proxy %s", client)

	buf := make([]byte, 65536)
	n, err := conn.Read(buf)
	if err != nil {
		log.Printf("[ERR] Read from proxy: %v", err)
		return
	}
	if n < 3 {
		log.Printf("[ERR] Invalid packet from proxy")
		return
	}

	proto := buf[0]
	port := int(binary.BigEndian.Uint16(buf[1:3]))
	if port < startPort || port > endPort {
		log.Printf("[ERR] Port %d out of allowed range", port)
		return
	}

	data, err := decompress(buf[3:n])
	if err != nil {
		log.Printf("[ERR] Decompression failed: %v", err)
		return
	}

	log.Printf("[INCOMING] %s %d ← proxy (%d bytes)", protoStr(proto), port, len(data))
	log.Printf("[HEX OUT] UDP → m45sci.xyz:%d:\n%s", port, hex.Dump(data))

	if proto == protoUDP {
		destAddrStr := fmt.Sprintf("m45sci.xyz:%d", port)
		udpAddr, err := net.ResolveUDPAddr("udp", destAddrStr)
		if err != nil {
			log.Printf("[ERR] Failed to resolve %s: %v", destAddrStr, err)
			return
		}
		udpConn, err := net.DialUDP("udp", nil, udpAddr)
		if err != nil {
			log.Printf("[ERR] UDP dial failed: %v", err)
			return
		}
		defer udpConn.Close()

		udpConn.Write(data)

		// Wait for response
		udpConn.SetReadDeadline(time.Now().Add(2 * time.Second))
		reply := make([]byte, 4096)
		n, _, err := udpConn.ReadFrom(reply)
		if err != nil {
			log.Printf("[WARN] No reply from m45sci.xyz:%d: %v", port, err)
			return
		}

		log.Printf("[REPLY] ← m45sci.xyz:%d (%d bytes)", port, n)

		respCompressed, err := compress(reply[:n])
		if err != nil {
			log.Printf("[ERR] Reply compression failed: %v", err)
			return
		}

		respPacket := new(bytes.Buffer)
		respPacket.WriteByte(proto)
		binary.Write(respPacket, binary.BigEndian, uint16(port))
		respPacket.Write(respCompressed)

		conn.Write(respPacket.Bytes())
		log.Printf("[OUTGOING] → proxy (%d compressed bytes)", len(respCompressed))
	}

	log.Printf("[DISCONNECT] From proxy %s", client)
}

func main() {
	flag.BoolVar(&verbose, "verbose", true, "Enable verbose logging (on by default)")
	flag.Parse()
	log.SetOutput(os.Stdout)
	log.Printf("[START] factServer on TCP %d", listenPort)

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", listenPort))
	if err != nil {
		log.Fatalf("[FATAL] Failed to listen: %v", err)
	}
	defer ln.Close()

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("[ERR] Accept failed: %v", err)
			continue
		}
		go handleConnection(conn)
	}
}
