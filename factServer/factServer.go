package main

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"io"
	"log"
	"net"
	"sync"
)

const (
	listenAddress = ":30000"           // TCP address to listen on for proxy connections
	targetAddress = "m45sci.xyz:10000" // UDP target (game server address and port)
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
	err = w.Close()
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

	// Resolve the UDP target address (the actual game server)
	targetUDPAddr, err := net.ResolveUDPAddr("udp", targetAddress)
	if err != nil {
		log.Fatalf("Failed to resolve target address %s: %v", targetAddress, err)
	}

	// Start listening for incoming TCP connections from factProxy
	listener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", listenAddress, err)
	}
	log.Printf("factServer listening on %s (forwarding UDP to %s)", listenAddress, targetAddress)

	for {
		tcpConn, err := listener.Accept()
		if err != nil {
			log.Printf("TCP accept error: %v", err)
			continue
		}
		log.Printf("Accepted connection from proxy %s", tcpConn.RemoteAddr())
		go handleConnection(tcpConn, targetUDPAddr)
	}
}

// handleConnection handles a single proxy connection over TCP.
func handleConnection(conn net.Conn, targetAddr *net.UDPAddr) {
	defer conn.Close()

	// Map of UDP port -> UDP connection (for sending to m45sci.xyz using that source port)
	udpConns := make(map[uint16]*net.UDPConn)
	// Mutex to synchronize writes on the TCP connection (for sending responses back)
	var tcpWriteMu sync.Mutex

	header := make([]byte, 7) // buffer for 7-byte message header
	for {
		// Read the message header from the proxy
		if _, err := io.ReadFull(conn, header); err != nil {
			if err == io.EOF {
				log.Printf("Proxy %s disconnected", conn.RemoteAddr())
			} else {
				log.Printf("Error reading from proxy connection: %v", err)
			}
			break // exit the loop on connection error/closure
		}
		frameType := FrameType(header[0])
		port := binary.BigEndian.Uint16(header[1:3])
		length := binary.BigEndian.Uint32(header[3:7])

		if frameType != TypeRequest {
			log.Printf("Warning: unexpected frame type %d from proxy, skipping", frameType)
			if length > 0 {
				// Discard the payload of an unknown message type
				io.CopyN(io.Discard, conn, int64(length))
			}
			continue
		}

		// Read the compressed UDP payload from the proxy
		payload := make([]byte, length)
		if _, err := io.ReadFull(conn, payload); err != nil {
			log.Printf("Error reading request payload (port %d): %v", port, err)
			break
		}
		log.Printf("Received UDP request from proxy on port %d (compressed %d bytes)", port, length)

		// Decompress the payload to get the original UDP data
		data, err := decompressData(payload)
		if err != nil {
			log.Printf("Decompression error on port %d: %v (dropping packet)", port, err)
			continue // skip this packet on error
		}

		// Get or create a UDP socket for this port to communicate with the target server
		udpConn, exists := udpConns[port]
		if !exists {
			// Try to bind a UDP socket to the same port as the client
			localAddr := net.UDPAddr{Port: int(port)}
			uconn, err := net.DialUDP("udp", &localAddr, targetAddr)
			if err != nil {
				log.Printf("Port %d unavailable, using an ephemeral port: %v", port, err)
				uconn, err = net.DialUDP("udp", nil, targetAddr)
				if err != nil {
					log.Printf("Failed to open UDP socket for port %d: %v (dropping packet)", port, err)
					continue
				}
				// Log which ephemeral port was chosen for this mapping
				localPort := uconn.LocalAddr().(*net.UDPAddr).Port
				log.Printf("Using UDP port %d for outgoing traffic (mapped from port %d)", localPort, port)
			} else {
				log.Printf("Opened UDP socket on port %d for outgoing traffic", port)
			}
			udpConn = uconn
			udpConns[port] = udpConn

			// Start a goroutine to listen for responses from the target on this UDP socket
			go func(p uint16, uconn *net.UDPConn) {
				buf := make([]byte, 65535)
				for {
					n, _, err := uconn.ReadFromUDP(buf)
					if err != nil {
						if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
							log.Printf("Temporary UDP read error on port %d: %v", p, err)
							continue
						}
						// Socket closed or fatal error
						log.Printf("UDP socket for port %d closed: %v", p, err)
						return
					}
					if n == 0 {
						continue
					}
					log.Printf("Received UDP response from target for port %d (%d bytes)", p, n)
					respData := buf[:n]

					// Compress the response data
					compData, err := compressData(respData)
					if err != nil {
						log.Printf("Compression error on response for port %d: %v (dropping)", p, err)
						continue
					}

					// Frame the response message for the proxy
					respLen := uint32(len(compData))
					frame := make([]byte, 7+respLen)
					frame[0] = byte(TypeResponse)
					binary.BigEndian.PutUint16(frame[1:3], p)
					binary.BigEndian.PutUint32(frame[3:7], respLen)
					copy(frame[7:], compData)

					// Send the response frame over TCP back to the proxy
					tcpWriteMu.Lock()
					_, err = conn.Write(frame)
					tcpWriteMu.Unlock()
					if err != nil {
						log.Printf("Error sending response to proxy (port %d): %v", p, err)
						return
					}
					log.Printf("Forwarded UDP response for port %d to proxy (compressed %d bytes)", p, len(compData))
				}
			}(port, udpConn)
		}

		// Forward the original UDP payload to the target game server
		_, err = udpConn.Write(data)
		if err != nil {
			log.Printf("Error sending UDP packet to target from port %d: %v", port, err)
			// Continue to next message; do not break the connection on a single UDP send failure
			continue
		}
		log.Printf("Forwarded UDP packet to target from port %d (%d bytes)", port, len(data))
	}

	// Cleanup: close all UDP sockets associated with this connection
	for p, uconn := range udpConns {
		uconn.Close()
		delete(udpConns, p)
	}
	log.Printf("Closed connection from proxy %s, cleaned up UDP sockets", conn.RemoteAddr())
}
