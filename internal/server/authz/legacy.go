// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package authz

import "net"

// LegacyAuthFunc is a function used to authorize requests compatible with legacy authorization.
type LegacyAuthFunc func(username, realm string, srcAddr net.Addr) (key []byte, ok bool)

// legacyAuthorizer is the an Authorizer implementation
// which wraps an AuthFunc in order to authorize requests.
type legacyAuthorizer struct {
	authFunc LegacyAuthFunc
}

// NewLegacy returns a new legacy authorizer.
func NewLegacy(fn LegacyAuthFunc) Authorizer {
	return &legacyAuthorizer{authFunc: fn}
}

// Authorize authorizes a request given request attributes.
func (a *legacyAuthorizer) Authorize(ra *RequestAttributes) (key []byte, ok bool) {
	return a.authFunc(ra.Username, ra.Realm, ra.SrcAddr)
}
