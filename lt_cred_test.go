// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js
// +build !js

package turn

import (
	"net"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/stretchr/testify/assert"
)

func TestLtCredMech(t *testing.T) {
	username := "1599491771"
	sharedSecret := "foobar"

	expectedPassword := "Tpz/nKkyvX/vMSLKvL4sbtBt8Vs=" //nolint:gosec
	actualPassword, _ := longTermCredentials(username, sharedSecret)
	if expectedPassword != actualPassword {
		t.Errorf("Expected %q, got %q", expectedPassword, actualPassword)
	}
}

func TestNewLongTermAuthHandler(t *testing.T) {
	const sharedSecret = "HELLO_WORLD"

	serverListener, err := net.ListenPacket("udp4", "0.0.0.0:3478")
	assert.NoError(t, err)

	server, err := NewServer(ServerConfig{
		AuthHandler: NewLongTermAuthHandler(sharedSecret, nil),
		PacketConnConfigs: []PacketConnConfig{
			{
				PacketConn: serverListener,
				RelayAddressGenerator: &RelayAddressGeneratorStatic{
					RelayAddress: net.ParseIP("127.0.0.1"),
					Address:      "0.0.0.0",
				},
			},
		},
		Realm:         "pion.ly",
		LoggerFactory: logging.NewDefaultLoggerFactory(),
	})
	assert.NoError(t, err)

	conn, err := net.ListenPacket("udp4", "0.0.0.0:0")
	assert.NoError(t, err)

	username, password, err := GenerateLongTermCredentials(sharedSecret, time.Minute)
	assert.NoError(t, err)

	addr := "127.0.0.1:3478"

	client, err := NewClient(&ClientConfig{
		STUNServerAddr: addr,
		TURNServerAddr: addr,
		Conn:           conn,
		Username:       username,
		Password:       password,
		LoggerFactory:  logging.NewDefaultLoggerFactory(),
	})
	assert.NoError(t, err)
	assert.NoError(t, client.Listen())

	relayConn, err := client.Allocate()
	assert.NoError(t, err)

	client.Close()
	assert.NoError(t, relayConn.Close())
	assert.NoError(t, conn.Close())
	assert.NoError(t, server.Close())
}

func TestLongTermTURNRESTAuthHandler(t *testing.T) {
	const sharedSecret = "HELLO_WORLD"

	serverListener, err := net.ListenPacket("udp4", "0.0.0.0:3478")
	assert.NoError(t, err)

	server, err := NewServer(ServerConfig{
		AuthHandler: LongTermTURNRESTAuthHandler(sharedSecret, nil),
		PacketConnConfigs: []PacketConnConfig{
			{
				PacketConn: serverListener,
				RelayAddressGenerator: &RelayAddressGeneratorStatic{
					RelayAddress: net.ParseIP("127.0.0.1"),
					Address:      "0.0.0.0",
				},
			},
		},
		Realm:         "pion.ly",
		LoggerFactory: logging.NewDefaultLoggerFactory(),
	})
	assert.NoError(t, err)

	conn, err := net.ListenPacket("udp4", "0.0.0.0:0")
	assert.NoError(t, err)

	username, password, err := GenerateLongTermTURNRESTCredentials(sharedSecret, "testuser", time.Minute)
	assert.NoError(t, err)

	client, err := NewClient(&ClientConfig{
		STUNServerAddr: "0.0.0.0:3478",
		TURNServerAddr: "0.0.0.0:3478",
		Conn:           conn,
		Username:       username,
		Password:       password,
		LoggerFactory:  logging.NewDefaultLoggerFactory(),
	})
	assert.NoError(t, err)
	assert.NoError(t, client.Listen())

	relayConn, err := client.Allocate()
	assert.NoError(t, err)

	client.Close()
	assert.NoError(t, relayConn.Close())
	assert.NoError(t, conn.Close())
	assert.NoError(t, server.Close())
}
