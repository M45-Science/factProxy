package main

import (
	"fmt"
	"os"
)

func handleGameListens() {
	for _, port := range listeners {
		buf := make([]byte, 2048)
		for {
			n, addr, err := port.ReadFromUDP(buf)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Read error on %v: %v\n", port.LocalAddr(), err)
				return
			}
			fmt.Printf("Received %d bytes from %v on %v: %s\n", n, addr, port.LocalAddr(), string(buf[:n]))
		}
	}
}
