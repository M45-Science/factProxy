package main

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
)

const (
	tcpListenPort   = 30000       // Where we listen for factProxy TCP connections
	targetDomain    = "127.0.0.1" // Target game server domain or IP
	targetPortStart = 10000       // Base game server UDP port
	proxyPortStart  = 10000       // Base proxy UDP port
	proxyPortEnd    = 10017       // End of proxy UDP port range
	udpPortOffset   = proxyPortStart - targetPortStart
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

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	listenAddr := fmt.Sprintf(":%d", tcpListenPort)
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("[FATAL] Failed to listen on %s: %v", listenAddr, err)
	}
	log.Printf("[START] factServer listening on TCP %s", listenAddr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("[ERR] Accept failed: %v", err)
			continue
		}
		log.Printf("[CONNECT] From proxy %s", conn.RemoteAddr())
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	var tcpWriteMu sync.Mutex
	udpConns := make(map[uint16]*net.UDPConn)

	header := make([]byte, 7)
	for {
		_, err := io.ReadFull(conn, header)
		if err != nil {
			log.Printf("[DISCONNECT] %s (%v)", conn.RemoteAddr(), err)
			break
		}

		frameType := FrameType(header[0])
		port := binary.BigEndian.Uint16(header[1:3])
		length := binary.BigEndian.Uint32(header[3:7])

		if frameType != TypeRequest {
			log.Printf("[WARN] Unknown frame type %d", frameType)
			io.CopyN(io.Discard, conn, int64(length))
			continue
		}

		if port < proxyPortStart || port > proxyPortEnd {
			log.Printf("[ERR] Port %d outside proxy range (%d–%d)", port, proxyPortStart, proxyPortEnd)
			io.CopyN(io.Discard, conn, int64(length))
			continue
		}

		payload := make([]byte, length)
		if _, err := io.ReadFull(conn, payload); err != nil {
			log.Printf("[ERR] Read payload: %v", err)
			break
		}

		data, err := decompressData(payload)
		if err != nil {
			log.Printf("[ERR] Decompression on port %d: %v", port, err)
			continue
		}

		mappedPort := port - udpPortOffset
		targetAddr := fmt.Sprintf("%s:%d", targetDomain, mappedPort)

		udpConn, ok := udpConns[port]
		if !ok {
			raddr, err := net.ResolveUDPAddr("udp", targetAddr)
			if err != nil {
				log.Printf("[ERR] Resolve %s: %v", targetAddr, err)
				continue
			}
			udpConn, err = net.DialUDP("udp", nil, raddr)
			if err != nil {
				log.Printf("[ERR] DialUDP %s: %v", targetAddr, err)
				continue
			}
			udpConns[port] = udpConn

			go func(p uint16, uconn *net.UDPConn) {
				buf := make([]byte, 65535)
				for {
					n, _, err := uconn.ReadFromUDP(buf)
					if err != nil {
						log.Printf("[ERR] UDP recv on port %d: %v", p, err)
						return
					}
					resp := buf[:n]
					comp, err := compressData(resp)
					if err != nil {
						log.Printf("[ERR] Compression resp port %d: %v", p, err)
						continue
					}

					frame := new(bytes.Buffer)
					frame.WriteByte(byte(TypeResponse))
					binary.Write(frame, binary.BigEndian, p)
					binary.Write(frame, binary.BigEndian, uint32(len(comp)))
					frame.Write(comp)

					tcpWriteMu.Lock()
					_, err = conn.Write(frame.Bytes())
					tcpWriteMu.Unlock()
					if err != nil {
						log.Printf("[ERR] TCP send resp port %d: %v", p, err)
						return
					}
					log.Printf("[REPLY] ← m45sci.xyz:%d → proxy (%d bytes)", mappedPort, len(resp))
				}
			}(port, udpConn)
		}

		_, err = udpConn.Write(data)
		if err != nil {
			log.Printf("[ERR] UDP send port %d: %v", port, err)
			continue
		}
		log.Printf("[FORWARD] UDP %d → %s (%d bytes)", port, targetAddr, len(data))
	}

	for _, c := range udpConns {
		c.Close()
	}
	log.Printf("[CLOSE] Cleaned up UDP sockets for %s", conn.RemoteAddr())
}
