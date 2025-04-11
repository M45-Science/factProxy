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
	"strconv"
	"syscall"
	"time"
)

const (
	SERVER_KEY = 0xCAFE69C0FFEE
	CLIENT_KEY = 0xADD069C0FFEE
)

const (
	protocolVersion = 1

	ReconDelaySec   = 5
	maxAttempts     = 2 * (60 / ReconDelaySec)
	ReconResetAfter = time.Minute * 5
)

var (
	serverID       int
	serverAddr     string
	useCompression bool
	con            net.Conn
	gamePorts      []int
)

func main() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	flag.StringVar(&serverAddr, "server", "m45sci.xyz:30000", "server:port")
	flag.Parse()

	go connectHandler()

	<-sigs
	log.Printf("[QUIT] Server shutting down: Signal: %v", sigs)
	if con != nil {
		con.Close()
	}
	log.Printf("Goodbye")
}

func connectHandler() {
	lastConnect := time.Now()
	for attempts := 0; attempts < maxAttempts; attempts++ {
		if attempts != 0 {
			time.Sleep(time.Duration(ReconDelaySec) * time.Second)
		}

		log.Printf("Connecting to %v...", serverAddr)
		var err error
		con, err = net.Dial("tcp", serverAddr)
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

		l, err := con.Write(buf)
		if err != nil {
			log.Printf("Tunnel write error: %v", err)
			continue
		}
		if l != bufLen {
			log.Printf("Tunnel write error: length differs: %vb of %vb.", l, bufLen)
			continue
		}

		err = handleTunnel(con)
		if err != nil {
			log.Printf("handleTunnel: %v", err)
		}

		//Eventually reset tries
		if time.Since(lastConnect) > ReconResetAfter {
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
	}
	log.Printf("Game ports: %v", portsStr)

	for {
		//Protocol version
		payloadLen, err := binary.ReadUvarint(reader)
		if err != nil {
			return fmt.Errorf("unable to read payload length: %v", err)
		}
		var payload = make([]byte, payloadLen)
		con.Read(payload)

		//Process payload
		log.Printf("Payload: %v", payloadLen)
	}
}
