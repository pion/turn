// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package main implements an example TURN server with TLS certificate-based authentication
package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/pion/turn/v4"
)

// getClientTLSAuthHandler returns an AuthHandler that validates client TLS certificates
//
// This handler ensures that the client presents at least one valid TLS certificate
// (valid meaning that it is compliant with the given x509.VerifyOptions object e.g.
// signed by a trusted CA) for which for which the CommonName must match the TURN
// request's username attribute.
//
// Using the CommonName as the username is just what we chose to do in this example.
// In your own code, you may choose to use some other uniquely-identifying property
// in the certificate e.g. serial number or combination of SANs.
func getClientTLSAuthHandler(verifyOpts x509.VerifyOptions) turn.AuthHandler {
	return func(ra *turn.RequestAttributes) (string, []byte, bool) {
		if ra.TLS == nil || len(ra.TLS.PeerCertificates) == 0 {
			log.Printf("Request not allowed: no TLS state metadata")

			return "", nil, false
		}

		for _, cert := range ra.TLS.PeerCertificates {
			if cert.Subject.CommonName != ra.Username {
				log.Printf("Certificate CN %q does not match username %q", cert.Subject.CommonName, ra.Username)

				continue
			}

			if _, err := cert.Verify(verifyOpts); err != nil {
				log.Printf("Certificate validation failed: %v", err)

				continue
			}

			log.Printf("Certificate validated for username %q", ra.Username)

			// Note the empty password for certificate-based auth
			return ra.Username, turn.GenerateAuthKey(ra.Username, ra.Realm, ""), true
		}

		log.Printf("Request not allowed: no valid certificates found")

		return "", nil, false
	}
}

func main() {
	publicIP := flag.String("public-ip", "", "IP Address that TURN can be contacted by.")
	port := flag.Int("port", 5349, "Listening port.")
	realm := flag.String("realm", "pion.ly", "Realm (defaults to \"pion.ly\")")
	certFile := flag.String("cert", "server.crt", "Server certificate (defaults to \"server.crt\")")
	keyFile := flag.String("key", "server.key", "Server key (defaults to \"server.key\")")
	caFile := flag.String("ca", "ca.crt", "CA certificate for client verification (defaults to \"ca.crt\")")
	flag.Parse()

	if len(*publicIP) == 0 {
		log.Fatalf("'public-ip' is required")
	}

	// Load server certificate
	serverCert, err := tls.LoadX509KeyPair(*certFile, *keyFile)
	if err != nil {
		log.Fatalf("Failed to load server certificate: %v", err)
	}

	// Load CA certificate for client verification
	caCert, err := os.ReadFile(*caFile)
	if err != nil {
		log.Fatalf("Failed to read CA certificate: %v", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		log.Fatalf("Failed to parse CA certificate")
	}

	// Create TLS config for listener to require client certificates.
	tlsConfig := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{serverCert},

		// NOTE: The setting below is very important. Make sure
		// the value you choose suits your use case well:
		//
		// - tls.RequestClientCert indicates that a client certificate should be requested
		// during the handshake, but does not require that the client send any
		// certificates.
		//
		// - tls.RequireAnyClientCert indicates that a client certificate should be requested
		// during the handshake, and that at least one certificate is required to be
		// sent by the client, but that certificate is not required to be valid.
		//
		// - tls.VerifyClientCertIfGiven indicates that a client certificate should be requested
		// during the handshake, but does not require that the client sends a
		// certificate. If the client does send a certificate it is required to be
		// valid.
		//
		// - tls.RequireAndVerifyClientCert indicates that a client certificate should be requested
		// during the handshake, and that at least one valid certificate is required
		// to be sent by the client.
		ClientAuth: tls.RequireAnyClientCert,
		ClientCAs:  caCertPool,
	}

	// Listen on all IPv4 interfaces.
	tlsListener, err := tls.Listen("tcp4", net.JoinHostPort("0.0.0.0", strconv.Itoa(*port)), tlsConfig)
	if err != nil {
		log.Fatalf("Failed to create TLS listener: %v", err)
	}

	log.Printf("TURN Server listening on 0.0.0.0:%d with TLS client certificate authentication", *port)
	log.Printf("Realm: %s", *realm)
	log.Printf("Relay IP: %s", *publicIP)

	server, err := turn.NewServer(turn.ServerConfig{
		Realm:       *realm,
		AuthHandler: getClientTLSAuthHandler(x509.VerifyOptions{Roots: caCertPool}),
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
		log.Fatalf("Failed to create TURN server: %v", err)
	}

	// Block until user sends SIGINT or SIGTERM
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs

	if err = server.Close(); err != nil {
		log.Panic(err)
	}
}
