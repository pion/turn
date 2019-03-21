package main

import (
	"flag"
	"github.com/pions/stun"
	"github.com/pions/turn"
	"gopkg.in/ini.v1"
	"log"
	"os"
	"regexp"
	"strconv"
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

func getIniConf() (port int, users map[string]string, realm string) {
	config := flag.Lookup("cfg").Value.String()
	log.Printf("Use config %s", config)

	cfg, err := ini.Load(config)

	if err != nil {
		log.Panic("Config file not loaded")
		os.Exit(1)
	}

	port, err = cfg.Section("server").Key("port").Int()

	if err != nil {
		log.Panic("Port is not specified")
		os.Exit(1)
	}

	usersString := cfg.Section("users").Key("users").String()
	realm = cfg.Section("users").Key("realm").String()

	if usersString == "" {
		log.Panic("USERS is a required environment variable")
	}

	usersMap := make(map[string]string)
	for _, kv := range regexp.MustCompile(`(\w+)=(\w+)`).FindAllStringSubmatch(usersString, -1) {
		usersMap[kv[1]] = kv[2]
	}

	return port, usersMap, realm
}

func getEnvConf() (port int, users map[string]string, realm string) {
	log.Printf("Use environment variables")

	usersMap := make(map[string]string)
	usersString := os.Getenv("USERS")
	if usersString == "" {
		log.Panic("USERS is a required environment variable")
	}
	for _, kv := range regexp.MustCompile(`(\w+)=(\w+)`).FindAllStringSubmatch(usersString, -1) {
		usersMap[kv[1]] = kv[2]
	}

	realm = os.Getenv("REALM")
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

	return udpPort, usersMap, realm
}

type config func() (port int, users map[string]string, realm string)

func main() {
	m := &myTurnServer{usersMap: make(map[string]string)}

	var configFn config

	config := flag.String("cfg", "config.ini", "Configuration file")
	flag.Parse()

	if *config == "" {
		configFn = getEnvConf
	} else {
		configFn = getIniConf
	}

	port, users, realm := configFn()

	m.usersMap = users

	log.Printf("Starting on port %d", port)

	turn.Start(turn.StartArguments{
		Server:  m,
		Realm:   realm,
		UDPPort: port,
	})
}
