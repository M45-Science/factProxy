package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	defaultTunnelPort     = 30000
	defaultMaxTunnels     = 100
	defaultTunnelListenMS = 100
	defaultGamePorts      = ""
	defaultCompression    = true
	defaultVerboseLog     = true
)

func main() {
	// Channel to receive OS signals
	sigs := make(chan os.Signal, 1)
	// Notify for SIGINT (Ctrl+C) and SIGTERM
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	var gpstr string
	var tlms int
	flag.IntVar(&tunnelPort, "tunnelPort", defaultTunnelPort, "")
	flag.StringVar(&gpstr, "gamePorts", defaultGamePorts, "comma-separated port list")
	flag.IntVar(&maxTunnels, "maxTunnels", defaultMaxTunnels, "")
	flag.IntVar(&tlms, "tunnelListenThrottleMS", defaultTunnelListenMS, "")
	flag.BoolVar(&useCompression, "useCompression", defaultCompression, "compress tunnel")
	flag.BoolVar(&verboseLog, "verboseLog", defaultVerboseLog, "enable or disable verbose (per-frame) logging")
	flag.Parse()

	parseGamePorts(gpstr)
	parseTunnelListenThrottle(tlms)

	go listenForTunnels()

	// Graceful exit
	<-sigs
	log.Printf("[QUIT] Server shutting down: Signal: %v", sigs)
	closeAllTunnels()
	log.Printf("Goodbye")
}

func parseGamePorts(input string) {
	portStringParts := strings.Split(input, ",")
	gamePorts = []int{}
	for _, portStr := range portStringParts {
		if strings.Contains(portStr, "-") {
			errStr := "gamePorts: unable to parse port range: %v"

			portRangeParts := strings.Split(portStr, "-")
			if len(portRangeParts) != 2 {
				log.Printf(errStr, portStr)
				continue
			}
			lowerStr := portRangeParts[0]
			upperStr := portRangeParts[1]

			var err error
			lowerPort, err := strconv.ParseUint(lowerStr, 10, 64)
			if err != nil {
				log.Printf(errStr, portStr)
			}
			upperPort, err := strconv.ParseUint(upperStr, 10, 64)
			if err != nil {
				log.Printf(errStr, portStr)
			}
			if lowerPort > upperPort {
				lowerPort, upperPort = upperPort, lowerPort
			}
			for port := lowerPort; port < upperPort; port++ {
				gamePorts = append(gamePorts, int(port))
			}
		}
		portNumber, err := strconv.ParseUint(portStr, 10, 64)
		if err != nil {
			log.Printf("gamePorts: unable to parse argument: %v (%v)", portStr, err)
			continue
		}
		gamePorts = append(gamePorts, int(portNumber))
	}
}

func parseTunnelListenThrottle(input int) {
	tunnelListenThrottle = time.Millisecond * time.Duration(input)
}
