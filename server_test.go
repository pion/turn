// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js
// +build !js

package turn

import (
	"fmt"
	"net"
	"syscall"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/transport/v3/test"
	"github.com/pion/transport/v3/vnet"
	"github.com/pion/turn/v3/internal/allocation"
	"github.com/pion/turn/v3/internal/proto"
	"github.com/stretchr/testify/assert"
)

func TestServer(t *testing.T) {
	lim := test.TimeOut(time.Second * 30)
	defer lim.Stop()

	report := test.CheckRoutines(t)
	defer report()

	loggerFactory := logging.NewDefaultLoggerFactory()

	credMap := map[string][]byte{
		"user": GenerateAuthKey("user", "pion.ly", "pass"),
	}

	t.Run("simple", func(t *testing.T) {
		udpListener, err := net.ListenPacket("udp4", "0.0.0.0:3478")
		assert.NoError(t, err)

		server, err := NewServer(ServerConfig{
			AuthHandler: func(username, _ string, _ net.Addr) (key []byte, ok bool) {
				if pw, ok := credMap[username]; ok {
					return pw, true
				}
				return nil, false
			},
			PacketConnConfigs: []PacketConnConfig{
				{
					PacketConn: udpListener,
					RelayAddressGenerator: &RelayAddressGeneratorStatic{
						RelayAddress: net.ParseIP("127.0.0.1"),
						Address:      "0.0.0.0",
					},
				},
			},
			Realm:         "pion.ly",
			LoggerFactory: loggerFactory,
		})
		assert.NoError(t, err)

		assert.Equal(t, proto.DefaultLifetime, server.channelBindTimeout, "should match")

		conn, err := net.ListenPacket("udp4", "0.0.0.0:0")
		assert.NoError(t, err)

		client, err := NewClient(&ClientConfig{
			Conn:          conn,
			LoggerFactory: loggerFactory,
		})
		assert.NoError(t, err)
		assert.NoError(t, client.Listen())

		_, err = client.SendBindingRequestTo(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 3478})
		assert.NoError(t, err, "should succeed")

		client.Close()
		assert.NoError(t, conn.Close())

		assert.NoError(t, server.Close())
	})

	t.Run("default inboundMTU", func(t *testing.T) {
		udpListener, err := net.ListenPacket("udp4", "0.0.0.0:3478")
		assert.NoError(t, err)
		server, err := NewServer(ServerConfig{
			LoggerFactory: loggerFactory,
			PacketConnConfigs: []PacketConnConfig{
				{
					PacketConn: udpListener,
					RelayAddressGenerator: &RelayAddressGeneratorStatic{
						RelayAddress: net.ParseIP("127.0.0.1"),
						Address:      "0.0.0.0",
					},
				},
			},
		})
		assert.NoError(t, err)
		assert.Equal(t, server.inboundMTU, defaultInboundMTU)
		assert.NoError(t, server.Close())
	})

	t.Run("Set inboundMTU", func(t *testing.T) {
		udpListener, err := net.ListenPacket("udp4", "0.0.0.0:3478")
		assert.NoError(t, err)
		server, err := NewServer(ServerConfig{
			InboundMTU:    2000,
			LoggerFactory: loggerFactory,
			PacketConnConfigs: []PacketConnConfig{
				{
					PacketConn: udpListener,
					RelayAddressGenerator: &RelayAddressGeneratorStatic{
						RelayAddress: net.ParseIP("127.0.0.1"),
						Address:      "0.0.0.0",
					},
				},
			},
		})
		assert.NoError(t, err)
		assert.Equal(t, server.inboundMTU, 2000)
		assert.NoError(t, server.Close())
	})

	t.Run("Delete allocation on spontaneous TCP close", func(t *testing.T) {
		// Test whether allocation is properly deleted when client spontaneously closes the
		// TCP connection underlying it
		tcpListener, err := net.Listen("tcp4", "127.0.0.1:3478")
		assert.NoError(t, err)

		server, err := NewServer(ServerConfig{
			AuthHandler: func(username, _ string, _ net.Addr) (key []byte, ok bool) {
				if pw, ok := credMap[username]; ok {
					return pw, true
				}
				return nil, false
			},
			ListenerConfigs: []ListenerConfig{
				{
					Listener: tcpListener,
					RelayAddressGenerator: &RelayAddressGeneratorStatic{
						RelayAddress: net.ParseIP("127.0.0.1"),
						Address:      "127.0.0.1",
					},
				},
			},
			Realm:         "pion.ly",
			LoggerFactory: loggerFactory,
		})
		assert.NoError(t, err)

		// make sure we can reuse the client port
		dialer := &net.Dialer{
			Control: func(_, _ string, conn syscall.RawConn) error {
				return conn.Control(func(descriptor uintptr) {
					_ = syscall.SetsockoptInt(int(descriptor), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
				})
			},
		}
		conn, err := dialer.Dial("tcp", "127.0.0.1:3478")
		assert.NoError(t, err)

		clientAddr := conn.LocalAddr()

		serverAddr, err := net.ResolveTCPAddr("tcp4", "127.0.0.1:3478")
		assert.NoError(t, err)

		client, err := NewClient(&ClientConfig{
			STUNServerAddr: serverAddr.String(),
			TURNServerAddr: serverAddr.String(),
			Conn:           NewSTUNConn(conn),
			Username:       "user",
			Password:       "pass",
			Realm:          "pion.ly",
			LoggerFactory:  loggerFactory,
		})
		assert.NoError(t, err)
		assert.NoError(t, client.Listen())

		_, err = client.SendBindingRequestTo(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 3478})
		assert.NoError(t, err, "should succeed")

		relayConn, err := client.Allocate()
		assert.NoError(t, err)
		assert.NotNil(t, relayConn)

		fiveTuple := &allocation.FiveTuple{
			Protocol: allocation.UDP, // Fixed UDP
			SrcAddr:  clientAddr,
			DstAddr:  serverAddr,
		}
		// Allocation exists
		assert.Len(t, server.allocationManagers, 1)
		assert.NotNil(t, server.allocationManagers[0].GetAllocation(fiveTuple))

		// client.Close()
		// This should properly close the client and delete the allocation on the server
		assert.NoError(t, conn.Close())

		// Let connection to properly close
		time.Sleep(100 * time.Millisecond)
		// to we still have the allocation on the server?
		assert.Nil(t, server.allocationManagers[0].GetAllocation(fiveTuple))

		client.Close()
		// This should err: client connection has gone so we cannot send the Refresh(0)
		// message
		assert.Error(t, relayConn.Close())
		assert.NoError(t, server.Close())
	})

	t.Run("Filter on client address and peer IP", func(t *testing.T) {
		udpListener, err := net.ListenPacket("udp4", "0.0.0.0:3478")
		assert.NoError(t, err)

		server, err := NewServer(ServerConfig{
			AuthHandler: func(username, _ string, _ net.Addr) (key []byte, ok bool) {
				if pw, ok := credMap[username]; ok {
					return pw, true
				}
				return nil, false
			},
			PacketConnConfigs: []PacketConnConfig{
				{
					PacketConn: udpListener,
					RelayAddressGenerator: &RelayAddressGeneratorStatic{
						RelayAddress: net.ParseIP("127.0.0.1"),
						Address:      "0.0.0.0",
					},
					PermissionHandler: func(src net.Addr, peer net.IP) bool {
						return src.String() == "127.0.0.1:54321" &&
							peer.Equal(net.ParseIP("127.0.0.4"))
					},
				},
			},
			Realm:         "pion.ly",
			LoggerFactory: loggerFactory,
		})
		assert.NoError(t, err)

		// Enforce correct client IP and port
		conn, err := net.ListenPacket("udp4", "127.0.0.1:54321")
		assert.NoError(t, err)

		addr := "127.0.0.1:3478"

		client, err := NewClient(&ClientConfig{
			STUNServerAddr: addr,
			TURNServerAddr: addr,
			Conn:           conn,
			Username:       "user",
			Password:       "pass",
			Realm:          "pion.ly",
			LoggerFactory:  loggerFactory,
		})
		assert.NoError(t, err)
		assert.NoError(t, client.Listen())

		relayConn, err := client.Allocate()
		assert.NoError(t, err)

		whiteAddr, errA := net.ResolveUDPAddr("udp", "127.0.0.4:12345")
		assert.NoError(t, errA, "should succeed")
		blackAddr, errB1 := net.ResolveUDPAddr("udp", "127.0.0.5:12345")
		assert.NoError(t, errB1, "should succeed")

		// Explicit CreatePermission
		err = client.CreatePermission(whiteAddr)
		assert.NoError(t, err, "grant permission for whitelisted peer")

		err = client.CreatePermission(blackAddr)
		assert.ErrorContains(t, err, "error", "deny permission for blacklisted peer address")

		err = client.CreatePermission(whiteAddr, whiteAddr)
		assert.NoError(t, err, "grant permission for repeated whitelisted peer addresses")

		err = client.CreatePermission(blackAddr)
		assert.ErrorContains(t, err, "error", "deny permission for repeated blacklisted peer address")

		// Isn't this a corner case in the spec?
		err = client.CreatePermission(whiteAddr, blackAddr)
		assert.ErrorContains(t, err, "error", "deny permission for mixed whitelisted and blacklisted peers")

		// Implicit CreatePermission for ChannelBindRequests: WriteTo always tries to bind a channel
		_, err = relayConn.WriteTo([]byte("Hello"), whiteAddr)
		assert.NoError(t, err, "write to whitelisted peer address succeeds - 1")

		_, err = relayConn.WriteTo([]byte("Hello"), blackAddr)
		assert.ErrorContains(t, err, "error", "write to blacklisted peer address fails - 1")

		_, err = relayConn.WriteTo([]byte("Hello"), whiteAddr)
		assert.NoError(t, err, "write to whitelisted peer address succeeds - 2")

		_, err = relayConn.WriteTo([]byte("Hello"), blackAddr)
		assert.ErrorContains(t, err, "error", "write to blacklisted peer address fails - 2")

		_, err = relayConn.WriteTo([]byte("Hello"), whiteAddr)
		assert.NoError(t, err, "write to whitelisted peer address succeeds - 3")

		_, err = relayConn.WriteTo([]byte("Hello"), blackAddr)
		assert.ErrorContains(t, err, "error", "write to blacklisted peer address fails - 3")

		// Let the previous transaction terminate
		time.Sleep(200 * time.Millisecond)
		assert.NoError(t, relayConn.Close())

		client.Close()
		assert.NoError(t, conn.Close())

		// Enforce filtered source address
		conn2, err := net.ListenPacket("udp4", "127.0.0.1:12321")
		assert.NoError(t, err)

		client2, err := NewClient(&ClientConfig{
			STUNServerAddr: addr,
			TURNServerAddr: addr,
			Conn:           conn2,
			Username:       "user",
			Password:       "pass",
			Realm:          "pion.ly",
			LoggerFactory:  loggerFactory,
		})
		assert.NoError(t, err)
		assert.NoError(t, client2.Listen())

		relayConn2, err := client2.Allocate()
		assert.NoError(t, err)

		// Explicit CreatePermission
		err = client2.CreatePermission(whiteAddr)
		assert.ErrorContains(t, err, "error", "deny permission from filtered source to whitelisted peer")

		err = client2.CreatePermission(blackAddr)
		assert.ErrorContains(t, err, "error", "deny permission from filtered source to blacklisted peer")

		// Implicit CreatePermission for ChannelBindRequests: WriteTo always tries to bind a channel
		_, err = relayConn2.WriteTo([]byte("Hello"), whiteAddr)
		assert.ErrorContains(t, err, "error", "write from filtered source to whitelisted peer fails - 1")

		_, err = relayConn2.WriteTo([]byte("Hello"), blackAddr)
		assert.ErrorContains(t, err, "error", "write from filtered source to blacklisted peer fails - 1")

		_, err = relayConn2.WriteTo([]byte("Hello"), whiteAddr)
		assert.ErrorContains(t, err, "error", "write from filtered source to whitelisted peer fails - 2")

		_, err = relayConn2.WriteTo([]byte("Hello"), blackAddr)
		assert.ErrorContains(t, err, "error", "write from filtered source to blacklisted peer fails - 2")

		_, err = relayConn2.WriteTo([]byte("Hello"), whiteAddr)
		assert.ErrorContains(t, err, "error", "write from filtered source to whitelisted peer fails - 3")

		_, err = relayConn2.WriteTo([]byte("Hello"), blackAddr)
		assert.ErrorContains(t, err, "error", "write from filtered source to blacklisted peer fails - 3")

		// Let the previous transaction terminate
		time.Sleep(200 * time.Millisecond)
		assert.NoError(t, relayConn2.Close())

		client2.Close()
		assert.NoError(t, conn2.Close())

		assert.NoError(t, server.Close())
	})
}

