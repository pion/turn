package turn

import (
	"net"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/transport/vnet"
	"github.com/stretchr/testify/assert"
)

func buildVNet() (*vnet.Router, *vnet.Net, *vnet.Net, error) {
	loggerFactory := logging.NewDefaultLoggerFactory()

	// WAN
	wan, err := vnet.NewRouter(&vnet.RouterConfig{
		CIDR:          "0.0.0.0/0",
		LoggerFactory: loggerFactory,
	})
	if err != nil {
		return nil, nil, nil, err
	}

	wanNet := vnet.NewNet(&vnet.NetConfig{
		StaticIP: "1.2.3.4", // will be assigned to eth0
	})

	err = wan.AddNet(wanNet)
	if err != nil {
		return nil, nil, nil, err
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
		return nil, nil, nil, err
	}

	lanNet := vnet.NewNet(&vnet.NetConfig{})
	err = lan.AddNet(lanNet)
	if err != nil {
		return nil, nil, nil, err
	}

	err = wan.AddRouter(lan)
	if err != nil {
		return nil, nil, nil, err
	}

	err = wan.Start()
	if err != nil {
		return nil, nil, nil, err
	}

	return wan, wanNet, lanNet, nil
}

func TestServerVNet(t *testing.T) {
	loggerFactory := logging.NewDefaultLoggerFactory()
	log := loggerFactory.NewLogger("test")

	t.Run("simple", func(t *testing.T) {
		wan, wanNet, lanNet, err := buildVNet()
		assert.NoError(t, err, "should succeed")
		defer wan.Stop() // nolint:errcheck

		credMap := map[string]string{}
		credMap["user"] = "pass"

		server := NewServer(&ServerConfig{
			AuthHandler: func(username string, srcAddr net.Addr) (password string, ok bool) {
				if pw, ok := credMap[username]; ok {
					return pw, true
				}
				return "", false
			},
			Realm:         "pion.ly",
			Net:           wanNet,
			LoggerFactory: loggerFactory,
		})

		err = server.AddListeningIPAddr("1.2.3.4")
		assert.NoError(t, err, "should succeed")

		doneCh := make(chan struct{})

		go func() {
			log.Debug("start listening...")
			err2 := server.Start()
			if err2 != nil {
				t.Logf("Start returned with err: %v", err2)
			}
			close(doneCh)
		}()

		// make sure the server is listening before running
		// the client.
		time.Sleep(100 * time.Microsecond)

		log.Debug("creating a client.")
		client, err := NewClient(&ClientConfig{
			ListeningAddress: "0.0.0.0:0",
			Net:              lanNet,
			LoggerFactory:    loggerFactory,
		})
		if !assert.NoError(t, err, "should succeed") {
			return
		}

		log.Debug("sending a binding request.")
		reflAddr, err := client.SendSTUNRequest(net.IPv4(1, 2, 3, 4), 3478)
		if !assert.NoError(t, err, "should succeed") {
			return
		}
		log.Debugf("mapped-address: %v", reflAddr.String())
		udpAddr := reflAddr.(*net.UDPAddr)

		// The mapped-address should have IP address that was assigned
		// to the LAN router.
		assert.True(t, udpAddr.IP.Equal(net.IPv4(5, 6, 7, 8)), "should match")

		// Close server
		err = server.Close()
		assert.NoError(t, err, "should succeed")

		<-doneCh
	})
}
