// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package main implements a CLI tool for generating
// long-term credentials.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/pion/turn/v3"
)

// Outputs username & password according to the
// Long-Term Credential Mechanism (RFC5389-10.2: https://tools.ietf.org/search/rfc5389#section-10.2)
func main() {
	authSecret := flag.String("authSecret", "", "Shared secret for the Long Term Credential Mechanism")
	showHelp := flag.Bool("h", false, "Show usage")
	flag.Parse()

	if showHelp != nil && *showHelp {
		log.Println("Usage:")
		log.Println("$ lt-cred-generator | xargs go run examples/turn-client/udp/main.go -host localhost -ping=true -user=")
		return
	}

	if authSecret == nil || len(*authSecret) == 0 {
		log.Fatal("Missing -authSecret parameter")
	}

	u, p, _ := turn.GenerateLongTermCredentials(*authSecret, time.Minute)
	if _, err := os.Stdout.WriteString(fmt.Sprintf("%s=%s", u, p)); err != nil { // For use with xargs
		log.Panicf("Failed to write to stdout: %s", err)
	}
	if _, err := os.Stderr.WriteString("\n"); err != nil { // Ignored by xargs
		log.Panicf("Failed to write to stderr: %s", err)
	}
}
