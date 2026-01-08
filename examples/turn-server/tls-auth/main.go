// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package main implements an example TURN server with TLS certificate-based authentication
package main

import (
	"crypto/md5" // nolint: gosec
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/pion/turn/v4"
)

// getClientTLSAuthHandler returns an AuthHandler that validates client TLS certificates
//
// This handler ensures that the client presents a valid TLS certificate (valid meaning
// that it is compliant with the given x509.VerifyOptions object e.g. signed by a trusted
// CA) for which for which the CommonName must match the TURN request's username attribute.
func getClientTLSAuthHandler(verifyOpts x509.VerifyOptions) turn.AuthHandler {
	return func(ra *turn.RequestAttributes) ([]byte, bool) {
		if ra.TLS == nil || len(ra.TLS.PeerCertificates) == 0 {
			log.Printf("Request not allowed: no TLS state metadata")

			return nil, false
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

			// in this example, we generate the turn auth key based only on the
			// certificate's common name, but you can use any attributes from the
			// certificate's contents and/or request attributes.
			hash := md5.New()                                                      // nolint: gosec
			fmt.Fprint(hash, strings.Join([]string{cert.Subject.CommonName}, ":")) // nolint: errcheck
			key := hash.Sum(nil)

			return key, true
		}

		log.Printf("Request not allowed: no valid certificates found")

		return nil, false
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
	cer, err := tls.LoadX509KeyPair(*certFile, *keyFile)
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

	// Create TLS listener that requires client certificates
	tlsListener, err := tls.Listen("tcp4", "0.0.0.0:"+strconv.Itoa(*port), &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cer},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caCertPool,
	})
	if err != nil {
		log.Fatalf("Failed to create TLS listener: %v", err)
	}

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
