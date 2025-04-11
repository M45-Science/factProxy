package main

import (
	"log"
	"net"
	"time"
)

const (
	routeLife          = time.Minute
	routeCleanInterval = time.Minute * 15
)

var (
	gamePortOffset int
)

type routeData struct {
	clientID          int
	tunnel            *tunnelCon
	ephemeralListener *net.UDPConn

	ephemeralPort int
	gamePort      int

	lastActive time.Time
}

func (con *tunnelCon) routeMapCleaner() {
	for con != nil {
		con.routeMapLock.Lock()
		for r, route := range con.IDMap {
			if time.Since(route.lastActive) > routeLife {
				delete(con.IDMap, r)
			}
		}
		con.routeMapLock.Unlock()

		time.Sleep(routeCleanInterval)
	}
}

func (con *tunnelCon) addRoute(route *routeData) {
	con.routeMapLock.Lock()
	defer con.routeMapLock.Unlock()

	con.routeTop++
	route.lastActive = time.Now()
	con.IDMap[con.routeTop] = route
	con.EphemeralMap[route.ephemeralPort] = route
}

// Automatically creates route if it does not exist
func (con *tunnelCon) lookupRoute(routeID int, create bool) *routeData {
	con.routeMapLock.Lock()
	defer con.routeMapLock.Unlock()

	route := con.IDMap[routeID]
	if route == nil {
		if create {
			newRoute := &routeData{clientID: routeID, tunnel: con, lastActive: time.Now()}
			newRoute.Birth()
		} else {
			log.Printf("lookupRoute: Route ID not found: %v", routeID)
			return nil
		}
	}
	route.lastActive = time.Now()
	return route
}

func (route *routeData) Birth() {
	laddr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0}
	conn, _ := net.ListenUDP("udp", laddr)

	route.ephemeralListener = conn
	route.ephemeralPort = laddr.Port
}
