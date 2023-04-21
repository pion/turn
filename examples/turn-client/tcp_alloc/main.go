// Package main implements a TURN client with support for TCP
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"

	"github.com/pion/logging"
	"github.com/pion/turn/v2"
)

func setupSignalingChannel(addrCh chan string, signaling bool, relayAddr string) {
	addr := "127.0.0.1:5000"
	if signaling {
		go func() {
			listen, err := net.Listen("tcp", addr)
			if err != nil {
				log.Panicf("Failed to create signaling server: %s", err)
			}
			defer listen.Close()
			for {
				conn, err := listen.Accept()
				if err != nil {
					log.Panicf("Failed to accept: %s", err)
				}
				go func() {
					message, err := bufio.NewReader(conn).ReadString('\n')
					if err != nil {
						log.Panicf("Failed to read relayAddr: %s", err)
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
		log.Fatalf("'host' is required")
	}

	if len(*user) == 0 {
		log.Fatalf("'user' is required")
	}

	// Dial TURN Server
	turnServerAddr := fmt.Sprintf("%s:%d", *host, *port)
	conn, err := net.Dial("tcp", turnServerAddr)
	if err != nil {
		log.Panicf("Failed to connect to TURN server: %s", err)
	}

	cred := strings.SplitN(*user, "=", 2)

	// Start a new TURN Client and wrap our net.Conn in a STUNConn
	// This allows us to simulate datagram based communication over a net.Conn
	cfg := &turn.ClientConfig{
		STUNServerAddr: turnServerAddr,
		TURNServerAddr: turnServerAddr,
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
			log.Fatalf("Failed to close connection: %s", closeErr)
		}
	}()

	log.Printf("relayed-address=%s", allocation.Addr().String())

	// Learn the peers relay address via signaling channel
	addrCh := make(chan string, 5)
	setupSignalingChannel(addrCh, *signaling, allocation.Addr().String())

	// Get peer address
	peerAddrString := <-addrCh
	res := strings.Split(peerAddrString, ":")
	peerIp := res[0]
	peerPort, _ := strconv.Atoi(res[1])

	log.Printf("Recieved peer address: %s", peerAddrString)

	buf := make([]byte, 4096)
	peerAddr := net.TCPAddr{IP: net.ParseIP(peerIp), Port: peerPort}
	var n int
	if *signaling {
		conn, err = allocation.Dial("tcp", peerAddrString)
		if err != nil {
			fmt.Println("Error connecting:", err)
		}
		conn.Write([]byte("hello!"))
		n, err = conn.Read(buf)
		if err != nil {
			log.Println("Error reading from relay conn:", err)
		}
		conn.Close()
	} else {
		client.CreatePermission(&peerAddr)
		conn, err := allocation.AcceptTCP()
		if err != nil {
			log.Println("Error accepting:", err)
		}
		log.Println("Accepted from:", conn.RemoteAddr())
		n, err = conn.Read(buf)
		if err != nil {
			log.Println("Error reading from relay conn:", err)
		}
		conn.Write([]byte("hello back!"))
		conn.Close()
	}
	log.Println("Read message:", string(buf[:n]))
}
