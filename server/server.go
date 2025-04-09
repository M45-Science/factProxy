package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/pierrec/lz4"
)

const (
	defaultMaxClients = 50
	defaultListenPort = 30000
	defaultListenMS   = 100
)

var (
	listenPort, maxClients int
	numClients             int
	listenThrottle         = time.Second
	listenThrottleMS       int
)

var (
	connLock sync.Mutex
	numConn  int
	connTop  uint64
	connList map[uint64]*connData
)

type connData struct {
	ID        uint64
	Conn      net.Conn
	Born      time.Time
	RecvBytes uint64
	SendBytes uint64
}

const headerSize = 24 / 8

type HeaderData struct {
	Length    uint16
	FrameType uint8
}

const helloSize = 16 / 8

type ConnHello struct {
	Version     uint8
	Compression uint8
}

const (
	COMPRESS_NONE = iota
	COMPRESS_LZ4
	COMPRESS_ZSTD
	COMPRESS_GZIP
	COMPRESS_ZIP
)

const (
	FRAME_HELLO = iota
	FRAME_RESPONSE
	FRAME_REPLY
)

func main() {
	flag.IntVar(&listenPort, "listenPort", defaultListenPort, "TCP Port to listen for proxy clients on")
	flag.IntVar(&maxClients, "maxClients", defaultMaxClients, "Maximum proxy clients")
	flag.IntVar(&listenThrottleMS, "listenThrottle", defaultListenMS, "Only answer check for new connections every X milliseconds.")
	flag.Parse()

	//Convert flag int to duration
	listenThrottle = time.Millisecond * time.Duration(listenPort)

	addr := fmt.Sprintf(":%d", listenPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("[FATAL] Unable to listen on port: %v.", listenPort)
	}
	log.Printf("[START] Server listening on %v.", listenPort)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("[ERR] Accept: %v", err)
			continue
		}
		if numClients > maxClients {
			conn.Close()
			continue
		}
		log.Printf("[CONNECT] From proxy: %s", conn.RemoteAddr())
		go handleConnection(conn)

		time.Sleep(listenThrottle)
	}
}

func handleConnection(conn net.Conn) {
	cond := startConn(conn)
	if cond == nil {
		return
	}
	defer closeConn(cond)

	var result ConnHello
	var helloBuf = make([]byte, helloSize)
	reader := bytes.NewReader(helloBuf)
	err := binary.Read(reader, binary.LittleEndian, result.Version)
	if err != nil {
		log.Printf("Unable to read header field: Version: %v", err)
		return
	}
	err = binary.Read(reader, binary.LittleEndian, result.Compression)
	if err != nil {
		log.Printf("Unable to read header field: Version: %v", err)
		return
	}

	for {
		var headerBuf = make([]byte, headerSize)
		if _, err := io.ReadFull(conn, headerBuf); err != nil {
			log.Printf("[DISCONNECT] %s (%v)", conn.RemoteAddr(), err)
			break
		}

		var result HeaderData
		reader := bytes.NewReader(headerBuf)
		err := binary.Read(reader, binary.LittleEndian, result.Length)
		if err != nil {
			log.Printf("Unable to read header field: Length: %v", err)
			break
		}
		err = binary.Read(reader, binary.LittleEndian, result.FrameType)
		if err != nil {
			log.Printf("Unable to read header field: FrameType: %v", err)
			break
		}

		// Ready Body
		zr := lz4.NewReader(headerBuf)
		if zr != nil {
			//
		}
	}
}

func closeConn(cond *connData) {
	if cond == nil {
		return
	}
	if connList[cond.ID] != nil {
		numClients--
		delete(connList, cond.ID)
	}
}

func startConn(conn net.Conn) *connData {
	connTop++

	if connList[connTop] == nil {
		numClients++
		newConn := &connData{ID: connTop, Conn: conn, Born: time.Now()}
		connList[connTop] = newConn
		return newConn
	}

	return nil
}
