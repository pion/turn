// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package main implements a simple TURN server with IPv6 support (RFC 6156)
package main

import (
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/pion/turn/v4"
)

func main() { //nolint:gocyclo,cyclop
	publicIP := flag.String("public-ip", "", "IPv6 Address that TURN can be contacted by.")
	port := flag.Int("port", 3478, "Listening port.")
	users := flag.String("users", "", "List of username and password (e.g. \"user=pass,user=pass\")")
	realm := flag.String("realm", "pion.ly", "Realm (defaults to \"pion.ly\")")
	flag.Parse()

	if len(*publicIP) == 0 {
		log.Fatalf("'public-ip' is required")
	} else if len(*users) == 0 {
		log.Fatalf("'users' is required")
	}

	// Validate that the public IP is IPv6
	parsedIP := net.ParseIP(*publicIP)
	if parsedIP == nil || parsedIP.To4() != nil {
		log.Fatalf("'public-ip' must be a valid IPv6 address")
	}

	// Create a UDP listener to pass into pion/turn
	// Using udp6 for IPv6 support
	udpListener, err := net.ListenPacket("udp6", net.JoinHostPort("::", strconv.Itoa(*port))) // nolint: noctx
	if err != nil {
		log.Panicf("Failed to create TURN server listener: %s", err)
	}

	// Cache -users flag for easy lookup later
	// If passwords are stored they should be saved to your DB hashed using turn.GenerateAuthKey
	usersMap := map[string][]byte{}
	for _, userPass := range strings.Split(*users, ",") {
		parts := strings.SplitN(userPass, "=", 2)
		if len(parts) == 2 {
			usersMap[parts[0]] = turn.GenerateAuthKey(parts[0], *realm, parts[1])
		}
	}

	server, err := turn.NewServer(turn.ServerConfig{
		Realm: *realm,
		// Set AuthHandler callback
		// This is called every time a user tries to authenticate with the TURN server
		// Return the key for that user, or false when no user is found
		AuthHandler: func(ra *turn.RequestAttributes) ([]byte, bool) {
			if key, ok := usersMap[ra.Username]; ok {
				return key, true
			}

			return nil, false
		},
		// PacketConnConfigs is a list of UDP Listeners and the configuration around them
		PacketConnConfigs: []turn.PacketConnConfig{
			{
				PacketConn: udpListener,
				RelayAddressGenerator: &turn.RelayAddressGeneratorStatic{
					// Claim that we are listening on IPv6 address passed by user
					RelayAddress: parsedIP,
					// Listen on all IPv6 interfaces
					Address: "::",
				},
			},
		},
	})
	if err != nil {
		log.Panic(err)
	}

	log.Printf("TURN server listening on [::]:%d with IPv6 support (RFC 6156)", *port)
	log.Printf("Public IPv6 address: %s", *publicIP)

	// Block until user sends SIGINT or SIGTERM
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs

	if err = server.Close(); err != nil {
		log.Panic(err)
	}
}
