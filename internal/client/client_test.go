// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package client

import (
	"net"

	"github.com/pion/stun/v2"
)

type mockClient struct {
	writeTo            func(data []byte, to net.Addr) (int, error)
	performTransaction func(msg *stun.Message, to net.Addr, dontWait bool) (TransactionResult, error)
	onDeallocated      func(relayedAddr net.Addr)
}

func (c *mockClient) WriteTo(data []byte, to net.Addr) (int, error) {
	if c.writeTo != nil {
		return c.writeTo(data, to)
	}
	return 0, nil
}

func (c *mockClient) PerformTransaction(msg *stun.Message, to net.Addr, dontWait bool) (TransactionResult, error) {
	if c.performTransaction != nil {
		return c.performTransaction(msg, to, dontWait)
	}
	return TransactionResult{}, nil
}

func (c *mockClient) OnDeallocated(relayedAddr net.Addr) {
	if c.onDeallocated != nil {
		c.onDeallocated(relayedAddr)
	}
}
