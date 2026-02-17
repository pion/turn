// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js
// +build !js

package e2e

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/transport/v4/test"
	"github.com/pion/turn/v5"
	"github.com/pion/turn/v5/internal/auth"
	"github.com/stretchr/testify/assert"
)

const (
	testUsername  = "user"
	testPassword  = "pass"
	testRealm     = "pion.ly"
	testMessage   = "Hello TURN"
	testTimeLimit = 5 * time.Second
)

var (
	errServerTimeout  = errors.New("waiting on serverReady err: timeout")
	errNoRelayAddress = errors.New("allocation succeeded but no relay address")
)

func randomPort(tb testing.TB) int {
	tb.Helper()
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0") // nolint: noctx
	assert.NoError(tb, err, "failed to pick port")

	defer func() {
		_ = conn.Close()
	}()
	switch addr := conn.LocalAddr().(type) {
	case *net.UDPAddr:
		return addr.Port
	default:
		assert.Fail(tb, "failed to acquire port", "unknown addr type %T", addr)

		return 0
	}
}

type testmgr struct {
	ctx             context.Context //nolint:containedctx
	serverAddr      string
	username        string
	password        string
	realm           string
	clientMutex     *sync.Mutex
	clientConn      net.PacketConn
	clientDone      chan error
	serverMutex     *sync.Mutex
	serverConn      net.PacketConn
	server          *turn.Server
	serverReady     chan struct{}
	serverDone      chan error
	errChan         chan error
	messageReceived chan string
	client          func(*testmgr)
	serverFunc      func(*testmgr)
}

func newTestMgr(
	t *testing.T,
	ctx context.Context,
	serverAddr string,
	username, password, realm string,
	serverFunc, client func(*testmgr),
) *testmgr {
	t.Helper()

	return &testmgr{
		ctx:             ctx,
		serverAddr:      serverAddr,
		username:        username,
		password:        password,
		realm:           realm,
		clientMutex:     &sync.Mutex{},
		serverMutex:     &sync.Mutex{},
		serverReady:     make(chan struct{}),
		serverDone:      make(chan error),
		clientDone:      make(chan error),
		errChan:         make(chan error),
		messageReceived: make(chan string),
		serverFunc:      serverFunc,
		client:          client,
	}
}

func (m *testmgr) assert(t *testing.T) {
	t.Helper()

	go m.serverFunc(m)
	go m.client(m)

	defer func() {
		if m.clientConn != nil {
			_ = m.clientConn.Close()
		}
		if m.serverConn != nil {
			_ = m.serverConn.Close()
		}
		if m.server != nil {
			_ = m.server.Close()
		}
	}()

	select {
	case err := <-m.errChan:
		assert.NoError(t, err)
	case msg := <-m.messageReceived:
		assert.Equal(t, testMessage, msg)
	case <-m.clientDone:
	case <-time.After(testTimeLimit):
		assert.Fail(t, "Test timeout")
	}
}

//nolint:cyclop
func (m *testmgr) cleanup(t *testing.T) {
	t.Helper()

	clientDone, serverDone := false, false
	for {
		select {
		case err := <-m.clientDone:
			if err != nil {
				t.Logf("Client error: %v", err)
			}
			clientDone = true
			if clientDone && serverDone {
				return
			}

		case err := <-m.serverDone:
			if err != nil {
				t.Logf("Server error: %v", err)
			}
			serverDone = true
			if clientDone && serverDone {
				return
			}

		case <-time.After(testTimeLimit):
			assert.Fail(t, "Test timeout waiting for cleanup")

			return
		}
	}
}

//nolint:varnamelen
func clientPion(m *testmgr) {
	select {
	case <-m.serverReady:
		// OK
	case <-time.After(time.Second * 5):
		m.errChan <- errServerTimeout

		return
	}

	m.clientMutex.Lock()
	defer m.clientMutex.Unlock()

	var err error
	lc := net.ListenConfig{}
	m.clientConn, err = lc.ListenPacket(m.ctx, "udp4", "127.0.0.1:0")
	if err != nil {
		m.errChan <- err

		return
	}

	client, err := turn.NewClient(&turn.ClientConfig{
		TURNServerAddr: m.serverAddr,
		Username:       m.username,
		Password:       m.password,
		Realm:          m.realm,
		Conn:           m.clientConn,
		LoggerFactory:  logging.NewDefaultLoggerFactory(),
	})
	if err != nil {
		m.errChan <- err

		return
	}
	defer client.Close()

	if err = client.Listen(); err != nil {
		m.errChan <- err

		return
	}

	relayConn, err := client.Allocate()
	if err != nil {
		m.errChan <- err

		return
	}
	defer func() {
		_ = relayConn.Close()
	}()

	// Verify we got a relay address
	if relayConn.LocalAddr() == nil {
		m.errChan <- errNoRelayAddress

		return
	}

	// Signal success
	m.messageReceived <- testMessage

	m.clientDone <- nil
	close(m.clientDone)
}

