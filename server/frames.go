package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"time"
)

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

type frameData struct {
	frameType  int
	data       []byte
	dataLength int
}

func (con tunnelCon) ReadFrame() (*frameData, error) {
	frameType, err := binary.ReadUvarint(con.Reader)
	if err != nil {
		return nil, fmt.Errorf("ReadFrame: unable to read frameType: %v", err)
	}

	frameLength, err := binary.ReadUvarint(con.Reader)
	if err != nil {
		return nil, fmt.Errorf("ReadFrame: unable to read frame length: %v", err)
	}

	var payload = make([]byte, frameLength)
	len, err := con.Con.Read(payload)
	con.RecvBytes += len
	if len != int(frameLength) {
		return nil, fmt.Errorf("ReadFrame: unable to read frame data: %v", err)
	}

	return &frameData{frameType: int(frameType), dataLength: int(frameLength), data: payload}, nil
}

func (con tunnelCon) WriteFrame(frameType int, buf []byte) error {
	var header []byte
	binary.AppendUvarint(header, uint64(frameType))

	switch frameType {
	case FRAME_HELLO:
		binary.AppendUvarint(header, protocolVersion)
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

func handleFrame(con tunnelCon, fd frameData) error {
	switch fd.frameType {
	case FRAME_GOODBYE:
		if fd.dataLength == 0 {
			log.Printf("[GOODBYE] FROM ID: %v", con.ID)
			con.Close()
		}
	default:
		log.Printf("handleFrame: Invalid frame type: %v from ID: %v", fd.frameType, con.ID)
		con.Close()
	}

	return nil
}

func listenForTunnels() {
	addr := fmt.Sprintf(":%d", tunnelPort)
	tunnelListener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("[FATAL] Tunnel unable to listen on port: %v.", tunnelPort)
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
		log.Printf("[CONNECT] New tunnel: %s", conn.RemoteAddr())
		go handleTunnelConnection(conn)

		time.Sleep(tunnelListenThrottle)
	}
}
