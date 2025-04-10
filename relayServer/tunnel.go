package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

const (
	protocolVersion = 1
	TOP_ID          = 0xFFFFFFFE
)

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

	routeMapLock sync.Mutex
	IDMap        map[int]*routeData
	EphemeralMap map[int]*routeData
	routeTop     int
}

// Add connection to list, increment count
func startTunnelConn(c net.Conn) (*tunnelCon, error) {

	tunnelLock.Lock()
	defer tunnelLock.Unlock()
	//Loop until we manage to get an ID
	for {
		//Chances of this are 0, but handle it anyway
		if tunnelTop == TOP_ID {
			tunnelTop = 0
		}
		if tunnelList[tunnelTop] == nil {
			tunnelCount++
			reader := bufio.NewReader(c)

			//Read frame 0
			proto, err := binary.ReadUvarint(reader)
			if err != nil {
				return nil, fmt.Errorf("startTunnelConn: unable to read frame type: %v", err)
			}

			if proto != protocolVersion {
				log.Printf("startTunnelConn: Protocol version not compatible: %v")
				return nil, nil
			}

			serverID, err := binary.ReadUvarint(reader)
			if err != nil {
				return nil, fmt.Errorf("startTunnelConn: unable to read server id: %v", err)
			}

			newConn := &tunnelCon{ID: int(serverID), Reader: reader, Con: c}
			//New ID
			if serverID == 0 {
				tunnelTop++
				serverID = uint64(tunnelTop)
				newConn.IDMap = map[int]*routeData{}
				newConn.Born = time.Now()
				log.Printf("[CONNECT] Tunnel: %v connected.", newConn.ID)
			} else {
				log.Printf("[RESUME] Tunnel: %v connection resumed.", newConn.ID)
			}

			tunnelList[tunnelTop] = newConn
			go newConn.routeMapCleaner(tunnelTop)

			return newConn, nil
		}
	}

	return nil, nil
}

func (con *tunnelCon) Write(buf []byte) error {
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
		con.Close()
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

	con.ReadFrames()
}

func closeAllTunnels() {
	tunnelLock.Lock()
	defer tunnelLock.Unlock()
	log.Printf("Closing tunnels...")
	for _, c := range tunnelList {
		c.Close()
	}
}

func handleFrame(con *tunnelCon, frameData *frameData) error {
	return nil
}
