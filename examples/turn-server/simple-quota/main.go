// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package main implements a simple TURN server with per-user allocation quotas.
package main

import (
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/pion/turn/v5"
)

const maxAllocationsPerUser = 1

func main() { // nolint:cyclop
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
	udpListener, err := net.ListenPacket("udp4", net.JoinHostPort("0.0.0.0", strconv.Itoa(*port))) // nolint: noctx
	if err != nil {
		log.Panicf("Failed to create TURN server listener: %s", err)
	}

	// Cache -users flag for easy lookup later
	// If passwords are stored they should be saved to your DB hashed using turn.GenerateAuthKey
	usersMap := map[string][]byte{}
	for _, userPass := range strings.Split(*users, ",") {
		parts := strings.SplitN(userPass, "=", 2)
		if len(parts) != 2 {
			log.Fatalf("Invalid user credential format '%s': expected 'username=password'", userPass)
		}
		usersMap[parts[0]] = turn.GenerateAuthKey(parts[0], *realm, parts[1])
	}

	// Track sessions count per user.
	userSessions := map[string]int{}
	userSessionLock := &sync.Mutex{}

	server, err := turn.NewServer(turn.ServerConfig{
		Realm: *realm,

		// Set AuthHandler callback
		// This is called every time a user tries to authenticate with the TURN server
		// Return the key for that user, or false when no user is found
		AuthHandler: func(ra *turn.RequestAttributes) (string, []byte, bool) {
			if key, ok := usersMap[ra.Username]; ok {
				return ra.Username, key, true
			}

			return "", nil, false
		},

		// QuotaHandler allows rejecting allocations when a user exceeds a quota.
		// This example limits concurrent allocations per user.
		QuotaHandler: func(userID, _ string, srcAddr net.Addr) (ok bool) {
			userSessionLock.Lock()
			defer userSessionLock.Unlock()
			if userSessions[userID] < maxAllocationsPerUser {
				return true
			}

			log.Printf("Allocation quota reached for user-id %q from %s", userID, srcAddr.String())

			return false
		},

		// Track allocations as they are Created+Deleted. This ensures that users
		// don't have more then maxAllocationsPerUser at a time
		EventHandler: turn.EventHandler{
			OnAllocationCreated: func(_, _ net.Addr, _, userID, _ string, _ net.Addr, _ int) {
				userSessionLock.Lock()
				defer userSessionLock.Unlock()

				userSessions[userID]++
			},
			OnAllocationDeleted: func(_, _ net.Addr, _, userID, _ string) {
				userSessionLock.Lock()
				defer userSessionLock.Unlock()

				if userSessions[userID] > 0 {
					userSessions[userID]--
				}
			},
		},

		// PacketConnConfigs is a list of UDP Listeners and the configuration around them
		PacketConnConfigs: []turn.PacketConnConfig{
			{
				PacketConn: udpListener,
				RelayAddressGenerator: &turn.RelayAddressGeneratorStatic{
					// Claim that we are listening on IP passed by user (This should be your Public IP)
					RelayAddress: net.ParseIP(*publicIP),
					// But actually be listening on every interface
					Address: "0.0.0.0",
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

	if err = server.Close(); err != nil {
		log.Panic(err)
	}
}
