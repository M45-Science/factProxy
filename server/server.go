package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	defaultTunnelPort     = 30000
	defaultMaxTunnels     = 100
	defaultTunnelListenMS = 1000
	defaultCompression    = true
)

func main() {
	// Channel to receive OS signals
	sigs := make(chan os.Signal, 1)
	// Notify for SIGINT (Ctrl+C) and SIGTERM
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	flag.IntVar(&tunnelPort, "tunnelPort", defaultTunnelPort, "")
	flag.IntVar(&maxTunnels, "maxTunnels", defaultMaxTunnels, "")
	flag.IntVar(&tunnelListenMS, "tunnelListenThrottleMS", defaultTunnelListenMS, "")
	flag.BoolVar(&useCompression, "useCompression", defaultCompression, "compress tunnel")
	flag.Parse()
	tunnelListenThrottle = time.Millisecond * time.Duration(tunnelListenMS)

	go listenForTunnels()

	// Graceful exit
	<-sigs
	log.Printf("[QUIT] Server shutting down: Signal: %v", sigs)
	closeAllTunnels()
	log.Printf("Goodbye")
}
