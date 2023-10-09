// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package offload

import "errors"

//nolint:revive
var (
	ErrUnsupportedProtocol        = errors.New("offload: protocol not supported")
	ErrConnectionNotFound         = errors.New("offload: connection not found")
	ErrXDPAlreadyInitialized      = errors.New("offload: XDP engine is already initialized")
	ErrXDPLocalRedirectProhibited = errors.New("offload: XDP local redirect not allowed")
)