type VNet struct {
	wan    *vnet.Router
	net0   *vnet.Net // net (0) on the WAN
	net1   *vnet.Net // net (1) on the WAN
	netL0  *vnet.Net // net (0) on the LAN
	server *Server
}

func (v *VNet) Close() error {
	if err := v.server.Close(); err != nil {
		return err
	}
	return v.wan.Stop()
}

func buildVNet() (*VNet, error) {
	loggerFactory := logging.NewDefaultLoggerFactory()

	// WAN
	wan, err := vnet.NewRouter(&vnet.RouterConfig{
		CIDR:          "0.0.0.0/0",
		LoggerFactory: loggerFactory,
	})
	if err != nil {
		return nil, err
	}

	net0, err := vnet.NewNet(&vnet.NetConfig{
		StaticIP: "1.2.3.4", // Will be assigned to eth0
	})
	if err != nil {
		return nil, err
	}

	err = wan.AddNet(net0)
	if err != nil {
		return nil, err
	}

	net1, err := vnet.NewNet(&vnet.NetConfig{
		StaticIP: "1.2.3.5", // Will be assigned to eth0
	})
	if err != nil {
		return nil, err
	}

	err = wan.AddNet(net1)
	if err != nil {
		return nil, err
	}

	// LAN
	lan, err := vnet.NewRouter(&vnet.RouterConfig{
		StaticIP: "5.6.7.8", // This router's external IP on eth0
		CIDR:     "192.168.0.0/24",
		NATType: &vnet.NATType{
			MappingBehavior:   vnet.EndpointIndependent,
			FilteringBehavior: vnet.EndpointIndependent,
		},
		LoggerFactory: loggerFactory,
	})
	if err != nil {
		return nil, err
	}

	netL0, err := vnet.NewNet(&vnet.NetConfig{})
	if err != nil {
		return nil, err
	}

	if err = lan.AddNet(netL0); err != nil {
		return nil, err
	}

	if err = wan.AddRouter(lan); err != nil {
		return nil, err
	}

	if err = wan.Start(); err != nil {
		return nil, err
	}

	// Start server...
	credMap := map[string][]byte{"user": GenerateAuthKey("user", "pion.ly", "pass")}

	udpListener, err := net0.ListenPacket("udp4", "0.0.0.0:3478")
	if err != nil {
		return nil, err
	}

	server, err := NewServer(ServerConfig{
		AuthHandler: func(username, _ string, _ net.Addr) (key []byte, ok bool) {
			if pw, ok := credMap[username]; ok {
				return pw, true
			}
			return nil, false
		},
		Realm: "pion.ly",
		PacketConnConfigs: []PacketConnConfig{
			{
				PacketConn: udpListener,
				RelayAddressGenerator: &RelayAddressGeneratorNone{
					Address: "1.2.3.4",
					Net:     net0,
				},
			},
		},
		LoggerFactory: loggerFactory,
	})
	if err != nil {
		return nil, err
	}

	// Register host names
	err = wan.AddHost("stun.pion.ly", "1.2.3.4")
	if err != nil {
		return nil, err
	}
	err = wan.AddHost("turn.pion.ly", "1.2.3.4")
	if err != nil {
		return nil, err
	}
	err = wan.AddHost("echo.pion.ly", "1.2.3.5")
	if err != nil {
		return nil, err
	}

	return &VNet{
		wan:    wan,
		net0:   net0,
		net1:   net1,
		netL0:  netL0,
		server: server,
	}, nil
}

