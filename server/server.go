package main

import (
	"bufio"
	"encoding/binary"
	"flag"
	"fmt"
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
	Reader     *bufio.Reader
	Conn       net.Conn
	Born       time.Time
	LastActive time.Time
	RecvBytes  int
	SendBytes  int
}

type frameData struct {
	frameType int
	length    int
	data      []byte
}

const (
	FRAME_HELLO = iota
	FRAME_RESPONSE
	FRAME_REPLY
	FRAME_GOODBYE
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
	log.Printf("[QUIT] Server shutting down: %v", sigs)
}

func handleConnection(c net.Conn) {
	//Limit max connections
	if numConn > numClients {
		c.Close()
	}

	con := startConn(c)
	if con == nil {
		return
	}
	defer con.Close()

	frameData, err := con.ReadFrame()
	if frameData == nil || err != nil {
		return
	}

	if frameData.frameType == FRAME_HELLO {
		if frameData.length > 0 {

		}
	}
}

func (con connData) WriteFrame(frameType int, buf []byte) error {
	var header []byte
	binary.AppendUvarint(header, uint64(frameType))

	switch frameType {
	case FRAME_HELLO:
		binary.AppendUvarint(header, PROTO_VERSION)
		con.Write(header)
	case FRAME_RESPONSE:
		//
	case FRAME_REPLY:
		//
	case FRAME_GOODBYE:
		binary.AppendUvarint(header, 0)
		con.Write(header)
		con.Close()
	default:
		return fmt.Errorf("invalid frame type: %v", frameType)
	}

	return nil
}

func (con connData) ReadFrame() (*frameData, error) {
	frameType, err := binary.ReadUvarint(con.Reader)
	if err != nil {
		return nil, fmt.Errorf("ReadFrame: unable to read frameType: %v", err)
	}

	frameLength, err := binary.ReadUvarint(con.Reader)
	if err != nil {
		return nil, fmt.Errorf("ReadFrame: unable to read frame length: %v", err)
	}

	if frameType == FRAME_GOODBYE {
		if frameLength == 0 {
			log.Printf("Client: %v goodbye.", con.ID)
			con.Close()
			return nil, nil
		}
	}

	var payload = make([]byte, frameLength)
	len, err := con.Conn.Read(payload)
	if len != int(frameLength) {
		return nil, fmt.Errorf("ReadFrame: unable to read frame data: %v", err)
	}

	return &frameData{frameType: int(frameType), length: int(frameLength), data: payload}, nil
}

// Close connection, remove from list, decrement connection count
func (cond *connData) Close() {
	if cond == nil {
		return
	}
	if connList[cond.ID] != nil {
		numClients--
		delete(connList, cond.ID)
	}
}

// Add connection to list, increment count
func startConn(c net.Conn) *connData {
	connTop++

	if connList[connTop] == nil {
		numClients++
		reader := bufio.NewReader(c)
		newConn := &connData{ID: connTop, Reader: reader, Conn: c, Born: time.Now()}
		connList[connTop] = newConn
		return newConn
	}

	return nil
}

func (con connData) Write(buf []byte) error {
	bufLen := len(buf)
	l, err := con.Conn.Write(buf)
	if bufLen != l || err != nil {
		return err
	}

	return nil
}
