package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"time"
)

type serverData struct {
	name string
	port int
}

type frameData struct {
	header   int
	payloads []payloadData
}

type payloadData struct {
	payloadLength int
	routeID       int
	contents      []byte
}

func (con *tunnelCon) ReadFrames() error {

	for {
		payloadLength, err := binary.ReadUvarint(con.Reader)
		if err != nil {
			return fmt.Errorf("ReadFrame: unable to read payload length: %v", err)
		}
		routeID, err := binary.ReadUvarint(con.Reader)
		if err != nil {
			return fmt.Errorf("ReadFrame: unable to read routeID: %v", err)
		}
		route := con.lookupRoute(int(routeID), false)
		if route == nil {
			return fmt.Errorf("ReadFrame: route not found")
		}

		payloadData := make([]byte, payloadLength)
		l, err := con.Con.Read(payloadData)
		if err != nil {
			return fmt.Errorf("ReadFrame: unable to read payload data: %v", err)
		}
		if l != int(payloadLength) {
			return fmt.Errorf("invalid payload length: %v", err)
		}

		log.Println("meep")
		sendPacket(route, payloadData)
	}
}

func sendPacket(route *routeData, payloadData []byte) error {
	return nil
}

func (con *tunnelCon) WriteFrame(payloads []payloadData) error {
	var header []byte
	binary.AppendUvarint(header, uint64(len(payloads)))

	for _, payload := range payloads {
		binary.AppendUvarint(header, uint64(len(payload.contents)))
		con.Con.Write(payload.contents)
	}
	return nil
}

func listenForTunnels() {
	addr := fmt.Sprintf(":%d", tunnelPort)
	tunnelListener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("[FATAL] Unable to listen on tunnel port: %v.", tunnelPort)
	}
	log.Printf("[START] Server tunnel listening on %v.", tunnelPort)

	for {
		conn, err := tunnelListener.Accept()
		if err != nil {
			log.Printf("[ERR] Tunnel accept: %v", err)
			continue
		}
		if tunnelCount > maxTunnels {
			conn.Close()
			continue
		}
		go handleTunnelConnection(conn)

		time.Sleep(tunnelListenThrottle)
	}
}
