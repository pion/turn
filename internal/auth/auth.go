// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package auth provides internal authentication / authorization
// types and utilities for the TURN server.
package auth

import (
	"crypto/tls"
	"net"
)

// RequestAttributes represents attributes of a TURN request which
// may be useful for authorizing the underlying request.
type RequestAttributes struct {
	Username string
	Realm    string
	SrcAddr  net.Addr
	TLS      *tls.ConnectionState

	// extend as needed
}

// AuthHandler is a callback used to handle incoming auth requests,
// allowing users to customize Pion TURN with custom behavior.
type AuthHandler func(ra *RequestAttributes) (userID string, key []byte, ok bool)
