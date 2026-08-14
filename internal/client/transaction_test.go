// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package client

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTransactionClose(t *testing.T) {
	serverA := &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 3478}
	serverB := &net.UDPAddr{IP: net.ParseIP("10.0.0.2"), Port: 3478}

	t.Run("Close unblocks WaitForResult with a clean error", func(t *testing.T) {
		tr := NewTransaction(&TransactionConfig{
			Key:      "k",
			To:       serverA,
			Interval: time.Second,
		})

		resCh := make(chan TransactionResult, 1)
		go func() { resCh <- tr.WaitForResult() }()

		tr.Close()

		select {
		case res := <-resCh:
			assert.True(t, errors.Is(res.Err, errTransactionClosed))
		case <-time.After(5 * time.Second):
			assert.Fail(t, "WaitForResult did not unblock on Close")
		}
	})

	t.Run("CloseAndDeleteAllTo scopes by destination", func(t *testing.T) {
		trMap := NewTransactionMap()
		trA := NewTransaction(&TransactionConfig{Key: "a", To: serverA, Interval: time.Second})
		trB := NewTransaction(&TransactionConfig{Key: "b", To: serverB, Interval: time.Second})
		trMap.Insert("a", trA)
		trMap.Insert("b", trB)

		resA := make(chan TransactionResult, 1)
		resB := make(chan TransactionResult, 1)
		go func() { resA <- trA.WaitForResult() }()
		go func() { resB <- trB.WaitForResult() }()

		trMap.CloseAndDeleteAllTo(serverA)

		select {
		case res := <-resA:
			assert.True(t, errors.Is(res.Err, errTransactionClosed))
		case <-time.After(5 * time.Second):
			assert.Fail(t, "transaction to the aborted destination did not unblock")
		}

		select {
		case res := <-resB:
			assert.Failf(t, "transaction to another destination was aborted", "err: %v", res.Err)
		case <-time.After(200 * time.Millisecond):
		}
		assert.Equal(t, 1, trMap.Size())

		trMap.CloseAndDeleteAll()
		select {
		case res := <-resB:
			assert.True(t, errors.Is(res.Err, errTransactionClosed))
		case <-time.After(5 * time.Second):
			assert.Fail(t, "remaining transaction did not unblock")
		}
	})
}
