// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package main implements a TURN client with TLS certificate-based authentication
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/pion/logging"
	"github.com/pion/turn/v4"
)

func main() { //nolint:cyclop
	host := flag.String("host", "", "TURN Server name.")
	port := flag.Int("port", 5349, "Listening port.")
	user := flag.String("user", "", "Username (must match client certificate CN)")
	realm := flag.String("realm", "pion.ly", "Realm (defaults to \"pion.ly\")")
	certFile := flag.String("cert", "client.crt", "Client certificate (defaults to \"client.crt\")")
	keyFile := flag.String("key", "client.key", "Client key (defaults to \"client.key\")")
	caFile := flag.String("ca", "ca.crt", "CA certificate for server verification (defaults to \"ca.crt\")")
	insecure := flag.Bool("insecure", false, "Skip server certificate verification")
	ping := flag.Bool("ping", false, "Run ping test")
	flag.Parse()

	if len(*host) == 0 {
		log.Fatalf("'host' is required")
	}

	if len(*user) == 0 {
		log.Fatalf("'user' is required")
	}

	// Load client certificate
	cert, err := tls.LoadX509KeyPair(*certFile, *keyFile)
	if err != nil {
		log.Fatalf("Failed to load client certificate: %v", err)
	}

	// Configure TLS
	tlsConfig := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
	}

	// Load CA certificate for server verification unless insecure mode is enabled
	if *insecure {
		log.Println("Warning: Server certificate verification is disabled")
		tlsConfig.InsecureSkipVerify = true
	} else {
		var caCert []byte
		caCert, err = os.ReadFile(*caFile)
		if err != nil {
			log.Fatalf("Failed to read CA certificate: %v", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			log.Fatalf("Failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}

	// Dial TURN Server with TLS
	turnServerAddr := net.JoinHostPort(*host, fmt.Sprintf("%d", *port))
	dialer := &tls.Dialer{Config: tlsConfig}
	conn, err := dialer.DialContext(context.Background(), "tcp", turnServerAddr)
	if err != nil {
		log.Panicf("Failed to connect to TURN server: %s", err)
	}

	// For TLS certificate-based authentication, the certificate itself provides
	// authentication. We use an empty password, and the auth key is generated as
	// MD5(username:realm:"") on both client and server sides.
	//
	// Note: this code could very well extract the username from the certificate's
	// CN instead of explicitly asking for it via the flag, but it's not done for
	// the sake of simplicity. Also, using the CN for a username is just what we
	// chose to do in this example. In your own code, you may choose to use some
	// other uniquely-identifying property in the certificate e.g. serial number
	// or combination of SANs.
	cfg := &turn.ClientConfig{
		STUNServerAddr: turnServerAddr,
		TURNServerAddr: turnServerAddr,
		Conn:           turn.NewSTUNConn(conn),
		Username:       *user,
		Password:       "", // Empty password for certificate-based auth
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
	// will return a net.PacketConn which represents the remote
	// socket.
	relayConn, err := client.Allocate()
	if err != nil {
		log.Panicf("Failed to allocate: %s", err)
	}
	defer func() {
		if closeErr := relayConn.Close(); closeErr != nil {
			log.Fatalf("Failed to close connection: %s", closeErr)
		}
	}()

	// The relayConn's local address is actually the transport
	// address assigned on the TURN server.
	log.Printf("relayed-address=%s", relayConn.LocalAddr().String())

	// If you provided `-ping`, perform a ping test against the
	// relayConn we have just allocated.
	if *ping {
		err = doPingTest(client, relayConn)
		if err != nil {
			log.Panicf("Failed to ping: %s", err)
		}
	}
}

func doPingTest(client *turn.Client, relayConn net.PacketConn) error { //nolint:cyclop
	// Send BindingRequest to learn our external IP
	mappedAddr, err := client.SendBindingRequest()
	if err != nil {
		return err
	}

	// Set up pinger socket (pingerConn)
	pingerConn, err := net.ListenPacket("udp4", "0.0.0.0:0") // nolint: noctx
	if err != nil {
		log.Panicf("Failed to listen: %s", err)
	}
	defer func() {
		if closeErr := pingerConn.Close(); closeErr != nil {
			log.Panicf("Failed to close connection: %s", closeErr)
		}
	}()

	// Punch a UDP hole for the relayConn by sending a data to the mappedAddr.
	// This will trigger a TURN client to generate a permission request to the
	// TURN server. After this, packets from the IP address will be accepted by
	// the TURN server.
	_, err = relayConn.WriteTo([]byte("Hello"), mappedAddr)
	if err != nil {
		return err
	}

	// Start read-loop on pingerConn
	go func() {
		buf := make([]byte, 1600)
		for {
			n, from, pingerErr := pingerConn.ReadFrom(buf)
			if pingerErr != nil {
				break
			}

			msg := string(buf[:n])
			if sentAt, pingerErr := time.Parse(time.RFC3339Nano, msg); pingerErr == nil {
				rtt := time.Since(sentAt)
				log.Printf("%d bytes from from %s time=%d ms\n", n, from.String(), int(rtt.Seconds()*1000))
			}
		}
	}()

	// Start read-loop on relayConn
	go func() {
		buf := make([]byte, 1600)
		for {
			n, from, readerErr := relayConn.ReadFrom(buf)
			if readerErr != nil {
				break
			}

			// Echo back
			if _, readerErr = relayConn.WriteTo(buf[:n], from); readerErr != nil {
				break
			}
		}
	}()

	time.Sleep(500 * time.Millisecond)

	// Send 10 packets from relayConn to the echo server
	for i := 0; i < 10; i++ {
		msg := time.Now().Format(time.RFC3339Nano)
		_, err = pingerConn.WriteTo([]byte(msg), relayConn.LocalAddr())
		if err != nil {
			return err
		}

		// For simplicity, this example does not wait for the pong (reply).
		// Instead, sleep 1 second.
		time.Sleep(time.Second)
	}

	return nil
}
