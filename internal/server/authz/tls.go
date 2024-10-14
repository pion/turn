// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package authz

import (
	"crypto/x509"
)

// tlsAuthorizer is the an Authorizer implementation which verifies
// client TLS certificate metadata in order to to authorize requests.
type tlsAuthorizer struct {
	verifyOpts        x509.VerifyOptions
	getKeyForUserFunc func(string) ([]byte, bool)
}

// NewTLS returns a new client tls certificate authorizer.
//
// This authorizer ensures that the client presents a valid TLS certificate
// for which the CommonName must match the TURN request's username attribute.
func NewTLS(
	verifyOpts x509.VerifyOptions,
	getKeyForUserFunc func(string) ([]byte, bool),
) Authorizer {
	return &tlsAuthorizer{
		verifyOpts:        verifyOpts,
		getKeyForUserFunc: getKeyForUserFunc,
	}
}

// Authorize authorizes a request given request attributes.
func (a *tlsAuthorizer) Authorize(ra *RequestAttributes) ([]byte, bool) {
	if ra.TLS == nil || len(ra.TLS.PeerCertificates) == 0 {
		// request not allowed due to not having tls state metadata
		// TODO: INFO log
		return nil, false
	}

	key, ok := a.getKeyForUserFunc(ra.Username)
	if !ok {
		// request not allowed due to having no key for the TURN request's username
		// TODO: INFO log
		return nil, false
	}

	for _, cert := range ra.TLS.PeerCertificates {
		if cert.Subject.CommonName != ra.Username {
			// cert not allowed due to not matching the TURN username
			// TODO: DEBUG log
			continue
		}

		if _, err := cert.Verify(a.verifyOpts); err != nil {
			// cert not allowed due to failed validation
			// TODO: WARN log
			continue
		}

		// a valid certificate was allowed
		return key, true
	}

	// request not allowed due to not having any valid certs
	// TODO: INFO log
	return nil, false
}