func TestServerVNet(t *testing.T) {
	lim := test.TimeOut(time.Second * 30)
	defer lim.Stop()

	report := test.CheckRoutines(t)
	defer report()

	loggerFactory := logging.NewDefaultLoggerFactory()
	log := loggerFactory.NewLogger("test")

	t.Run("SendBindingRequest", func(t *testing.T) {
		v, err := buildVNet()
		assert.NoError(t, err)
		defer func() {
			assert.NoError(t, v.Close())
		}()

		lconn, err := v.netL0.ListenPacket("udp4", "0.0.0.0:0")
		assert.NoError(t, err, "should succeed")
		defer func() {
			assert.NoError(t, lconn.Close())
		}()

		stunAddr := "1.2.3.4:3478"

		log.Debug("creating a client.")
		client, err := NewClient(&ClientConfig{
			STUNServerAddr: stunAddr,
			Conn:           lconn,
			LoggerFactory:  loggerFactory,
		})
		assert.NoError(t, err, "should succeed")
		assert.NoError(t, client.Listen(), "should succeed")
		defer client.Close()

		log.Debug("sending a binding request.")
		reflAddr, err := client.SendBindingRequest()
		assert.NoError(t, err)
		log.Debugf("mapped-address: %s", reflAddr)
		udpAddr, ok := reflAddr.(*net.UDPAddr)
		assert.True(t, ok)

		// The mapped-address should have IP address that was assigned
		// to the LAN router.
		assert.True(t, udpAddr.IP.Equal(net.IPv4(5, 6, 7, 8)), "should match")
	})
}

