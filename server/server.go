package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"time"
)

const (
	listenThrottle = time.Second
)

var (
	listenPort, maxClients int
)

func main() {
	flag.Int("listenPort", 30000, "TCP Port to listen for proxy clients on")
	flag.Int("maxClients", 10, "Maximum proxy clients")
	flag.Parse()

	addr := fmt.Sprintf(":%d", listenPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Unable to listen on port %v.", listenPort)
	}
	log.Printf("[START] Server listening on %v.", listenPort)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("[ERR] Accept: %v", err)
			continue
		}
		log.Printf("[CONNECT] From proxy %s", conn.RemoteAddr())
		go handleConnection(conn)

		time.Sleep(listenThrottle)
	}
}

func handleConnection(conn net.Conn) {

}
