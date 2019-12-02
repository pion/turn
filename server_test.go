package turn

import (
	"net"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/transport/vnet"
	"github.com/pion/turn/internal/proto"
	"github.com/stretchr/testify/assert"
)

func TestServer(t *testing.T) {
	loggerFactory := logging.NewDefaultLoggerFactory()
	log := loggerFactory.NewLogger("test")

	credMap := map[string]string{
		"user": "pass",
	}

	t.Run("simple", func(t *testing.T) {
		udpListener, err := net.ListenPacket("udp4", "0.0.0.0:3478")
		if err != nil {
			t.Fatalf("Failed to create TURN server listener: %s", err)
		}

		server, err := NewServer(ServerConfig{
			AuthHandler: func(username string, srcAddr net.Addr) (password string, ok bool) {
				if pw, ok := credMap[username]; ok {
					return pw, true
				}
				return "", false
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

		// make sure the server is listening before running
		// the client.
		time.Sleep(100 * time.Microsecond)

		log.Debug("creating a client.")
		conn, err := net.ListenPacket("udp4", "0.0.0.0:0")
		if !assert.NoError(t, err, "should succeed") {
			return
		}
		client, err := NewClient(&ClientConfig{
			Conn:          conn,
			LoggerFactory: loggerFactory,
		})
		if !assert.NoError(t, err, "should succeed") {
			return
		}
		err = client.Listen()
		if !assert.NoError(t, err, "should succeed") {
			return
		}
		defer client.Close()

		log.Debug("sending a binding request.")

		_, err = client.SendBindingRequestTo(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 3478})
		assert.NoError(t, err, "should succeed")

		log.Debug("now closing the server...")

		// Close server
		err = server.Close()
		assert.NoError(t, err, "should succeed")
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
	err := v.server.Close()
	v.wan.Stop()
	if err != nil {
		return err
	}
	return nil
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
	credMap := map[string]string{}
	credMap["user"] = "pass"

	udpListener, err := net0.ListenPacket("udp4", "0.0.0.0:3478")
	if err != nil {
		return nil, err
	}

	server, err := NewServer(ServerConfig{
		AuthHandler: func(username string, srcAddr net.Addr) (password string, ok bool) {
			if pw, ok := credMap[username]; ok {
				return pw, true
			}
			return "", false
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
	loggerFactory := logging.NewDefaultLoggerFactory()
	log := loggerFactory.NewLogger("test")

	t.Run("SendBindingRequest", func(t *testing.T) {
		v, err := buildVNet()
		if !assert.NoError(t, err, "should succeed") {
			return
		}
		defer v.Close()

		lconn, err := v.netL0.ListenPacket("udp4", "0.0.0.0:0")
		if !assert.NoError(t, err, "should succeed") {
			return
		}
		defer lconn.Close()

		log.Debug("creating a client.")
		client, err := NewClient(&ClientConfig{
			STUNServerAddr: "1.2.3.4:3478",
			Conn:           lconn,
			Net:            v.netL0,
			LoggerFactory:  loggerFactory,
		})
		if !assert.NoError(t, err, "should succeed") {
			return
		}
		err = client.Listen()
		if !assert.NoError(t, err, "should succeed") {
			return
		}
		defer client.Close()

		log.Debug("sending a binding request.")
		reflAddr, err := client.SendBindingRequest()
		if !assert.NoError(t, err, "should succeed") {
			return
		}
		log.Debugf("mapped-address: %v", reflAddr.String())
		udpAddr := reflAddr.(*net.UDPAddr)

		// The mapped-address should have IP address that was assigned
		// to the LAN router.
		assert.True(t, udpAddr.IP.Equal(net.IPv4(5, 6, 7, 8)), "should match")
	})

	t.Run("Echo via relay", func(t *testing.T) {
		v, err := buildVNet()
		if !assert.NoError(t, err, "should succeed") {
			return
		}

		lconn, err := v.netL0.ListenPacket("udp4", "0.0.0.0:0")
		if !assert.NoError(t, err, "should succeed") {
			return
		}

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
		if !assert.NoError(t, err, "should succeed") {
			return
		}
		err = client.Listen()
		if !assert.NoError(t, err, "should succeed") {
			return
		}

		log.Debug("sending a binding request.")
		conn, err := client.Allocate()
		if !assert.NoError(t, err, "should succeed") {
			return
		}

		log.Debugf("laddr: %s", conn.LocalAddr().String())

		echoConn, err := v.net1.ListenPacket("udp4", "1.2.3.5:5678")
		if !assert.NoError(t, err, "should succeed") {
			return
		}

		go func() {
			buf := make([]byte, 1500)
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
				if !assert.NoError(t, err2, "should succeed") {
					break
				}
			}
		}()

		buf := make([]byte, 1500)

		for i := 0; i < 10; i++ {
			log.Debug("sending \"Hello\"..")
			_, err = conn.WriteTo([]byte("Hello"), echoConn.LocalAddr())
			if !assert.NoError(t, err, "should succeed") {
				return
			}

			_, from, err2 := conn.ReadFrom(buf)
			assert.NoError(t, err2, "should succeed")

			// verify the message was received from the relay address
			assert.Equal(t, echoConn.LocalAddr().String(), from.String(), "should match")

			time.Sleep(100 * time.Millisecond)
		}

		time.Sleep(100 * time.Millisecond)
		err = conn.Close()
		assert.NoError(t, err, "should succeed")
		log.Debug("RELAY CONN CLOSED")

		err = echoConn.Close()
		assert.NoError(t, err, "should succeed")
		log.Debug("ECHO SERVER CLOSED")

		client.Close()
		log.Debug("TURN CLIENT CLOSED")

		err = lconn.Close()
		assert.NoError(t, err, "should succeed")
		log.Debug("LOCAL CONN CLOSED")

		err = v.Close()
		assert.NoError(t, err, "should succeed")
		log.Debug("VNET CLOSED")
	})
}
