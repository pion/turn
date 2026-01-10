// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package main implements a script to generate a self-signed CA certificate.
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"
	"time"
)

func ensureRequiredStringFlags(flags map[string]string) {
	for name, value := range flags {
		if value == "" {
			log.Fatalf("Required flag %s is missing", name)
		}
	}
}

func generateCACertificate(
	key *rsa.PrivateKey,
	certCN string,
	certOrg string,
	certLifetime time.Duration,
) ([]byte, error) {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   certCN,
			Country:      []string{"US"},
			Organization: []string{certOrg},
		},
		NotBefore:             time.Now().Add(time.Minute * -5),
		NotAfter:              time.Now().Add(certLifetime),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certData, err := x509.CreateCertificate(rand.Reader, &template, &template, key.Public(), key)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	return certData, nil
}

func main() {
	// Parse input flags
	certOrg := flag.String("cert-org", "", "Organization for certificate (required)")
	certCN := flag.String("cert-cn", "", "Common name (CN) for certificate (required)")
	certLifetimeSeconds := flag.Int("cert-lifetime-seconds", 0, "Certificate lifetime in seconds (required)")
	caCertFile := flag.String("cert-out", "", "Output file-path for generated CA certificate (required)")
	caCertKeyFile := flag.String("key-out", "", "Output file-path for generated CA certificate private key (required)")
	flag.Parse()

	// Validate and format flags
	ensureRequiredStringFlags(map[string]string{
		"-cert-org": *certOrg,
		"-cert-cn":  *certCN,
		"-cert-out": *caCertFile,
		"-key-out":  *caCertKeyFile,
	})

	if *certLifetimeSeconds <= 0 {
		log.Fatal("Required flag -cert-lifetime-seconds must be greater than 0")
	}

	certLifetime := time.Duration(*certLifetimeSeconds) * time.Second

	// Generate 2048-bit RSA key-pair
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatalf("Failed to generate key pair: %v", err)
	}

	// Encode key as PEM
	keyBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	// Generate CA certificate
	certData, err := generateCACertificate(key, *certCN, *certOrg, certLifetime)
	if err != nil {
		log.Fatalf("Failed to generate certificate: %v", err)
	}

	// Encode certificate as PEM
	certBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certData,
	})

	// Write key file
	if err := os.WriteFile(*caCertKeyFile, keyBytes, 0o600); err != nil {
		log.Fatal(err)
	}

	// Write cert file
	if err := os.WriteFile(*caCertFile, certBytes, 0o600); err != nil {
		log.Fatal(err)
	}

	log.Printf("✅ Certificate and private key written: %s, %s", *caCertFile, *caCertKeyFile)
}
