// +build !js

package turn

import (
	"net"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/transport/vnet"
	"github.com/stretchr/testify/assert"
)

func createListeningTestClient(t *testing.T, loggerFactory logging.LoggerFactory) (*Client, net.PacketConn, bool) {
	conn, err := net.ListenPacket("udp4", "0.0.0.0:0")
	assert.NoError(t, err)

	c, err := NewClient(&ClientConfig{
		Conn:          conn,
		Software:      "TEST SOFTWARE",
		LoggerFactory: loggerFactory,
	})
	assert.NoError(t, err)
	assert.NoError(t, c.Listen())

	return c, conn, true
}

func createListeningTestClientWithSTUNServ(t *testing.T, loggerFactory logging.LoggerFactory) (*Client, net.PacketConn, bool) {
	conn, err := net.ListenPacket("udp4", "0.0.0.0:0")
	assert.NoError(t, err)

	c, err := NewClient(&ClientConfig{
		STUNServerAddr: "stun1.l.google.com:19302",
		Conn:           conn,
		Net:            vnet.NewNet(nil),
		LoggerFactory:  loggerFactory,
	})
	assert.NoError(t, err)
	assert.NoError(t, c.Listen())

	return c, conn, true
}

func TestClientWithSTUN(t *testing.T) {
	loggerFactory := logging.NewDefaultLoggerFactory()
	log := loggerFactory.NewLogger("test")

	t.Run("SendBindingRequest", func(t *testing.T) {
		c, pc, ok := createListeningTestClientWithSTUNServ(t, loggerFactory)
		if !ok {
			return
		}
		defer c.Close()

		resp, err := c.SendBindingRequest()
		assert.NoError(t, err, "should succeed")
		log.Debugf("mapped-addr: %s", resp.String())
		assert.Equal(t, 0, c.trMap.Size(), "should be no transaction left")
		assert.NoError(t, pc.Close())
	})

	t.Run("SendBindingRequestTo Parallel", func(t *testing.T) {
		c, pc, ok := createListeningTestClient(t, loggerFactory)
		if !ok {
			return
		}
		defer c.Close()

		// simple channel fo go routine start signaling
		started := make(chan struct{})
		finished := make(chan struct{})
		var err1 error

		to, err := net.ResolveUDPAddr("udp4", "stun1.l.google.com:19302")
		assert.NoError(t, err)

		// stun1.l.google.com:19302, more at https://gist.github.com/zziuni/3741933#file-stuns-L5
		go func() {
			close(started)
			_, err1 = c.SendBindingRequestTo(to)
			close(finished)
		}()

		// block until go routine is started to make two almost parallel requests

		<-started

		if _, err = c.SendBindingRequestTo(to); err != nil {
			t.Fatal(err)
		}

		<-finished
		if err1 != nil {
			t.Fatal(err)
		}

		assert.NoError(t, pc.Close())
	})

	t.Run("NewClient should fail if Conn is nil", func(t *testing.T) {
		_, err := NewClient(&ClientConfig{
			LoggerFactory: loggerFactory,
		})
		assert.Error(t, err, "should fail")
	})

	t.Run("SendBindingRequestTo timeout", func(t *testing.T) {
		c, pc, ok := createListeningTestClient(t, loggerFactory)
		if !ok {
			return
		}
		defer c.Close()

		to, err := net.ResolveUDPAddr("udp4", "127.0.0.1:9")
		assert.NoError(t, err)

		c.rto = 10 * time.Millisecond // force short timeout

		_, err = c.SendBindingRequestTo(to)
		assert.NotNil(t, err)
		assert.NoError(t, pc.Close())
	})
}

// Create an allocation, and then delete all nonces
// The subsequent Write on the allocation will cause a CreatePermission
// which will be forced to handle a stale nonce response
func TestClientNonceExpiration(t *testing.T) {
	// lim := test.TimeOut(time.Second * 30)
	// defer lim.Stop()

	// report := test.CheckRoutines(t)
	// defer report()

	udpListener, err := net.ListenPacket("udp4", "0.0.0.0:3478")
	assert.NoError(t, err)

	server, err := NewServer(ServerConfig{
		AuthHandler: func(username, realm string, srcAddr net.Addr) (key []byte, ok bool) {
			return GenerateAuthKey(username, realm, "pass"), true
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
		Realm: "pion.ly",
	})
	assert.NoError(t, err)

	conn, err := net.ListenPacket("udp4", "0.0.0.0:0")
	assert.NoError(t, err)

	client, err := NewClient(&ClientConfig{
		Conn:           conn,
		STUNServerAddr: "127.0.0.1:3478",
		TURNServerAddr: "127.0.0.1:3478",
		Username:       "foo",
		Password:       "pass",
	})
	assert.NoError(t, err)
	assert.NoError(t, client.Listen())

	allocation, err := client.Allocate()
	assert.NoError(t, err)

	server.nonces.Range(func(key, value interface{}) bool {
		server.nonces.Delete(key)
		return true
	})

	_, err = allocation.WriteTo([]byte{0x00}, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080})
	assert.NoError(t, err)

	// Shutdown
	assert.NoError(t, allocation.Close())
	assert.NoError(t, conn.Close())
	assert.NoError(t, server.Close())
}
