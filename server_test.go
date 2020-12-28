// +build !js

package turn

import (
	"net"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/transport/test"
	"github.com/pion/transport/vnet"
	"github.com/pion/turn/v2/internal/proto"
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
			AuthHandler: func(username, realm string, srcAddr net.Addr) (key []byte, ok bool) {
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

	net0 := vnet.NewNet(&vnet.NetConfig{
		StaticIP: "1.2.3.4", // will be assigned to eth0
	})

	err = wan.AddNet(net0)
	if err != nil {
		return nil, err
	}

	net1 := vnet.NewNet(&vnet.NetConfig{
		StaticIP: "1.2.3.5", // will be assigned to eth0
	})

	err = wan.AddNet(net1)
	if err != nil {
		return nil, err
	}

	// LAN
	lan, err := vnet.NewRouter(&vnet.RouterConfig{
		StaticIP: "5.6.7.8", // this router's external IP on eth0
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

	netL0 := vnet.NewNet(&vnet.NetConfig{})

	if err = lan.AddNet(netL0); err != nil {
		return nil, err
	}

	if err = wan.AddRouter(lan); err != nil {
		return nil, err
	}

	if err = wan.Start(); err != nil {
		return nil, err
	}

	// start server...
	credMap := map[string][]byte{"user": GenerateAuthKey("user", "pion.ly", "pass")}

	udpListener, err := net0.ListenPacket("udp4", "0.0.0.0:3478")
	if err != nil {
		return nil, err
	}

	server, err := NewServer(ServerConfig{
		AuthHandler: func(username, realm string, srcAddr net.Addr) (key []byte, ok bool) {
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

	// register host names
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

		log.Debug("creating a client.")
		client, err := NewClient(&ClientConfig{
			STUNServerAddr: "1.2.3.4:3478",
			Conn:           lconn,
			Net:            v.netL0,
			LoggerFactory:  loggerFactory,
		})
		assert.NoError(t, err, "should succeed")
		assert.NoError(t, client.Listen(), "should succeed")
		defer client.Close()

		log.Debug("sending a binding request.")
		reflAddr, err := client.SendBindingRequest()
		assert.NoError(t, err)
		log.Debugf("mapped-address: %v", reflAddr.String())
		udpAddr := reflAddr.(*net.UDPAddr)

		// The mapped-address should have IP address that was assigned
		// to the LAN router.
		assert.True(t, udpAddr.IP.Equal(net.IPv4(5, 6, 7, 8)), "should match")
	})

	t.Run("Echo via relay", func(t *testing.T) {
		v, err := buildVNet()
		assert.NoError(t, err)

		lconn, err := v.netL0.ListenPacket("udp4", "0.0.0.0:0")
		assert.NoError(t, err)

		log.Debug("creating a client.")
		client, err := NewClient(&ClientConfig{
			STUNServerAddr: "stun.pion.ly:3478",
			TURNServerAddr: "turn.pion.ly:3478",
			Username:       "user",
			Password:       "pass",
			Conn:           lconn,
			Net:            v.netL0,
			LoggerFactory:  loggerFactory,
		})

		assert.NoError(t, err)
		assert.NoError(t, client.Listen())

		log.Debug("sending a binding request.")
		conn, err := client.Allocate()
		assert.NoError(t, err)

		log.Debugf("laddr: %s", conn.LocalAddr().String())

		echoConn, err := v.net1.ListenPacket("udp4", "1.2.3.5:5678")
		assert.NoError(t, err)

		go func() {
			buf := make([]byte, 1600)
			for {
				n, from, err2 := echoConn.ReadFrom(buf)
				if err2 != nil {
					break
				}

				// verify the message was received from the relay address
				assert.Equal(t, conn.LocalAddr().String(), from.String(), "should match")
				assert.Equal(t, "Hello", string(buf[:n]), "should match")

				// echo the data
				_, err2 = echoConn.WriteTo(buf[:n], from)
				assert.NoError(t, err2)
			}
		}()

		buf := make([]byte, 1600)

		for i := 0; i < 10; i++ {
			log.Debug("sending \"Hello\"..")
			_, err = conn.WriteTo([]byte("Hello"), echoConn.LocalAddr())
			assert.NoError(t, err)

			_, from, err2 := conn.ReadFrom(buf)
			assert.NoError(t, err2)

			// verify the message was received from the relay address
			assert.Equal(t, echoConn.LocalAddr().String(), from.String(), "should match")

			time.Sleep(100 * time.Millisecond)
		}

		time.Sleep(100 * time.Millisecond)
		client.Close()

		assert.NoError(t, conn.Close(), "should succeed")
		assert.NoError(t, echoConn.Close(), "should succeed")
		assert.NoError(t, lconn.Close(), "should succeed")
		assert.NoError(t, v.Close(), "should succeed")
	})
}
