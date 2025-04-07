package main

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

const (
	serverAddress = "127.0.0.1:30000"
	udpPortStart  = 20000
	udpPortEnd    = 20018
)

type FrameType byte

const (
	TypeRequest  FrameType = 0
	TypeResponse FrameType = 1
)

func compressData(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	_, err := w.Write(data)
	w.Close()
	return buf.Bytes(), err
}

func decompressData(data []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

var (
	udpConns      = make(map[uint16]*net.UDPConn)
	clientAddrMap sync.Map
	tcpWriteMu    sync.Mutex
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	for port := udpPortStart; port <= udpPortEnd; port++ {
		addr := net.UDPAddr{Port: port}
		udpConn, err := net.ListenUDP("udp", &addr)
		if err != nil {
			log.Fatalf("Unable to bind UDP port %d: %v", port, err)
		}
		udpConns[uint16(port)] = udpConn
		log.Printf("[LISTEN] UDP %d", port)
	}

	for {
		log.Printf("[CONNECTING] Attempting TCP to %s", serverAddress)
		conn, err := net.Dial("tcp", serverAddress)
		if err != nil {
			log.Printf("[RETRY] TCP connect failed: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}
		log.Printf("[CONNECTED] TCP to %s", serverAddress)

		runProxy(conn)

		log.Printf("[DISCONNECTED] TCP connection lost, reconnecting...")
		time.Sleep(2 * time.Second)
	}
}

func runProxy(conn net.Conn) {
	defer conn.Close()

	go readFromTCP(conn)

	for port, udpConn := range udpConns {
		go handleUDP(uint16(port), udpConn, conn)
	}

	select {}
}

func handleUDP(port uint16, udpConn *net.UDPConn, conn net.Conn) {
	buf := make([]byte, 65535)
	for {
		n, clientAddr, err := udpConn.ReadFrom(buf)
		if err != nil {
			log.Printf("[ERR] UDP read on port %d: %v", port, err)
			return
		}
		clientAddrMap.Store(port, clientAddr)
		log.Printf("[RECV] UDP %d ← %s (%d bytes)", port, clientAddr.String(), n)

		compData, err := compressData(buf[:n])
		if err != nil {
			log.Printf("[ERR] Compression on port %d: %v", port, err)
			continue
		}

		frame := new(bytes.Buffer)
		frame.WriteByte(byte(TypeRequest))
		binary.Write(frame, binary.BigEndian, port)
		binary.Write(frame, binary.BigEndian, uint32(len(compData)))
		frame.Write(compData)

		tcpWriteMu.Lock()
		_, err = conn.Write(frame.Bytes())
		tcpWriteMu.Unlock()
		if err != nil {
			log.Printf("[ERR] TCP write failed on port %d: %v", port, err)
			return
		}
		log.Printf("[FORWARD] UDP %d → TCP (%d → %d bytes)", port, n, len(compData))
	}
}

func readFromTCP(conn net.Conn) {
	header := make([]byte, 7)
	for {
		_, err := io.ReadFull(conn, header)
		if err != nil {
			log.Printf("[ERR] TCP read failed: %v", err)
			return
		}
		frameType := FrameType(header[0])
		port := binary.BigEndian.Uint16(header[1:3])
		length := binary.BigEndian.Uint32(header[3:7])

		if frameType != TypeResponse {
			log.Printf("[WARN] Unknown frame type: %d", frameType)
			io.CopyN(io.Discard, conn, int64(length))
			continue
		}

		payload := make([]byte, length)
		if _, err := io.ReadFull(conn, payload); err != nil {
			log.Printf("[ERR] Reading payload: %v", err)
			return
		}

		data, err := decompressData(payload)
		if err != nil {
			log.Printf("[ERR] Decompression failed on port %d: %v", port, err)
			continue
		}

		addrVal, ok := clientAddrMap.Load(port)
		if !ok {
			log.Printf("[WARN] No known client for port %d", port)
			continue
		}

		udpConn := udpConns[port]
		if udpConn == nil {
			log.Printf("[ERR] No UDPConn for port %d", port)
			continue
		}

		_, err = udpConn.WriteTo(data, addrVal.(net.Addr))
		if err != nil {
			log.Printf("[ERR] Sending UDP reply: %v", err)
		} else {
			log.Printf("[SEND] UDP %d → %s (%d bytes)", port, addrVal.(net.Addr).String(), len(data))
		}
	}
}
