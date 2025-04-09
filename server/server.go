package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const (
	PROTO_VERSION     = 1
	defaultListenPort = 30000

	defaultMaxClients     = 100
	defaultMaxGameClients = 100

	defaultListenMS    = 1000
	defaultCompression = true
)

var (
	listenPort, maxClients, maxGameClients int
	numClients                             int
	listenThrottle                         = time.Second
	listenThrottleMS                       int
	useCompression                         bool
)

var (
	connLock sync.Mutex
	numConn  int
	connTop  int
	connList map[int]*connData
)

type connData struct {
	ID         int
	Conn       net.Conn
	Born       time.Time
	LastActive time.Time
	RecvBytes  int
	SendBytes  int
}

const (
	FRAME_HELLO = iota
	FRAME_RESPONSE
	FRAME_REPLY
)

func main() {
	// Channel to receive OS signals
	sigs := make(chan os.Signal, 1)
	// Notify for SIGINT (Ctrl+C) and SIGTERM
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	flag.IntVar(&listenPort, "listenPort", defaultListenPort, "TCP Port to listen for proxy clients on.")
	flag.IntVar(&maxClients, "maxClients", defaultMaxClients, "Maximum proxy clients.")
	flag.IntVar(&maxGameClients, "maxGameClients", defaultMaxGameClients, "Maximum game clients.")
	flag.IntVar(&listenThrottleMS, "listenThrottle", defaultListenMS, "Only answer check for new connections every X milliseconds.")
	flag.BoolVar(&useCompression, "useCompression", defaultCompression, "")
	flag.Parse()

	//Convert flag int to duration
	listenThrottle = time.Millisecond * time.Duration(listenThrottleMS)

	addr := fmt.Sprintf(":%d", listenPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("[FATAL] Unable to listen on port: %v.", listenPort)
	}
	log.Printf("[START] Server listening on %v.", listenPort)

	go func() {
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
	}()

	<-sigs
	// TO DO: Handle shutdown here
	log.Println("[QUIT] Server shutting down.")
}

func handleConnection(conn net.Conn) {
	//Limit max connections
	if numConn > numClients {
		conn.Close()
	}

	cond := startConn(conn)
	if cond == nil {
		return
	}
	defer closeConn(cond)

	var helloBuf []byte
	reader := bytes.NewReader(helloBuf)

	var Version int
	err := binary.Read(reader, binary.LittleEndian, Version)
	if err != nil {
		log.Printf("Unable to read header field: Version: %v", err)
		return
	}

	for {
		var err error
		var headerBuf []byte
		if _, err := io.ReadFull(conn, headerBuf); err != nil {
			log.Printf("[DISCONNECT] %s (%v)", conn.RemoteAddr(), err)
			break
		}
		reader := bytes.NewReader(headerBuf)

		var Length uint64
		Length, err = binary.ReadUvarint(reader)
		if err != nil {
			log.Printf("Unable to read header field: Length: %v", err)
			break
		}
		var FrameType uint64
		FrameType, err = binary.ReadUvarint(reader)
		if err != nil {
			log.Printf("Unable to read header field: FrameType: %v", err)
			break
		}

		if Length != 0 && FrameType != 0 {
			//Placeholder
		}
	}
}

// Close connection, remove from list, decrement connection count
func closeConn(cond *connData) {
	if cond == nil {
		return
	}
	if connList[cond.ID] != nil {
		numClients--
		delete(connList, cond.ID)
	}
}

// Add connection to list, increment count
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
