package main

import (
	"log"
	"os"
	"regexp"
	"strconv"

	"github.com/pions/pkg/stun"
	"github.com/pions/turn"
)

type myTurnServer struct {
	usersMap map[string]string
}

func (m *myTurnServer) AuthenticateRequest(username string, srcAddr *stun.TransportAddr) (password string, ok bool) {
	if password, ok := m.usersMap[username]; ok {
		return password, true
	}
	return "", false
}

func main() {
	m := &myTurnServer{usersMap: make(map[string]string)}

	users := os.Getenv("USERS")
	if users == "" {
		log.Panic("USERS is a required environment variable")
	}
	for _, kv := range regexp.MustCompile(`(\w+)=(\w+)`).FindAllStringSubmatch(users, -1) {
		m.usersMap[kv[1]] = kv[2]
	}

	realm := os.Getenv("REALM")
	if realm == "" {
		log.Panic("REALM is a required environment variable")
	}

	udpPortStr := os.Getenv("UDP_PORT")
	if udpPortStr == "" {
		log.Panic("UDP_PORT is a required environment variable")
	}
	udpPort, err := strconv.Atoi(udpPortStr)
	if err != nil {
		log.Panic(err)
	}

	turn.Start(turn.StartArguments{
		Server:  m,
		Realm:   realm,
		UDPPort: udpPort,
	})
}
