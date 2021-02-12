package main

import (
	"crypto/tls"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"syscall"

	"github.com/pion/turn/v2"
)

func main() {
	publicIP := flag.String("public-ip", "", "IP Address that TURN can be contacted by.")
	port := flag.Int("port", 5349, "Listening port.")
	users := flag.String("users", "", "List of username and password (e.g. \"user=pass,user=pass\")")
	realm := flag.String("realm", "pion.ly", "Realm (defaults to \"pion.ly\")")
	certFile := flag.String("cert", "server.crt", "Certificate (defaults to \"server.crt\")")
	keyFile := flag.String("key", "server.key", "Key (defaults to \"server.key\")")
	flag.Parse()

	if len(*publicIP) == 0 {
		log.Fatalf("'public-ip' is required")
	} else if len(*users) == 0 {
		log.Fatalf("'users' is required")
	}

	cer, err := tls.LoadX509KeyPair(*certFile, *keyFile)
	if err != nil {
		log.Println(err)
		return
	}

	// Create a TLS listener to pass into pion/turn
	// pion/turn itself doesn't allocate any TLS listeners, but lets the user pass them in
	// this allows us to add logging, storage or modify inbound/outbound traffic
	tlsListener, err := tls.Listen("tcp4", "0.0.0.0:"+strconv.Itoa(*port), &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cer},
	})
	if err != nil {
		log.Println(err)
		return
	}

	// Cache -users flag for easy lookup later
	// If passwords are stored they should be saved to your DB hashed using turn.GenerateAuthKey
	usersMap := map[string][]byte{}
	for _, kv := range regexp.MustCompile(`(\w+)=(\w+)`).FindAllStringSubmatch(*users, -1) {
		usersMap[kv[1]] = turn.GenerateAuthKey(kv[1], *realm, kv[2])
	}

	s, err := turn.NewServer(turn.ServerConfig{
		Realm: *realm,
		// Set AuthHandler callback
		// This is called everytime a user tries to authenticate with the TURN server
		// Return the key for that user, or false when no user is found
		AuthHandler: func(username string, realm string, srcAddr net.Addr) ([]byte, bool) {
			if key, ok := usersMap[username]; ok {
				return key, true
			}
			return nil, false
		},
		// ListenerConfig is a list of Listeners and the configuration around them
		ListenerConfigs: []turn.ListenerConfig{
			{
				Listener: tlsListener,
				RelayAddressGenerator: &turn.RelayAddressGeneratorStatic{
					RelayAddress: net.ParseIP(*publicIP),
					Address:      "0.0.0.0",
				},
			},
		},
	})
	if err != nil {
		log.Panic(err)
	}

	// Block until user sends SIGINT or SIGTERM
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs

	if err = s.Close(); err != nil {
		log.Panic(err)
	}
}
