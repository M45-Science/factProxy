package main

import (
	"bufio"
	"log"
	"net"
	"sync"
	"time"
)

const protocolVersion = 1

var (
	tunnelPort, maxTunnels int
	gamePorts              []int
	tunnelListenThrottle   time.Duration
	useCompression         bool
	verboseLog             bool

	tunnelLock  sync.Mutex
	tunnelTop   int
	tunnelCount int
	tunnelList  map[int]*tunnelCon
)

type tunnelCon struct {
	ID         int
	Reader     *bufio.Reader
	Con        net.Conn
	Born       time.Time
	LastActive time.Time
	RecvBytes  int
	SendBytes  int
}

// Add connection to list, increment count
func startTunnelConn(c net.Conn) *tunnelCon {
	tunnelTop++

	tunnelLock.Lock()
	defer tunnelLock.Unlock()
	if tunnelList[tunnelTop] == nil {
		tunnelCount++
		reader := bufio.NewReader(c)
		newConn := &tunnelCon{ID: tunnelTop, Reader: reader, Con: c, Born: time.Now()}
		tunnelList[tunnelTop] = newConn

		log.Printf("[CONNECT] ID: %v connected.", newConn.ID)
		return newConn
	}

	return nil
}

func (con tunnelCon) Write(buf []byte) error {
	bufLen := len(buf)
	l, err := con.Con.Write(buf)
	if bufLen != l || err != nil {
		return err
	}

	con.SendBytes += bufLen
	return nil
}

// Close connection, remove from list, decrement connection count
func (con *tunnelCon) Close() {
	if con == nil {
		log.Fatal("attempt to close nil tunnelCon")
	}
	tunnelLock.Lock()
	defer tunnelLock.Unlock()
	if tunnelList[con.ID] != nil {
		con.WriteFrame(FRAME_GOODBYE, nil)
		log.Printf("[DISCONNECT] Tunnel ID: %v was disconnected.", con.ID)
		tunnelCount--
		delete(tunnelList, con.ID)
	}
}

// Read and process tunnel frames
func handleTunnelConnection(c net.Conn) {

	con := startTunnelConn(c)
	if con == nil {
		return
	}
	defer con.Close()

	frameData, err := con.ReadFrame()
	if frameData == nil || err != nil {
		return
	}

	if frameData.frameType == FRAME_HELLO {
		if protocolVersion == frameData.payloadLength {
			for {
				fd, err := con.ReadFrame()
				if err != nil {
					return
				}
				err = handleFrame(*con, *fd)
			}
		} else {
			log.Printf("Protocol version not compatible: ID: %v, Version: %v", con.ID, frameData.payloadLength)
		}
	} else {
		log.Printf("Did not receive hello frame from ID: %v.", con.ID)
	}
}

func closeAllTunnels() {
	tunnelLock.Lock()
	defer tunnelLock.Unlock()
	for _, c := range tunnelList {
		c.Close()
	}
}
