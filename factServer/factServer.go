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
	"time"
)

const (
	tcpListenPort   = 30000
	targetDomain    = "m45sci.xyz"
	targetPortStart = 10000
	proxyPortStart  = 20000
	proxyPortEnd    = 20018
	udpPortOffset   = proxyPortStart - targetPortStart
	tickRate        = time.Second / 30
	maxPayloadBytes = 1 << 20 // 1MB
)

type FrameType byte

const (
	TypeRequest      FrameType = 0
	TypeResponse     FrameType = 1
	TypeBatchRequest FrameType = 2
	TypeBatchReply   FrameType = 3
)

type framedMessage struct {
	port uint16
	data []byte
}

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

	addr := fmt.Sprintf(":%d", tcpListenPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("[FATAL] Listen %s: %v", addr, err)
	}
	log.Printf("[START] factServer listening on %s", addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("[ERR] Accept: %v", err)
			continue
		}
		log.Printf("[CONNECT] From proxy %s", conn.RemoteAddr())
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	var tcpWriteMu sync.Mutex
	var responseQueue []framedMessage
	var respMu sync.Mutex

	udpConns := make(map[uint16]*net.UDPConn)

	// Response batch writer
	go func() {
		ticker := time.NewTicker(tickRate)
		defer ticker.Stop()
		for range ticker.C {
			respMu.Lock()
			if len(responseQueue) == 0 {
				respMu.Unlock()
				continue
			}

			var batch bytes.Buffer
			for _, msg := range responseQueue {
				batch.WriteByte(byte(TypeResponse))
				binary.Write(&batch, binary.BigEndian, msg.port)
				binary.Write(&batch, binary.BigEndian, uint32(len(msg.data)))
				batch.Write(msg.data)
			}
			responseQueue = responseQueue[:0]
			respMu.Unlock()

			compressed, err := compressData(batch.Bytes())
			if err != nil {
				log.Printf("[ERR] Compress response batch: %v", err)
				continue
			}

			frame := new(bytes.Buffer)
			frame.WriteByte(byte(TypeBatchReply))
			binary.Write(frame, binary.BigEndian, uint32(len(compressed)))
			frame.Write(compressed)

			tcpWriteMu.Lock()
			_, err = conn.Write(frame.Bytes())
			tcpWriteMu.Unlock()
			if err != nil {
				log.Printf("[ERR] TCP write response batch: %v", err)
				return
			}
			log.Printf("[SEND] Batch reply (%d bytes compressed)", len(compressed))
		}
	}()

	// TCP reader loop
	for {
		header := make([]byte, 5)
		if _, err := io.ReadFull(conn, header); err != nil {
			log.Printf("[DISCONNECT] %s (%v)", conn.RemoteAddr(), err)
			break
		}
		frameType := FrameType(header[0])
		length := binary.BigEndian.Uint32(header[1:5])
		body := make([]byte, length)
		if _, err := io.ReadFull(conn, body); err != nil {
			log.Printf("[ERR] Read TCP body: %v", err)
			break
		}

		if frameType != TypeBatchRequest {
			log.Printf("[WARN] Unexpected frame type: %d", frameType)
			continue
		}

		unzipped, err := decompressData(body)
		if err != nil {
			log.Printf("[ERR] Decompress batch: %v", err)
			continue
		}

		buf := bytes.NewBuffer(unzipped)
		for buf.Len() >= 7 {
			hdr := buf.Next(7)
			ptype := FrameType(hdr[0])
			port := binary.BigEndian.Uint16(hdr[1:3])
			size := binary.BigEndian.Uint32(hdr[3:7])
			if buf.Len() < int(size) {
				log.Printf("[WARN] Incomplete frame in batch")
				break
			}
			payload := buf.Next(int(size))

			if ptype != TypeRequest {
				log.Printf("[WARN] Unexpected inner frame type: %d", ptype)
				continue
			}
			if port < proxyPortStart || port > proxyPortEnd {
				log.Printf("[ERR] Port %d out of proxy range", port)
				continue
			}

			mappedPort := port - udpPortOffset
			target := fmt.Sprintf("%s:%d", targetDomain, mappedPort)

			udpConn, ok := udpConns[port]
			if !ok {
				raddr, err := net.ResolveUDPAddr("udp", target)
				if err != nil {
					log.Printf("[ERR] Resolve %s: %v", target, err)
					continue
				}
				udpConn, err = net.DialUDP("udp", nil, raddr)
				if err != nil {
					log.Printf("[ERR] DialUDP %s: %v", target, err)
					continue
				}
				udpConns[port] = udpConn

				go func(p uint16, uconn *net.UDPConn) {
					buf := make([]byte, 65535)
					for {
						n, _, err := uconn.ReadFromUDP(buf)
						if err != nil {
							log.Printf("[ERR] UDP recv port %d: %v", p, err)
							return
						}
						resp := buf[:n]

						respMu.Lock()
						responseQueue = append(responseQueue, framedMessage{
							port: p,
							data: append([]byte{}, resp...),
						})
						respMu.Unlock()
						log.Printf("[RECV] m45sci.xyz:%d → UDP %d (%d bytes)", p-udpPortOffset, p, n)
					}
				}(port, udpConn)
			}

			_, err = udpConn.Write(payload)
			if err != nil {
				log.Printf("[ERR] UDP send %d → %s: %v", port, target, err)
				continue
			}
			log.Printf("[FORWARD] TCP → m45sci.xyz:%d (%d bytes)", mappedPort, len(payload))
		}
	}

	for _, c := range udpConns {
		c.Close()
	}
	log.Printf("[CLEANUP] Closed UDP conns for %s", conn.RemoteAddr())
}
