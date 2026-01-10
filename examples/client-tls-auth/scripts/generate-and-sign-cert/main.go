// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package main implements a script to generate client and server certificates
// and sign them with a given certificate authority (CA) certificate.
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"os"
	"strings"
	"time"
)

var (
	errCACertPEMDecode = errors.New("failed to decode CA certificate PEM")
	errCAKeyPEMDecode  = errors.New("failed to decode CA private key PEM")
)

func generateAndSignCert(
	randReader io.Reader,
	commonName string,
	organization string,
	certLifetime time.Duration,
	caCert *x509.Certificate,
	caKey *rsa.PrivateKey,
	certFile string,
	keyFile string,
	ipAddrs []net.IP,
	dnsNames []string,
) error {
	// Generate 2048-bit RSA key-pair
	key, err := rsa.GenerateKey(randReader, 2048)
	if err != nil {
		return fmt.Errorf("failed to generate key pair: %w", err)
	}

	// Encode key as PEM
	keyBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	// Generate random serial number
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(randReader, serialNumberLimit)
	if err != nil {
		return fmt.Errorf("failed to generate serial number: %w", err)
	}

	// Define the certificate template
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   commonName,
			Country:      []string{"US"},
			Organization: []string{organization},
		},
		NotBefore:             time.Now().Add(time.Minute * -5), // valid from 5 minutes ago (allows for minor clock skews)
		NotAfter:              time.Now().Add(certLifetime),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	if len(ipAddrs) > 0 {
		template.IPAddresses = ipAddrs
	}
	if len(dnsNames) > 0 {
		template.DNSNames = dnsNames
	}

	// Create certificate signed by CA
	certData, err := x509.CreateCertificate(randReader, &template, caCert, key.Public(), caKey)
	if err != nil {
		return fmt.Errorf("failed to create certificate: %w", err)
	}

	// Encode certificate as PEM
	certBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certData,
	})

	// Write key file
	if err := os.WriteFile(keyFile, keyBytes, 0o600); err != nil {
		return fmt.Errorf("failed to write key file: %w", err)
	}

	// Write cert file
	if err := os.WriteFile(certFile, certBytes, 0o600); err != nil {
		return fmt.Errorf("failed to write certificate file: %w", err)
	}

	return nil
}

func ensureRequiredStringFlags(flags map[string]string) {
	for name, value := range flags {
		if value == "" {
			log.Fatalf("Required flag %s is missing", name)
		}
	}
}

func loadCACertificate(caCertFile string) (*x509.Certificate, error) {
	caCertPEM, err := os.ReadFile(caCertFile) // #nosec G304 (script has to read the user-specified file)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %w", err)
	}

	caCertBlock, _ := pem.Decode(caCertPEM)
	if caCertBlock == nil {
		return nil, errCACertPEMDecode
	}

	caCert, err := x509.ParseCertificate(caCertBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CA certificate: %w", err)
	}

	return caCert, nil
}

func loadCAPrivateKey(caKeyFile string) (*rsa.PrivateKey, error) {
	caKeyPEM, err := os.ReadFile(caKeyFile) // #nosec G304 (script has to read the user-specified file)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA private key: %w", err)
	}

	caKeyBlock, _ := pem.Decode(caKeyPEM)
	if caKeyBlock == nil {
		return nil, errCAKeyPEMDecode
	}

	caKey, err := x509.ParsePKCS1PrivateKey(caKeyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CA private key: %w", err)
	}

	return caKey, nil
}

func main() {
	// Parse input flags
	certOrg := flag.String("cert-org", "", "Organization for certificate (required)")
	certCommonName := flag.String("cert-cn", "", "Common name (CN) for certificate (required)")
	certIPs := flag.String("cert-ips", "", "IP addresses to be included in SAN section of certificate (defaults to no IPs)")      // nolint:lll
	certSANs := flag.String("cert-sans", "", "DNS names to be included in SAN section of certificate (defaults to no DNS names)") // nolint:lll
	certLifetimeSeconds := flag.Int("cert-lifetime-seconds", 60*60*12, "Certificate lifetime in seconds (defaults to 12 hours)")  // nolint:lll
	certFile := flag.String("cert-out", "", "Output file-path for certificate (required)")
	certKeyFile := flag.String("key-out", "", "Output file-path for certificate private key (required)")
	caCertFile := flag.String("ca-cert", "", "CA certificate file (required)")
	caCertKeyFile := flag.String("ca-key", "", "CA certificate private key file (required)")
	flag.Parse()

	// Validate and format flags
	ensureRequiredStringFlags(map[string]string{
		"-cert-org": *certOrg,
		"-cert-cn":  *certCommonName,
		"-cert-out": *certFile,
		"-key-out":  *certKeyFile,
		"-ca-cert":  *caCertFile,
		"-ca-key":   *caCertKeyFile,
	})

	if *certLifetimeSeconds <= 0 {
		log.Fatal("Required flag -cert-lifetime-seconds must be greater than 0")
	}
	certLifetime := time.Duration(*certLifetimeSeconds) * time.Second

	var ipAddrs []net.IP
	if len(*certIPs) > 0 {
		for i, ip := range strings.Split(*certIPs, ",") {
			parsedIP := net.ParseIP(strings.TrimSpace(ip))
			if parsedIP == nil {
				log.Fatalf("IP address entry \"%s\" (index %d) in flag -ips is not a valid IP address", ip, i)
			}
			ipAddrs = append(ipAddrs, parsedIP)
		}
	}

	var dnsNames []string
	if len(*certSANs) > 0 {
		for _, dnsName := range strings.Split(*certSANs, ",") {
			dnsNames = append(dnsNames, strings.TrimSpace(dnsName))
		}
	}

	// Load CA certificate
	caCert, err := loadCACertificate(*caCertFile)
	if err != nil {
		log.Fatalf("Failed to load CA certificate: %v", err)
	}

	// Load CA private key
	caKey, err := loadCAPrivateKey(*caCertKeyFile)
	if err != nil {
		log.Fatalf("Failed to load CA private key: %v", err)
	}

	// Generate certificate
	if err := generateAndSignCert(
		rand.Reader,
		*certCommonName,
		*certOrg,
		certLifetime,
		caCert,
		caKey,
		*certFile,
		*certKeyFile,
		ipAddrs,
		dnsNames,
	); err != nil {
		log.Fatalf("Failed to generate certificate: %v", err)
	}

	log.Printf("✅ Certificate and private key written: %s, %s", *certFile, *certKeyFile)
}
