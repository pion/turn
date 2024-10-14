// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package authz

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

// Authorizer represents functionality required to authorize a request.
type Authorizer interface {
	Authorize(ra *RequestAttributes) (key []byte, ok bool)
}
