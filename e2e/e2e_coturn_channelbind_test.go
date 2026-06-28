// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build coturn && !js

package e2e

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/stun/v3"
	"github.com/pion/turn/v5"
	"github.com/stretchr/testify/assert"
)

const channelBindDropDuration = 1500 * time.Millisecond

type channelBindDropConn struct {
	net.PacketConn

	dropUntil           atomic.Int64
	droppedChannelBinds atomic.Uint32
	sentChannelData     atomic.Uint32
}

func (c *channelBindDropConn) dropChannelBindResponsesFor(d time.Duration) {
	c.dropUntil.Store(time.Now().Add(d).UnixNano())
}

func (c *channelBindDropConn) ReadFrom(p []byte) (int, net.Addr, error) {
	for {
		n, addr, err := c.PacketConn.ReadFrom(p)
		if err != nil {
			return n, addr, err
		}
		if time.Now().UnixNano() < c.dropUntil.Load() && isChannelBindSTUNMessage(p[:n]) {
			c.droppedChannelBinds.Add(1)

			continue
		}

		return n, addr, nil
	}
}

func (c *channelBindDropConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	if isTURNChannelData(p) {
		c.sentChannelData.Add(1)
	}

	return c.PacketConn.WriteTo(p, addr)
}

func isChannelBindSTUNMessage(data []byte) bool {
	msg := &stun.Message{Raw: data}
	if err := msg.Decode(); err != nil {
		return false
	}

	return msg.Type.Method == stun.MethodChannelBind
}

func isTURNChannelData(data []byte) bool {
	return len(data) >= 4 && data[0]&0xc0 == 0x40
}

func clientPionChannelBindAmbiguous400(manager *testmgr) {
	if err := waitForCoturnReady(manager); err != nil {
		manager.errChan <- err

		return
	}
	manager.clientMutex.Lock()
	defer manager.clientMutex.Unlock()

	recoveryClient, err := newChannelBindRecoveryClient(manager)
	if err != nil {
		manager.errChan <- err

		return
	}
	defer recoveryClient.close()

	if err = exerciseChannelBindAmbiguous400(recoveryClient); err != nil {
		manager.errChan <- err

		return
	}

	manager.messageReceived <- testMessage

	manager.clientDone <- nil
	close(manager.clientDone)
}

func waitForCoturnReady(manager *testmgr) error {
	select {
	case <-manager.serverReady:
		return nil
	case <-time.After(5 * time.Second):
		return errServerTimeout
	}
}

type channelBindRecoveryClient struct {
	client       *turn.Client
	filteredConn *channelBindDropConn
	peerConn     net.PacketConn
	relayConn    net.PacketConn
}

func newChannelBindRecoveryClient(manager *testmgr) (*channelBindRecoveryClient, error) {
	var err error
	manager.clientConn, err = net.ListenPacket("udp4", "127.0.0.1:0") //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("listen client: %w", err)
	}
	filteredConn := &channelBindDropConn{PacketConn: manager.clientConn}
	client, err := turn.NewClient(&turn.ClientConfig{
		TURNServerAddr: manager.serverAddr,
		Username:       manager.username,
		Password:       manager.password,
		Realm:          manager.realm,
		Conn:           filteredConn,
		RTO:            10 * time.Millisecond,
		LoggerFactory:  logging.NewDefaultLoggerFactory(),
	})
	if err != nil {
		return nil, fmt.Errorf("new client: %w", err)
	}

	if err = client.Listen(); err != nil {
		client.Close()

		return nil, fmt.Errorf("listen: %w", err)
	}

	relayConn, err := client.Allocate()
	if err != nil {
		client.Close()

		return nil, fmt.Errorf("allocate: %w", err)
	}

	peerConn, err := net.ListenPacket("udp4", "127.0.0.1:0") //nolint:noctx
	if err != nil {
		_ = relayConn.Close()
		client.Close()

		return nil, fmt.Errorf("listen peer: %w", err)
	}

	return &channelBindRecoveryClient{
		client:       client,
		filteredConn: filteredConn,
		peerConn:     peerConn,
		relayConn:    relayConn,
	}, nil
}