func TestConsumeSingleTURNFrame(t *testing.T) {
	type testCase struct {
		data []byte
		err  error
	}
	cases := map[string]testCase{
		"channel data":                          {data: []byte{0x40, 0x01, 0x00, 0x08, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}, err: nil},
		"partial data less than channel header": {data: []byte{1}, err: errIncompleteTURNFrame},
		"partial stun message":                  {data: []byte{0x0, 0x16, 0x02, 0xDC, 0x21, 0x12, 0xA4, 0x42, 0x0, 0x0, 0x0}, err: errIncompleteTURNFrame},
		"stun message":                          {data: []byte{0x0, 0x16, 0x00, 0x02, 0x21, 0x12, 0xA4, 0x42, 0xf7, 0x43, 0x81, 0xa3, 0xc9, 0xcd, 0x88, 0x89, 0x70, 0x58, 0xac, 0x73, 0x0, 0x0}},
	}

	for name, cs := range cases {
		c := cs
		t.Run(name, func(t *testing.T) {
			n, e := consumeSingleTURNFrame(c.data)
			assert.Equal(t, c.err, e)
			if e == nil {
				assert.Equal(t, len(c.data), n)
			}
		})
	}
}

func TestSTUNOnly(t *testing.T) {
	serverAddr, err := net.ResolveUDPAddr("udp4", "0.0.0.0:3478")
	assert.NoError(t, err)

	serverConn, err := net.ListenPacket(serverAddr.Network(), serverAddr.String())
	assert.NoError(t, err)

	defer serverConn.Close() //nolint:errcheck

	server, err := NewServer(ServerConfig{
		PacketConnConfigs: []PacketConnConfig{{
			PacketConn: serverConn,
		}},
		Realm:         "pion.ly",
		LoggerFactory: logging.NewDefaultLoggerFactory(),
	})
	assert.NoError(t, err)

	defer server.Close() //nolint:errcheck

	conn, err := net.ListenPacket("udp4", "0.0.0.0:0")
	assert.NoError(t, err)

	client, err := NewClient(&ClientConfig{
		Conn:           conn,
		STUNServerAddr: "127.0.0.1:3478",
		TURNServerAddr: "127.0.0.1:3478",
		Username:       "user",
		Password:       "pass",
		Realm:          "pion.ly",
		LoggerFactory:  logging.NewDefaultLoggerFactory(),
	})
	assert.NoError(t, err)
	assert.NoError(t, client.Listen())
	defer client.Close()

	reflAddr, err := client.SendBindingRequest()
	assert.NoError(t, err)

	_, ok := reflAddr.(*net.UDPAddr)
	assert.True(t, ok)

	_, err = client.Allocate()
	assert.Equal(t, err.Error(), "Allocate error response (error 400: )")
}

