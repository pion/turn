// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package main implements a TURN client with support for TCP
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"strings"

	"github.com/pion/logging"
	"github.com/pion/turn/v3"
)

func setupSignalingChannel(addrCh chan string, signaling bool, relayAddr string) {
	addr := "127.0.0.1:5000"
	if signaling {
		go func() {
			listener, err := net.Listen("tcp", addr)
			if err != nil {
				log.Panicf("Failed to create signaling server: %s", err)
			}
			defer listener.Close() //nolint:errcheck,gosec
			for {
				conn, err := listener.Accept()
				if err != nil {
					log.Panicf("Failed to accept: %s", err)
				}

				go func() {
					var message string
					message, err = bufio.NewReader(conn).ReadString('\n')
					if err != nil {
						log.Panicf("Failed to read from relayAddr: %s", err)
					}
					addrCh <- message[:len(message)-1]
				}()

				if _, err = conn.Write([]byte(fmt.Sprintf("%s\n", relayAddr))); err != nil {
					log.Panicf("Failed to write relayAddr: %s", err)
				}
			}
		}()
	} else {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			log.Panicf("Error dialing: %s", err)
		}
		message, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			log.Panicf("Failed to read relayAddr: %s", err)
		}
		addrCh <- message[:len(message)-1]
		if _, err = conn.Write([]byte(fmt.Sprintf("%s\n", relayAddr))); err != nil {
			log.Panicf("Failed to write relayAddr: %s", err)
		}
	}
}

func main() {
	host := flag.String("host", "", "TURN Server name.")
	port := flag.Int("port", 3478, "Listening port.")
	user := flag.String("user", "", "A pair of username and password (e.g. \"user=pass\")")
	realm := flag.String("realm", "pion.ly", "Realm (defaults to \"pion.ly\")")
	signaling := flag.Bool("signaling", false, "Whether to start signaling server otherwise connect")

	flag.Parse()

	if len(*host) == 0 {
		log.Panicf("'host' is required")
	}

	if len(*user) == 0 {
		log.Panicf("'user' is required")
	}

	// Dial TURN Server
	turnServerAddrStr := fmt.Sprintf("%s:%d", *host, *port)

	turnServerAddr, err := net.ResolveTCPAddr("tcp", turnServerAddrStr)
	if err != nil {
		log.Panicf("Failed to resolve TURN server address: %s", err)
	}

	conn, err := net.DialTCP("tcp", nil, turnServerAddr)
	if err != nil {
		log.Panicf("Failed to connect to TURN server: %s", err)
	}

	cred := strings.SplitN(*user, "=", 2)

	// Start a new TURN Client and wrap our net.Conn in a STUNConn
	// This allows us to simulate datagram based communication over a net.Conn
	cfg := &turn.ClientConfig{
		STUNServerAddr: turnServerAddrStr,
		TURNServerAddr: turnServerAddrStr,
		Conn:           turn.NewSTUNConn(conn),
		Username:       cred[0],
		Password:       cred[1],
		Realm:          *realm,
		LoggerFactory:  logging.NewDefaultLoggerFactory(),
	}

	client, err := turn.NewClient(cfg)
	if err != nil {
		log.Panicf("Failed to create TURN client: %s", err)
	}
	defer client.Close()

	// Start listening on the conn provided.
	err = client.Listen()
	if err != nil {
		log.Panicf("Failed to listen: %s", err)
	}

	// Allocate a relay socket on the TURN server. On success, it
	// will return a client.TCPAllocation which represents the remote
	// socket.
	allocation, err := client.AllocateTCP()
	if err != nil {
		log.Panicf("Failed to allocate: %s", err)
	}
	defer func() {
		if closeErr := allocation.Close(); closeErr != nil {
			log.Panicf("Failed to close connection: %s", closeErr)
		}
	}()

	log.Printf("relayed-address=%s", allocation.Addr())

	// Learn the peers relay address via signaling channel
	addrCh := make(chan string, 5)
	setupSignalingChannel(addrCh, *signaling, allocation.Addr().String())

	// Get peer address
	peerAddrStr := <-addrCh
	peerAddr, err := net.ResolveTCPAddr("tcp", peerAddrStr)
	if err != nil {
		log.Panicf("Failed to resolve peer address: %s", err)
	}

	log.Printf("Received peer address: %s", peerAddrStr)

	buf := make([]byte, 4096)
	var n int
	if *signaling {
		conn, err := allocation.DialTCP("tcp", nil, peerAddr)
		if err != nil {
			log.Panicf("Failed to dial: %s", err)
		}

		if _, err = conn.Write([]byte("hello!")); err != nil {
			log.Panicf("Failed to write: %s", err)
		}

		n, err = conn.Read(buf)
		if err != nil {
			log.Panicf("Failed to read from relay connection: %s", err)
		}

		if err := conn.Close(); err != nil {
			log.Panicf("Failed to close: %s", err)
		}
	} else {
		if err := client.CreatePermission(peerAddr); err != nil {
			log.Panicf("Failed to create permission: %s", err)
		}

		conn, err := allocation.AcceptTCP()
		if err != nil {
			log.Panicf("Failed to accept TCP connection: %s", err)
		}

		log.Printf("Accepted connection from: %s", conn.RemoteAddr())

		n, err = conn.Read(buf)
		if err != nil {
			log.Panicf("Failed to read from relay conn: %s", err)
		}

		if _, err := conn.Write([]byte("hello back!")); err != nil {
			log.Panicf("Failed to write: %s", err)
		}

		if err := conn.Close(); err != nil {
			log.Panicf("Failed to close: %s", err)
		}
	}

	log.Printf("Read message: %s", string(buf[:n]))
}
