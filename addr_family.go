// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package turn

import "github.com/pion/turn/v4/internal/proto"

// RequestedAddressFamily represents the REQUESTED-ADDRESS-FAMILY Attribute as
// defined in RFC 6156 Section 4.1.1.
type RequestedAddressFamily = proto.RequestedAddressFamily

// Values for RequestedAddressFamily as defined in RFC 6156 Section 4.1.1.
const (
	RequestedAddressFamilyIPv4 = proto.RequestedFamilyIPv4
	RequestedAddressFamilyIPv6 = proto.RequestedFamilyIPv6
)
