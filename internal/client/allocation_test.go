// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/stun/v4"
	"github.com/stretchr/testify/assert"
)

func TestRefreshAllocationReturnsTurnError(t *testing.T) {
	client := &mockClient{
		performTransaction: func(*stun.Message, net.Addr, bool, context.Context) (TransactionResult, error) {
			return TransactionResult{Msg: stun.MustBuild(
				stun.NewType(stun.MethodRefresh, stun.ClassErrorResponse),
				stun.ErrorCodeAttribute{Code: stun.CodeTryAlternate, Reason: []byte("Try Alternate")},
			)}, nil
		},
	}

	a := &allocation{
		client:     client,
		serverAddr: &net.UDPAddr{IP: net.IPv4(1, 2, 3, 4), Port: 3478},
		username:   stun.NewUsername("user"),
		realm:      stun.NewRealm("realm"),
		integrity:  stun.NewShortTermIntegrity("pass"),
		_nonce:     stun.NewNonce("nonce"),
		log:        logging.NewDefaultLoggerFactory().NewLogger("test"),
	}

	err := a.refreshAllocation(time.Minute, false)
	var turnErr *stun.TurnError
	assert.Error(t, err)
	assert.True(t, errors.As(err, &turnErr), "should return a TurnError")
	assert.Equal(t, stun.CodeTryAlternate, turnErr.ErrorCodeAttr.Code)
}
