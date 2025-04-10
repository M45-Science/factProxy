package main

import (
	"encoding/binary"
	"flag"
	"log"
	"net"
	"time"
)

var (
	serverAddr     string
	useCompression bool

	RecvBytes, SendBytes int
)

const (
	SERVER_KEY = 0xCAFE69C0FFEE
	CLIENT_KEY = 0xADD069C0FFEE
)

const (
	PROTO_VERSION = 1
	ReconDelaySec = 5
	maxAttempts   = 25
)

func main() {
	flag.StringVar(&serverAddr, "server", "m45sci.xyz:30000", "server:port")
	flag.Parse()

	for x := 0; x < maxAttempts; x++ {
		if x != 0 {
			time.Sleep(time.Duration(ReconDelaySec) * time.Second)
		}

		conn, err := net.Dial("tcp", serverAddr)
		if err != nil {
			log.Printf("Dial: Unable to connect to %v: %v", serverAddr, err)
			continue
		}

		var buf []byte
		binary.PutUvarint(buf, PROTO_VERSION)
		conn.Write(buf)
	}

	log.Printf("Too many connection attempts (%v), stopping. Relaunch to try again.", maxAttempts)
	time.Sleep(time.Minute)
}
