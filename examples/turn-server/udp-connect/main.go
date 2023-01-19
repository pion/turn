// Package main implements a performance optimized TURN server that uses client-specific connected
// sockets using SO_REUSEPORT.
package main

import (
	"context"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"syscall"

	"github.com/pion/logging"
	"github.com/pion/turn/v2"
)

func main() {
	publicIP := flag.String("public-ip", "", "IP Address that TURN can be contacted by.")
	port := flag.Int("port", 3478, "Listening port.")
	users := flag.String("users", "", "List of username and password (e.g. \"user=pass,user=pass\")")
	realm := flag.String("realm", "pion.ly", "Realm (defaults to \"pion.ly\")")
	flag.Parse()

	if len(*publicIP) == 0 {
		log.Fatalf("'public-ip' is required")
	} else if len(*users) == 0 {
		log.Fatalf("'users' is required")
	}

	// Create a UDP listener to pass into pion/turn
	// pion/turn itself doesn't allocate any UDP sockets, but lets the user pass them in
	// this allows us to add logging, storage or modify inbound/outbound traffic
	listenerConfig := &net.ListenConfig{Control: reuseAddr}
	udpListener, err := listenerConfig.ListenPacket(context.Background(), "udp4",
		"0.0.0.0:"+strconv.Itoa(*port))
	if err != nil {
		log.Panicf("Failed to create TURN server listener: %s", err)
	}

	// Cache -users flag for easy lookup later
	// If passwords are stored they should be saved to your DB hashed using turn.GenerateAuthKey
	usersMap := map[string][]byte{}
	for _, kv := range regexp.MustCompile(`(\w+)=(\w+)`).FindAllStringSubmatch(*users, -1) {
		usersMap[kv[1]] = turn.GenerateAuthKey(kv[1], *realm, kv[2])
	}

	logger := logging.NewDefaultLoggerFactory()
	// uncomment this to raise loglevel
	// logger.DefaultLogLevel = logging.LogLevelTrace

	s, err := turn.NewServer(turn.ServerConfig{
		Realm: *realm,
		// Set AuthHandler callback
		// This is called every time a user tries to authenticate with the TURN server
		// Return the key for that user, or false when no user is found
		AuthHandler: func(username string, realm string, srcAddr net.Addr) ([]byte, bool) {
			if key, ok := usersMap[username]; ok {
				return key, true
			}
			return nil, false
		},
		// PacketConnConfigs is a list of UDP Listeners and the configuration around them
		PacketConnConfigs: []turn.PacketConnConfig{
			{
				PacketConn: udpListener,
				Connect: func(conn net.PacketConn, remoteAddr net.Addr) (net.PacketConn, error) {
					log.Printf("connecting back to client %q", remoteAddr.String())
					dialer := &net.Dialer{LocalAddr: conn.LocalAddr(), Control: reuseAddr}
					newConn, err := dialer.Dial(remoteAddr.Network(), remoteAddr.String())
					if err != nil {
						return nil, err
					}
					return turn.NewWrapConn(newConn, conn.LocalAddr()), nil
				},
				RelayAddressGenerator: &turn.RelayAddressGeneratorStatic{
					RelayAddress: net.ParseIP(*publicIP), // Claim that we are listening on IP passed by user (This should be your Public IP)
					Address:      "0.0.0.0",              // But actually be listening on every interface
				},
			},
		},
		LoggerFactory: logger,
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

func reuseAddr(network, address string, conn syscall.RawConn) error {
	return conn.Control(func(descriptor uintptr) {
		_ = syscall.SetsockoptInt(int(descriptor), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
		// syscall.SetsockoptInt(int(descriptor), syscall.SOL_SOCKET, syscall.SO_REUSEPORT, 1)
	})
}
