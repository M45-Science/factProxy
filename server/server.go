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
	FRAME_MESSAGE
	FRAME_REPLY
	FRAME_GOODBYE
)

var frameName []string = []string{
	"HELLO",
	"MESSAGE",
	"REPLY",
	"GOODBYE",
	"UNKNOWN",
}

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
	log.Printf("[QUIT] Server shutting down: Signal: %v", sigs)
	connLock.Lock()
	defer connLock.Unlock()
	for _, c := range connList {
		c.Close()
	}
	log.Printf("Goodbye")
}

func handleConnection(c net.Conn) {

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
		if PROTO_VERSION == frameData.length {
			for {
				fd, err := con.ReadFrame()
				if err != nil {
					return
				}
				err = handleFrame(*con, *fd)
			}
		} else {
			log.Printf("Protocol version not compatible: ID: %v, Version: %v", con.ID, frameData.length)
		}
	} else {
		log.Printf("Did not receive hello frame from ID: %v.", con.ID)
	}
}

func handleFrame(con connData, fd frameData) error {
	switch fd.frameType {
	case FRAME_GOODBYE:
		if fd.length == 0 {
			log.Printf("[GOODBYE] FROM ID: %v", con.ID)
			con.Close()
		}
	default:
		log.Printf("handleFrame: Invalid frame type: %v from ID: %v", fd.frameType, con.ID)
		con.Close()
	}

	return nil
}

func (con connData) WriteFrame(frameType int, buf []byte) error {
	var header []byte
	binary.AppendUvarint(header, uint64(frameType))

	switch frameType {
	case FRAME_HELLO:
		binary.AppendUvarint(header, PROTO_VERSION)
		con.Write(header)
	case FRAME_MESSAGE:
		//
	case FRAME_REPLY:
		//
	case FRAME_GOODBYE:
		log.Printf("[GOODBYE] TO ID: %v", con.ID)
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

	var payload = make([]byte, frameLength)
	len, err := con.Conn.Read(payload)
	con.RecvBytes += len
	if len != int(frameLength) {
		return nil, fmt.Errorf("ReadFrame: unable to read frame data: %v", err)
	}

	return &frameData{frameType: int(frameType), length: int(frameLength), data: payload}, nil
}

// Close connection, remove from list, decrement connection count
func (con *connData) Close() {
	if con == nil {
		return
	}
	connLock.Lock()
	defer connLock.Unlock()
	if connList[con.ID] != nil {
		con.WriteFrame(FRAME_GOODBYE, nil)
		log.Printf("[DISCONNECT] ID: %v was disconnected.", con.ID)
		numClients--
		delete(connList, con.ID)
	}
}

// Add connection to list, increment count
func startConn(c net.Conn) *connData {
	connTop++

	connLock.Lock()
	defer connLock.Unlock()
	if connList[connTop] == nil {
		numClients++
		reader := bufio.NewReader(c)
		newConn := &connData{ID: connTop, Reader: reader, Conn: c, Born: time.Now()}
		connList[connTop] = newConn

		log.Printf("[CONNECT] ID: %v connected.", newConn.ID)
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

	con.SendBytes += bufLen
	return nil
}
