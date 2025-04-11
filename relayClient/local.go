package main

import (
	"fmt"
	"net"
)

func handleGameListens() {
	for _, port := range listeners {
		go func(p *net.UDPConn) {
			buf := make([]byte, 65536)
			for {
				n, addr, err := p.ReadFromUDP(buf)
				if err != nil {
					//fmt.Fprintf(os.Stderr, "Read error on %v: %v\n", p.LocalAddr(), err)
					return
				}
				fmt.Printf("Received %d bytes from %v on %v: %s\n", n, addr, p.LocalAddr(), string(buf[:n]))
			}
		}(port)
	}
}
