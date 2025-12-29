// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package allocation

import "errors"

var (
	ErrTCPConnectionTimeoutOrFailure = errors.New("failed to create tcp connection")
	ErrDupeTCPConnection             = errors.New("tcp connection already exists for peer address")

	errAllocatePacketConnMustBeSet  = errors.New("AllocatePacketConn must be set")
	errAllocateListenerMustBeSet    = errors.New("AllocateListener must be set")
	errLeveledLoggerMustBeSet       = errors.New("LeveledLogger must be set")
	errSameChannelDifferentPeer     = errors.New("you cannot use the same channel number with different peer")
	errNilFiveTuple                 = errors.New("allocations must not be created with nil FivTuple")
	errNilFiveTupleSrcAddr          = errors.New("allocations must not be created with nil FiveTuple.SrcAddr")
	errNilFiveTupleDstAddr          = errors.New("allocations must not be created with nil FiveTuple.DstAddr")
	errNilTurnSocket                = errors.New("allocations must not be created with nil turnSocket")
	errLifetimeZero                 = errors.New("allocations must not be created with a lifetime of 0")
	errDupeFiveTuple                = errors.New("allocation attempt created with duplicate FiveTuple")
	errFailedToCastUDPAddr          = errors.New("failed to cast net.Addr to *net.UDPAddr")
	errFailedToAllocateEvenPort     = errors.New("failed to allocate an even port")
	errAdminProhibited              = errors.New("permission request administratively prohibited")
	errFailedToGenerateConnectionID = errors.New("failed to generate a unique connection id")
	errInvalidPeerAddress           = errors.New("invalid peer address")
	errNilRelaySocket               = errors.New("allocation has no relay socket")
)
