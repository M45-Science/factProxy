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
	listenPort = 30000
	protoUDP   = 0x01
	protoTCP   = 0x02
	startPort  = 10000
	endPort    = 10018
	targetHost = "127.0.0.1"
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
	case protoTCP:
		return "TCP"
	}
	return "UNKNOWN"
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
		log.Printf("[ERR] Port %d out of range", port)
		return
	}

	data, err := decompress(buf[3:n])
	if err != nil {
		log.Printf("[ERR] Decompression failed: %v", err)
		return
	}

	log.Printf("[INCOMING] %s %d ← proxy (%d bytes)", protoStr(proto), port, len(data))
	log.Printf("[HEX OUT] %s → %s:%d\n%s", protoStr(proto), targetHost, port, hex.Dump(data))

	var resp []byte
	switch proto {
	case protoUDP:
		dest := fmt.Sprintf("%s:%d", targetHost, port)
		udpAddr, _ := net.ResolveUDPAddr("udp", dest)
		udpConn, _ := net.DialUDP("udp", nil, udpAddr)
		defer udpConn.Close()
		udpConn.Write(data)

		udpConn.SetReadDeadline(time.Now().Add(2 * time.Second))
		reply := make([]byte, 4096)
		n, _, err := udpConn.ReadFrom(reply)
		if err == nil {
			resp = reply[:n]
		}
	case protoTCP:
		dest := fmt.Sprintf("%s:%d", targetHost, port)
		tcpConn, err := net.Dial("tcp", dest)
		if err != nil {
			log.Printf("[ERR] TCP dial to game server failed: %v", err)
			return
		}
		defer tcpConn.Close()
		tcpConn.Write(data)
		reply, _ := io.ReadAll(tcpConn)
		resp = reply
	default:
		log.Printf("[ERR] Unknown protocol: %d", proto)
		return
	}

	respCompressed, err := compress(resp)
	if err != nil {
		log.Printf("[ERR] Compression failed: %v", err)
		return
	}

	packet := new(bytes.Buffer)
	packet.WriteByte(proto)
	binary.Write(packet, binary.BigEndian, uint16(port))
	packet.Write(respCompressed)

	conn.Write(packet.Bytes())
	log.Printf("[OUTGOING] → proxy (%d compressed bytes)", len(respCompressed))
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