//nolint:varnamelen
func serverPion(m *testmgr) {
	m.serverMutex.Lock()
	defer m.serverMutex.Unlock()

	var err error
	lc := net.ListenConfig{}
	m.serverConn, err = lc.ListenPacket(m.ctx, "udp4", m.serverAddr)
	if err != nil {
		m.errChan <- err

		return
	}

	m.server, err = turn.NewServer(turn.ServerConfig{
		Realm: m.realm,
		AuthHandler: func(ra *auth.RequestAttributes) (userID string, key []byte, ok bool) {
			if ra.Username == m.username && ra.Realm == m.realm {
				return ra.Username, turn.GenerateAuthKey(ra.Username, m.realm, m.password), true
			}

			return "", nil, false
		},
		PacketConnConfigs: []turn.PacketConnConfig{
			{
				PacketConn:            m.serverConn,
				RelayAddressGenerator: &turn.RelayAddressGeneratorNone{Address: "127.0.0.1"},
			},
		},
		LoggerFactory: logging.NewDefaultLoggerFactory(),
	})
	if err != nil {
		m.errChan <- err

		return
	}

	m.serverReady <- struct{}{}

	<-m.ctx.Done()
	m.serverDone <- nil
	close(m.serverDone)
}

//nolint:varnamelen
func clientPionIPv6(m *testmgr) {
	select {
	case <-m.serverReady:
	case <-time.After(time.Second * 5):
		m.errChan <- errServerTimeout

		return
	}

	m.clientMutex.Lock()
	defer m.clientMutex.Unlock()

	var err error
	lc := net.ListenConfig{}
	m.clientConn, err = lc.ListenPacket(m.ctx, "udp6", "[::1]:0")
	if err != nil {
		m.errChan <- err

		return
	}

	client, err := turn.NewClient(&turn.ClientConfig{
		STUNServerAddr:         m.serverAddr,
		TURNServerAddr:         m.serverAddr,
		Username:               m.username,
		Password:               m.password,
		Realm:                  m.realm,
		Conn:                   m.clientConn,
		RequestedAddressFamily: turn.RequestedAddressFamilyIPv6,
		LoggerFactory:          logging.NewDefaultLoggerFactory(),
	})
	if err != nil {
		m.errChan <- err

		return
	}
	defer client.Close()

	if err = client.Listen(); err != nil {
		m.errChan <- err

		return
	}

	relayConn, err := client.Allocate()
	if err != nil {
		m.errChan <- err

		return
	}
	defer func() {
		_ = relayConn.Close()
	}()

	if relayConn.LocalAddr() == nil {
		m.errChan <- errNoRelayAddress

		return
	}

	m.messageReceived <- testMessage

	m.clientDone <- nil
	close(m.clientDone)
}

//nolint:varnamelen
func serverPionIPv6(m *testmgr) {
	m.serverMutex.Lock()
	defer m.serverMutex.Unlock()

	var err error
	lc := net.ListenConfig{}
	m.serverConn, err = lc.ListenPacket(m.ctx, "udp6", m.serverAddr)
	if err != nil {
		m.errChan <- err

		return
	}

	host, _, err := net.SplitHostPort(m.serverAddr)
	if err != nil {
		m.errChan <- err

		return
	}

	m.server, err = turn.NewServer(turn.ServerConfig{
		Realm: m.realm,
		AuthHandler: func(ra *auth.RequestAttributes) (userID string, key []byte, ok bool) {
			if ra.Username == m.username && ra.Realm == m.realm {
				return ra.Username, turn.GenerateAuthKey(ra.Username, m.realm, m.password), true
			}

			return "", nil, false
		},
		PacketConnConfigs: []turn.PacketConnConfig{
			{
				PacketConn:            m.serverConn,
				RelayAddressGenerator: &turn.RelayAddressGeneratorNone{Address: host},
			},
		},
		LoggerFactory: logging.NewDefaultLoggerFactory(),
	})
	if err != nil {
		m.errChan <- err

		return
	}

	m.serverReady <- struct{}{}

	<-m.ctx.Done()
	m.serverDone <- nil
	close(m.serverDone)
}

func testPionE2ESimple(t *testing.T, server, client func(*testmgr)) {
	t.Helper()
	lim := test.TimeOut(time.Second * 30)
	defer lim.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	serverAddr := fmt.Sprintf("127.0.0.1:%d", randomPort(t))
	mgr := newTestMgr(t, ctx, serverAddr, testUsername, testPassword, testRealm, server, client)
	defer mgr.cleanup(t)
	mgr.assert(t)
}

func testPionE2ESimpleIPv6(t *testing.T, server, client func(*testmgr)) {
	t.Helper()
	lim := test.TimeOut(time.Second * 30)
	defer lim.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	serverAddr := fmt.Sprintf("[::1]:%d", randomPort(t))
	mgr := newTestMgr(t, ctx, serverAddr, testUsername, testPassword, testRealm, server, client)
	defer mgr.cleanup(t)
	mgr.assert(t)
}

func TestPionE2ESimple(t *testing.T) {
	t.Parallel()
	testPionE2ESimple(t, serverPion, clientPion)
}

func TestPionE2ESimpleIPv6(t *testing.T) {
	t.Parallel()
	testPionE2ESimpleIPv6(t, serverPionIPv6, clientPionIPv6)
}
