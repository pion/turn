package turn

import (
	"net"
	"testing"
	"time"

	"github.com/gortc/turn"
	"github.com/pion/logging"
	"github.com/stretchr/testify/assert"
)

func TestServer(t *testing.T) {
	loggerFactory := logging.NewDefaultLoggerFactory()
	log := loggerFactory.NewLogger("test")

	credMap := map[string]string{
		"user": "pass",
	}

	t.Run("simple", func(t *testing.T) {

		server := NewServer(&ServerConfig{
			AuthHandler: func(username string, srcAddr net.Addr) (password string, ok bool) {
				if pw, ok := credMap[username]; ok {
					return pw, true
				}
				return "", false
			},
			Realm:         "pion.ly",
			LoggerFactory: loggerFactory,
		})

		assert.Equal(t, turn.DefaultLifetime, server.channelBindTimeout, "should match")

		err := server.AddListeningIPAddr("127.0.0.1")
		assert.NoError(t, err, "should succeed")

		log.Debug("start listening...")
		err = server.Start()
		assert.NoError(t, err, "should succeed")

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
		to := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 3478}
		resp, err := client.SendBindingRequestTo(to)
		assert.NoError(t, err, "should succeed")
		t.Logf("resp: %v", resp)

		log.Debug("now closing the server...")

		// Close server
		err = server.Close()
		assert.NoError(t, err, "should succeed")
	})

	t.Run("Relay IPs default to the listening IPs", func(t *testing.T) {
		server := NewServer(&ServerConfig{
			AuthHandler: func(username string, srcAddr net.Addr) (password string, ok bool) {
				if pw, ok := credMap[username]; ok {
					return pw, true
				}
				return "", false
			},
			Realm:         "pion.ly",
			LoggerFactory: loggerFactory,
		})

		assert.Equal(t, turn.DefaultLifetime, server.channelBindTimeout, "should match")

		err := server.AddListeningIPAddr("127.0.0.1")
		assert.NoError(t, err, "should succeed")

		log.Debug("start listening...")
		err = server.Start()
		assert.NoError(t, err, "should succeed")

		// make sure the server is listening before running
		// the client.
		time.Sleep(100 * time.Microsecond)

		assert.Equal(t, 1, len(server.relayIPs), "should match")
		assert.True(t, server.relayIPs[0].Equal(net.IPv4(127, 0, 0, 1)), "should match")

		// Close server
		err = server.Close()
		assert.NoError(t, err, "should succeed")
	})

	t.Run("AddRelayIPAddr", func(t *testing.T) {
		server := NewServer(&ServerConfig{
			AuthHandler: func(username string, srcAddr net.Addr) (password string, ok bool) {
				if pw, ok := credMap[username]; ok {
					return pw, true
				}
				return "", false
			},
			Realm:         "pion.ly",
			LoggerFactory: loggerFactory,
		})

		assert.Equal(t, turn.DefaultLifetime, server.channelBindTimeout, "should match")

		err := server.AddListeningIPAddr("127.0.0.1")
		assert.NoError(t, err, "should succeed")

		err = server.AddRelayIPAddr("127.0.0.2")
		assert.NoError(t, err, "should succeed")

		err = server.AddRelayIPAddr("127.0.0.3")
		assert.NoError(t, err, "should succeed")

		log.Debug("start listening...")
		err = server.Start()
		assert.NoError(t, err, "should succeed")

		// make sure the server is listening before running
		// the client.
		time.Sleep(100 * time.Microsecond)

		assert.Equal(t, 2, len(server.relayIPs), "should match")
		assert.True(t, server.relayIPs[0].Equal(net.IPv4(127, 0, 0, 2)), "should match")
		assert.True(t, server.relayIPs[1].Equal(net.IPv4(127, 0, 0, 3)), "should match")

		// Close server
		err = server.Close()
		assert.NoError(t, err, "should succeed")
	})

	t.Run("Adds SOFTWARE attribute to response", func(t *testing.T) {
		const testSoftware = "SERVER_SOFTWARE"
		cfg := &ServerConfig{
			AuthHandler: func(username string, srcAddr net.Addr) (password string, ok bool) {
				if pw, ok := credMap[username]; ok {
					return pw, true
				}
				return "", false
			},
			Realm:         "pion.ly",
			Software:      testSoftware,
			LoggerFactory: loggerFactory,
		}

		server := NewServer(cfg)

		err := server.AddListeningIPAddr("127.0.0.1")
		assert.NoError(t, err, "should succeed")

		log.Debug("start listening...")
		err = server.Start()
		assert.NoError(t, err, "should succeed")

		// make sure the server is listening before running
		// the client.
		time.Sleep(100 * time.Microsecond)

		lconn, err := net.ListenPacket("udp4", "0.0.0.0:0")
		if !assert.NoError(t, err, "should succeed") {
			return
		}
		defer lconn.Close() // nolint:errcheck,gosec

		log.Debug("creating a client.")
		client, err := NewClient(&ClientConfig{
			Conn:          lconn,
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
		resp, err := client.SendBindingRequestTo(&net.UDPAddr{
			IP:   net.IPv4(127, 0, 0, 1),
			Port: 3478,
		})
		assert.NoError(t, err, "should succeed")
		t.Logf("resp: %v", resp)

		log.Debug("now closing the server...")

		// Close server
		err = server.Close()
		assert.NoError(t, err, "should succeed")
	})
}
