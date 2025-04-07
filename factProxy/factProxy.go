package main

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"io"
	"log"
	"net"
	"os"
	"sync"
)

const (
	serverAddress = "127.0.0.1:30000" // Address of the factServer to connect to
	udpPortStart  = 20000             // Starting UDP port to listen on
	udpPortEnd    = 20018             // Ending UDP port (inclusive) to listen on
)

// FrameType defines the message type in the framing protocol
type FrameType byte

const (
	TypeRequest  FrameType = 0 // Message from proxy to server (UDP request)
	TypeResponse FrameType = 1 // Message from server to proxy (UDP response)
)

// compressData compresses a byte slice using zlib and returns the compressed data.
func compressData(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	_, err := w.Write(data)
	if err != nil {
		w.Close()
		return nil, err
	}
	err = w.Close() // flush and finalize compression
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// decompressData decompresses a zlib-compressed byte slice.
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

	// Connect to the TCP server
	conn, err := net.Dial("tcp", serverAddress)
	if err != nil {
		log.Fatalf("Failed to connect to server %s: %v", serverAddress, err)
	}
	defer conn.Close()
	log.Printf("Connected to factServer at %s", serverAddress)

	// Listen on UDP ports 20000–20018
	udpConns := make(map[uint16]*net.UDPConn)
	for port := udpPortStart; port <= udpPortEnd; port++ {
		addr := net.UDPAddr{Port: port}
		udpConn, err := net.ListenUDP("udp", &addr)
		if err != nil {
			log.Fatalf("Unable to bind UDP port %d: %v", port, err)
		}
		udpConns[uint16(port)] = udpConn
		log.Printf("Listening on UDP port %d", port)
	}

	// Map to remember the last client address for each port (for sending responses back)
	var clientAddrMap sync.Map // key: uint16 port, value: net.Addr

	// Mutex to synchronize writes to the TCP connection
	var tcpWriteMu sync.Mutex

	// Goroutine to handle incoming responses from the TCP stream (server -> proxy)
	go func() {
		header := make([]byte, 7) // 1 byte type, 2 bytes port, 4 bytes length
		for {
			// Read the fixed header first
			if _, err := io.ReadFull(conn, header); err != nil {
				if err == io.EOF {
					log.Println("TCP connection closed by server")
				} else {
					log.Printf("Error reading from TCP connection: %v", err)
				}
				// If connection is lost, terminate the proxy
				os.Exit(1)
			}
			frameType := FrameType(header[0])
			port := binary.BigEndian.Uint16(header[1:3])
			length := binary.BigEndian.Uint32(header[3:7])

			// Validate expected frame type
			if frameType != TypeResponse {
				log.Printf("Warning: unexpected frame type %d received, skipping", frameType)
				// Skip the payload for unknown frame type
				if length > 0 {
					io.CopyN(io.Discard, conn, int64(length))
				}
				continue
			}

			// Read the compressed payload
			payload := make([]byte, length)
			if _, err := io.ReadFull(conn, payload); err != nil {
				log.Printf("Error reading TCP payload (port %d): %v", port, err)
				os.Exit(1)
			}

			// Decompress the payload
			data, err := decompressData(payload)
			if err != nil {
				log.Printf("Decompression error for response on port %d: %v", port, err)
				continue // drop this message if corrupted
			}

			// Retrieve the original client address for this port
			addrVal, ok := clientAddrMap.Load(port)
			if !ok {
				log.Printf("No client address for port %d; dropping response", port)
				continue
			}
			clientAddr := addrVal.(net.Addr)

			// Send the UDP response back to the client on the correct port
			udpConn := udpConns[port]
			if udpConn == nil {
				log.Printf("No UDP socket for port %d; cannot forward response", port)
				continue
			}
			_, err = udpConn.WriteTo(data, clientAddr)
			if err != nil {
				log.Printf("Error sending UDP response to client %s (port %d): %v", clientAddr, port, err)
			} else {
				log.Printf("Forwarded UDP response to %s (port %d), %d bytes", clientAddr, port, len(data))
			}
		}
	}()

	// Start a goroutine for each UDP port to forward incoming packets to the TCP server
	for port, udpConn := range udpConns {
		go func(p uint16, uc *net.UDPConn) {
			buf := make([]byte, 65535)
			for {
				n, clientAddr, err := uc.ReadFrom(buf)
				if err != nil {
					if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
						log.Printf("Temporary error on UDP port %d: %v", p, err)
						continue
					}
					log.Printf("UDP port %d closed or error: %v", p, err)
					return
				}
				if n == 0 {
					continue
				}
				// Record the client address to send responses later
				clientAddrMap.Store(p, clientAddr)
				data := buf[:n]
				log.Printf("Received UDP packet from %s on port %d (%d bytes)", clientAddr, p, n)

				// Compress the UDP payload
				compData, err := compressData(data)
				if err != nil {
					log.Printf("Compression error on port %d: %v (dropping packet)", p, err)
					continue
				}

				// Construct the TCP frame (request message)
				payloadLen := uint32(len(compData))
				frame := make([]byte, 7+payloadLen)
				frame[0] = byte(TypeRequest)
				binary.BigEndian.PutUint16(frame[1:3], p)
				binary.BigEndian.PutUint32(frame[3:7], payloadLen)
				copy(frame[7:], compData)

				// Send the frame over TCP to the server
				tcpWriteMu.Lock()
				_, err = conn.Write(frame)
				tcpWriteMu.Unlock()
				if err != nil {
					log.Printf("Error forwarding packet from port %d to server: %v", p, err)
					// If TCP write fails, assume connection is broken; exit this goroutine
					return
				}
				log.Printf("Forwarded UDP packet from port %d to server (compressed %d bytes)", p, len(compData))
			}
		}(port, udpConn)
	}

	// Keep the main goroutine alive indefinitely (the proxy runs until interrupted)
	select {}
}
