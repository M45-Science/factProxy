package main

import (
	"bytes"
	"compress/zlib"
	"context"
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
	tickRate        = time.Second / 30 // 1/30th second flush
	maxPayloadBytes = 1 << 20
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

var (
	udpConns      = make(map[uint16]*net.UDPConn)
	clientAddrMap sync.Map
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup

	// TCP reader
	wg.Add(1)
	go func() {
		defer wg.Done()
		tcpReader(ctx, conn, cancel)
	}()

	// TCP batch writer
	wg.Add(1)
	go func() {
		defer wg.Done()
		tcpBatchWriter(ctx, conn, cancel)
	}()

	// UDP listeners
	for port, udpConn := range udpConns {
		wg.Add(1)
		go func(p uint16, uc *net.UDPConn) {
			defer wg.Done()
			handleUDP(ctx, p, uc, cancel)
		}(port, udpConn)
	}

	wg.Wait()
	conn.Close()
	log.Printf("[CLEANUP] TCP session closed")
}

func handleUDP(ctx context.Context, port uint16, udpConn *net.UDPConn, cancel context.CancelFunc) {
	buf := make([]byte, 65535)
	for {
		select {
		case <-ctx.Done():
			return
		default:
			udpConn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			n, clientAddr, err := udpConn.ReadFrom(buf)
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					continue
				}
				log.Printf("[ERR] UDP read %d: %v", port, err)
				cancel()
				return
			}
			clientAddrMap.Store(port, clientAddr)

			data := buf[:n]
			log.Printf("[RECV] UDP %d ← %s (%d bytes)", port, clientAddr.String(), n)

			frame := new(bytes.Buffer)
			frame.WriteByte(byte(TypeRequest))
			binary.Write(frame, binary.BigEndian, port)
			binary.Write(frame, binary.BigEndian, uint32(len(data)))
			frame.Write(data)

			outMu.Lock()
			outgoingQueue = append(outgoingQueue, framedMessage{
				port: port,
				data: frame.Bytes(),
			})
			outMu.Unlock()
		}
	}
}

func tcpBatchWriter(ctx context.Context, conn net.Conn, cancel context.CancelFunc) {
	ticker := time.NewTicker(tickRate)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
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

			_, err = conn.Write(frame.Bytes())
			if err != nil {
				log.Printf("[ERR] TCP write failed: %v", err)
				cancel()
				return
			}
			log.Printf("[SEND] Batch (%d bytes compressed)", len(comp))
		}
	}
}

func tcpReader(ctx context.Context, conn net.Conn, cancel context.CancelFunc) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			header := make([]byte, 5)
			if _, err := io.ReadFull(conn, header); err != nil {
				log.Printf("[ERR] TCP read header: %v", err)
				cancel()
				return
			}
			frameType := FrameType(header[0])
			length := binary.BigEndian.Uint32(header[1:5])
			body := make([]byte, length)
			if _, err := io.ReadFull(conn, body); err != nil {
				log.Printf("[ERR] TCP read body: %v", err)
				cancel()
				return
			}

			if frameType != TypeBatchReply {
				log.Printf("[WARN] Unexpected frame type %d", frameType)
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
					log.Printf("[WARN] Truncated inner frame")
					break
				}
				payload := buf.Next(int(size))

				if ptype != TypeResponse {
					log.Printf("[WARN] Unexpected inner frame: %d", ptype)
					continue
				}

				addrVal, ok := clientAddrMap.Load(port)
				if !ok {
					log.Printf("[WARN] Unknown client for UDP %d", port)
					continue
				}

				udpConn := udpConns[port]
				if udpConn == nil {
					log.Printf("[ERR] No UDPConn for port %d", port)
					continue
				}

				_, err := udpConn.WriteTo(payload, addrVal.(net.Addr))
				if err != nil {
					log.Printf("[ERR] Send UDP %d → %s: %v", port, addrVal, err)
				} else {
					log.Printf("[SEND] UDP %d → %s (%d bytes)", port, addrVal, len(payload))
				}
			}
		}
	}
}
