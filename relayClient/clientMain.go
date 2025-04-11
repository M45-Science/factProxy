package main

import (
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	SERVER_KEY = 0xCAFE69C0FFEE
	CLIENT_KEY = 0xADD069C0FFEE
)

const (
	protocolVersion = 1

	reconDelaySec   = 5
	maxAttempts     = 2 * (60 / reconDelaySec)
	reconResetAfter = time.Minute * 5
)

var (
	serverID         int
	tunnelServerAddr string
	tunnelCon        net.Conn
)

var (
	gamePorts []int
	listeners []*net.UDPConn
)

func main() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	flag.StringVar(&tunnelServerAddr, "server", "m45sci.xyz:30000", "server:port")
	flag.BoolVar(&forceHTML, "openList", false, "Write "+htmlFileName+" and then attempt to open it.")
	flag.Parse()

	go connectHandler()

	<-sigs
	log.Printf("[QUIT] Server shutting down: Signal: %v", sigs)
	if tunnelCon != nil {
		tunnelCon.Close()
	}
	log.Printf("Goodbye")
}
