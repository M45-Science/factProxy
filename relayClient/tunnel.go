package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"strconv"
	"time"
)

func connectHandler() {
	lastConnect := time.Now()
	for attempts := 0; attempts < maxAttempts; attempts++ {
		if attempts != 0 {
			time.Sleep(time.Duration(reconDelaySec) * time.Second)
		}

		log.Printf("Connecting to %v...", tunnelServerAddr)
		var err error
		tunnelCon, err = net.Dial("tcp", tunnelServerAddr)
		if err != nil {
			log.Printf("Unable to connect: %v", err)
			continue
		}

		//Write frame 0
		var buf []byte
		buf = binary.AppendUvarint(buf, CLIENT_KEY)
		buf = binary.AppendUvarint(buf, protocolVersion)
		buf = binary.AppendUvarint(buf, uint64(serverID))
		bufLen := len(buf)

		l, err := tunnelCon.Write(buf)
		if err != nil {
			log.Printf("Tunnel write error: %v", err)
			continue
		}
		if l != bufLen {
			log.Printf("Tunnel write error: length differs: %vb of %vb.", l, bufLen)
			continue
		}

		err = handleTunnel(tunnelCon)
		if err != nil {
			log.Printf("handleTunnel: %v", err)
		}

		//Eventually reset tries
		if time.Since(lastConnect) > reconResetAfter {
			attempts = 0
		}
		lastConnect = time.Now()
	}

	log.Printf("Too many unsuccsessful connection attempts (%v), stopping.\nRelaunch to try again.", maxAttempts)
	time.Sleep(time.Hour * 24)
}

func handleTunnel(c net.Conn) error {
	reader := bufio.NewReader(c)

	//Read key
	key, err := binary.ReadUvarint(reader)
	if err != nil {
		return fmt.Errorf("unable to read key: %v", err)
	}
	if key != SERVER_KEY {
		return fmt.Errorf("incorrect key.")
	}

	//Protocol version
	proto, err := binary.ReadUvarint(reader)
	if err != nil {
		return fmt.Errorf("unable to read protocol version: %v", err)
	}
	if proto != protocolVersion {
		return fmt.Errorf("protocol version not compatible: %v", proto)
	}

	//Server ID
	sid, err := binary.ReadUvarint(reader)
	if err != nil {
		return fmt.Errorf("unable to read server id: %v", err)
	}
	serverID = int(sid)

	//GamePort Count
	gamePortCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return fmt.Errorf("unable to read gamePortCount: %v", err)
	}
	gamePorts = []int{}
	portsStr := ""
	listeners = []*net.UDPConn{}
	for p := range gamePortCount {
		port, err := binary.ReadUvarint(reader)
		if err != nil {
			return fmt.Errorf("unable to read gamePort: %v", err)
		}
		gamePorts = append(gamePorts, int(port))
		if p != 0 {
			portsStr = portsStr + ", "
		}
		portsStr = portsStr + strconv.FormatUint(port, 10)

		laddr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: int(port)}
		conn, _ := net.ListenUDP("udp", laddr)
		defer conn.Close()
		listeners = append(listeners, conn)
	}
	log.Printf("Game ports: %v", portsStr)
	outputServerList()

	go handleGameListens()
	handleTunnelListen(reader)

	return nil
}

func handleTunnelListen(reader *bufio.Reader) {
	for {
		payloadLen, err := binary.ReadUvarint(reader)
		if err != nil {
			log.Printf("unable to read payload length: %v", err)
			return
		}
		var payload = make([]byte, payloadLen)
		tunnelCon.Read(payload)

		//Process payload
		log.Printf("Payload: %v", payloadLen)
	}
}