func (c *channelBindRecoveryClient) close() {
	_ = c.peerConn.Close()
	_ = c.relayConn.Close()
	c.client.Close()
}

func exerciseChannelBindAmbiguous400(recovery *channelBindRecoveryClient) error {
	peerAddr := recovery.peerConn.LocalAddr()
	payload := []byte(testMessage)
	retryPayload := []byte(testMessage + " retry")

	recovery.filteredConn.dropChannelBindResponsesFor(channelBindDropDuration)
	if _, err := recovery.relayConn.WriteTo(payload, peerAddr); err != nil {
		return fmt.Errorf("first WriteTo: %w", err)
	}

	time.Sleep(channelBindDropDuration + 500*time.Millisecond)
	if recovery.filteredConn.droppedChannelBinds.Load() == 0 {
		return errors.New("did not drop any coturn ChannelBind responses") //nolint:err113
	}

	drainPackets(recovery.peerConn)
	sentChannelDataStart := recovery.filteredConn.sentChannelData.Load()
	if err := writeUntilClosedWithoutChannelData(
		recovery.relayConn,
		peerAddr,
		retryPayload,
		recovery.filteredConn,
		sentChannelDataStart,
	); err != nil {
		return err
	}

	return nil
}

func writeUntilClosedWithoutChannelData(
	relayConn net.PacketConn,
	peerAddr net.Addr,
	payload []byte,
	filteredConn *channelBindDropConn,
	sentChannelDataStart uint32,
) error {
	deadline := time.Now().Add(testTimeLimit)
	attempt := 0
	for time.Now().Before(deadline) {
		attempt++
		attemptPayload := []byte(fmt.Sprintf("%s %d", payload, attempt))
		if _, err := relayConn.WriteTo(attemptPayload, peerAddr); err != nil {
			if isAllocationClosedWriteError(relayConn, err) {
				return nil
			}

			return fmt.Errorf("WriteTo failed before expected allocation close: %w", err)
		}
		if filteredConn.sentChannelData.Load() > sentChannelDataStart {
			return errors.New("sent ChannelData after ambiguous ChannelBind 400") //nolint:err113
		}
		time.Sleep(50 * time.Millisecond)
	}

	return errors.New("timed out waiting for allocation close after ambiguous ChannelBind 400") //nolint:err113
}

func isAllocationClosedWriteError(relayConn net.PacketConn, err error) bool {
	var opErr *net.OpError
	if !errors.As(err, &opErr) || opErr.Op != "write" || opErr.Err == nil {
		return false
	}

	localAddr := relayConn.LocalAddr()
	if localAddr == nil || opErr.Addr == nil ||
		opErr.Net != localAddr.Network() ||
		opErr.Addr.String() != localAddr.String() {
		return false
	}

	return errors.Is(err, net.ErrClosed) ||
		strings.Contains(opErr.Err.Error(), "use of closed network connection")
}

func drainPackets(conn net.PacketConn) {
	buf := make([]byte, 1500)
	for {
		_ = conn.SetReadDeadline(time.Now().Add(25 * time.Millisecond))
		if _, _, err := conn.ReadFrom(buf); err != nil {
			return
		}
	}
}

func TestPionCoturnChannelBindAmbiguous400Closes(t *testing.T) {
	t.Parallel()
	testPionE2ESimple(t, serverCoturn, clientPionChannelBindAmbiguous400)
}

func TestTURNChannelDataDetection(t *testing.T) {
	assert.True(t, isTURNChannelData([]byte{0x40, 0x00, 0x00, 0x00}))
	assert.False(t, isTURNChannelData([]byte{0x00, 0x01, 0x00, 0x00}))
	assert.False(t, isTURNChannelData([]byte{0x80, 0x00, 0x00, 0x00}))
	assert.False(t, isTURNChannelData([]byte{0x40, 0x00, 0x00}))
}
