// factProxy.go (with batching)
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
	serverAddress   = "127.0.0.1:30000"
	udpPortStart    = 20000
	udpPortEnd      = 20018
	tickRate        = time.Second / 30 // 1/30th of a second
	maxPayloadBytes = 1 << 20          // 1MB per batch
)

type FrameType byte

const (
	TypeRequest      FrameType = 0
	TypeResponse     FrameType = 1
	TypeBatchRequest FrameType = 2
	TypeBatchReply   FrameType = 3
)

type framedMessage struct {
	typ  FrameType
	port uint16
	data []byte
}

var (
	udpConns      = make(map[uint16]*net.UDPConn)
	clientAddrMap sync.Map
	tcpWriteMu    sync.Mutex
	outgoingQueue = make([]framedMessage, 0)
	outMu         sync.Mutex
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
		log.Printf("[CONNECTING] TCP → %s", serverAddress)
		conn, err := net.Dial("tcp", serverAddress)
		if err != nil {
			log.Printf("[RETRY] TCP connect failed: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}
		log.Printf("[CONNECTED] TCP to %s", serverAddress)
		runProxy(conn)
		log.Printf("[DISCONNECTED] TCP lost, reconnecting...")
		time.Sleep(2 * time.Second)
	}
}

func runProxy(conn net.Conn) {
	defer conn.Close()

	go tcpReader(conn)
	go tcpBatchWriter(conn)

	for port, udpConn := range udpConns {
		go func(p uint16, uc *net.UDPConn) {
			buf := make([]byte, 65535)
			for {
				n, addr, err := uc.ReadFrom(buf)
				if err != nil {
					log.Printf("[ERR] UDP read %d: %v", p, err)
					return
				}
				clientAddrMap.Store(p, addr)
				data := buf[:n]
				log.Printf("[RECV] UDP %d ← %s (%d bytes)", p, addr.String(), n)

				// Create frame and queue it
				msg := new(bytes.Buffer)
				msg.WriteByte(byte(TypeRequest))
				binary.Write(msg, binary.BigEndian, p)
				binary.Write(msg, binary.BigEndian, uint32(len(data)))
				msg.Write(data)

				outMu.Lock()
				outgoingQueue = append(outgoingQueue, framedMessage{
					typ:  TypeRequest,
					port: p,
					data: msg.Bytes(),
				})
				outMu.Unlock()
			}
		}(port, udpConn)
	}
}

func tcpBatchWriter(conn net.Conn) {
	ticker := time.NewTicker(tickRate)
	defer ticker.Stop()

	for range ticker.C {
		outMu.Lock()
		if len(outgoingQueue) == 0 {
			outMu.Unlock()
			continue
		}

		var batchData bytes.Buffer
		for _, msg := range outgoingQueue {
			batchData.Write(msg.data)
		}
		outgoingQueue = outgoingQueue[:0]
		outMu.Unlock()

		comp, err := compressData(batchData.Bytes())
		if err != nil {
			log.Printf("[ERR] Compress batch: %v", err)
			continue
		}

		frame := new(bytes.Buffer)
		frame.WriteByte(byte(TypeBatchRequest))
		binary.Write(frame, binary.BigEndian, uint32(len(comp)))
		frame.Write(comp)

		tcpWriteMu.Lock()
		_, err = conn.Write(frame.Bytes())
		tcpWriteMu.Unlock()
		if err != nil {
			log.Printf("[ERR] Write batch to server: %v", err)
			return
		}
		log.Printf("[SEND] Batch (%d bytes compressed)", len(comp))
	}
}

func tcpReader(conn net.Conn) {
	for {
		header := make([]byte, 5)
		if _, err := io.ReadFull(conn, header); err != nil {
			log.Printf("[ERR] TCP read header: %v", err)
			return
		}
		frameType := FrameType(header[0])
		length := binary.BigEndian.Uint32(header[1:5])
		body := make([]byte, length)
		if _, err := io.ReadFull(conn, body); err != nil {
			log.Printf("[ERR] TCP read body: %v", err)
			return
		}

		if frameType != TypeBatchReply {
			log.Printf("[WARN] Unexpected frame type %d", frameType)
			continue
		}

		unzipped, err := decompressData(body)
		if err != nil {
			log.Printf("[ERR] Decompress batch reply: %v", err)
			continue
		}

		buf := bytes.NewBuffer(unzipped)
		for buf.Len() >= 7 {
			hdr := buf.Next(7)
			ptype := FrameType(hdr[0])
			port := binary.BigEndian.Uint16(hdr[1:3])
			size := binary.BigEndian.Uint32(hdr[3:7])
			if buf.Len() < int(size) {
				log.Printf("[WARN] Skipping incomplete frame")
				break
			}
			payload := buf.Next(int(size))

			if ptype != TypeResponse {
				log.Printf("[WARN] Skipping unexpected inner type: %d", ptype)
				continue
			}

			addrVal, ok := clientAddrMap.Load(port)
			if !ok {
				log.Printf("[WARN] No addr for UDP %d", port)
				continue
			}
			udpConn := udpConns[port]
			if udpConn != nil {
				udpConn.WriteTo(payload, addrVal.(net.Addr))
				log.Printf("[RECV] TCP → UDP %d → %s (%d bytes)", port, addrVal.(net.Addr).String(), len(payload))
			}
		}
	}
}