func RunBenchmarkServer(b *testing.B, clientNum int) {
	loggerFactory := logging.NewDefaultLoggerFactory()
	credMap := map[string][]byte{
		"user": GenerateAuthKey("user", "pion.ly", "pass"),
	}

	testSeq := []byte("benchmark-data")

	// Setup server
	serverAddr, err := net.ResolveUDPAddr("udp4", "0.0.0.0:3478")
	if err != nil {
		b.Fatalf("Failed to resolve server address: %s", err)
	}

	serverConn, err := net.ListenPacket(serverAddr.Network(), serverAddr.String())
	if err != nil {
		b.Fatalf("Failed to allocate server listener at %s:%s", serverAddr.Network(), serverAddr.String())
	}
	defer serverConn.Close() //nolint:errcheck

	server, err := NewServer(ServerConfig{
		AuthHandler: func(username, _ string, _ net.Addr) (key []byte, ok bool) {
			if pw, ok := credMap[username]; ok {
				return pw, true
			}
			return nil, false
		},
		PacketConnConfigs: []PacketConnConfig{{
			PacketConn: serverConn,
			RelayAddressGenerator: &RelayAddressGeneratorStatic{
				RelayAddress: net.ParseIP("127.0.0.1"),
				Address:      "0.0.0.0",
			},
		}},
		Realm:         "pion.ly",
		LoggerFactory: loggerFactory,
	})
	if err != nil {
		b.Fatalf("Failed to start server: %s", err)
	}
	defer server.Close() //nolint:errcheck

	// Create a sink
	sinkAddr, err := net.ResolveUDPAddr("udp4", "0.0.0.0:65432")
	if err != nil {
		b.Fatalf("Failed to resolve sink address: %s", err)
	}

	sink, err := net.ListenPacket(sinkAddr.Network(), sinkAddr.String())
	if err != nil {
		b.Fatalf("Failed to allocate sink: %s", err)
	}
	defer sink.Close() //nolint:errcheck

	go func() {
		buf := make([]byte, 1600)
		for {
			// Ignore "use of closed network connection" errors
			if _, _, listenErr := sink.ReadFrom(buf); listenErr != nil {
				return
			}

			// Do not care about received data
		}
	}()

	// Setup client(s)
	clients := make([]net.PacketConn, clientNum)
	for i := 0; i < clientNum; i++ {
		clientConn, listenErr := net.ListenPacket("udp4", "0.0.0.0:0")
		if listenErr != nil {
			b.Fatalf("Failed to allocate socket for client %d: %s", i+1, err)
		}
		defer clientConn.Close() //nolint:errcheck

		client, err := NewClient(&ClientConfig{
			STUNServerAddr: "127.0.0.1:3478",
			TURNServerAddr: "127.0.0.1:3478",
			Conn:           clientConn,
			Username:       "user",
			Password:       "pass",
			Realm:          "pion.ly",
			LoggerFactory:  loggerFactory,
		})
		if err != nil {
			b.Fatalf("Failed to start client %d: %s", i+1, err)
		}
		defer client.Close()

		if listenErr := client.Listen(); listenErr != nil {
			b.Fatalf("Client %d cannot listen: %s", i+1, listenErr)
		}

		// Create an allocation
		turnConn, err := client.Allocate()
		if err != nil {
			b.Fatalf("Client %d cannot create allocation: %s", i+1, err)
		}
		defer turnConn.Close() //nolint:errcheck

		clients[i] = turnConn
	}

	// Run benchmark
	for j := 0; j < b.N; j++ {
		for i := 0; i < clientNum; i++ {
			if _, err := clients[i].WriteTo(testSeq, sinkAddr); err != nil {
				b.Fatalf("Client %d cannot send to TURN server: %s", i+1, err)
			}
		}
	}
}

// BenchmarkServer will benchmark the server with multiple simultaneous client connections
func BenchmarkServer(b *testing.B) {
	for i := 1; i <= 4; i++ {
		b.Run(fmt.Sprintf("client_num_%d", i), func(b *testing.B) {
			RunBenchmarkServer(b, i)
		})
	}
}
