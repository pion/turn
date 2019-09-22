package turn

import (
	"net"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/transport/vnet"
	"github.com/stretchr/testify/assert"
)

func createListeningTestClient(t *testing.T, loggerFactory logging.LoggerFactory) (*Client, bool) {
	conn, err := net.ListenPacket("udp4", "0.0.0.0:0")
	if !assert.NoError(t, err, "should succeed") {
		return nil, false
	}
	c, err := NewClient(&ClientConfig{
		Conn:          conn,
		Software:      "TEST SOFTWARE",
		LoggerFactory: loggerFactory,
	})
	if !assert.NoError(t, err, "should succeed") {
		return nil, false
	}
	err = c.Listen()
	if !assert.NoError(t, err, "should succeed") {
		return nil, false
	}

	return c, true
}

func createListeningTestClientWithSTUNServ(t *testing.T, loggerFactory logging.LoggerFactory) (*Client, bool) {
	conn, err := net.ListenPacket("udp4", "0.0.0.0:0")
	if !assert.NoError(t, err, "should succeed") {
		return nil, false
	}
	c, err := NewClient(&ClientConfig{
		STUNServerAddr: "stun1.l.google.com:19302",
		Conn:           conn,
		Net:            vnet.NewNet(nil),
		LoggerFactory:  loggerFactory,
	})
	if !assert.NoError(t, err, "should succeed") {
		return nil, false
	}
	err = c.Listen()
	if !assert.NoError(t, err, "should succeed") {
		return nil, false
	}

	return c, true
}

func TestClientWithSTUN(t *testing.T) {
	loggerFactory := logging.NewDefaultLoggerFactory()
	log := loggerFactory.NewLogger("test")

	t.Run("SendBindingRequest", func(t *testing.T) {
		c, ok := createListeningTestClientWithSTUNServ(t, loggerFactory)
		if !ok {
			return
		}
		defer c.Close()

		resp, err := c.SendBindingRequest()
		assert.NoError(t, err, "should succeed")
		log.Debugf("mapped-addr: %s", resp.String())
		assert.Equal(t, 0, c.trMap.Size(), "should be no transaction left")
	})

	t.Run("SendBindingRequestTo Parallel", func(t *testing.T) {
		c, ok := createListeningTestClient(t, loggerFactory)
		if !ok {
			return
		}
		defer c.Close()

		// simple channel fo go routine start signaling
		started := make(chan struct{})
		finished := make(chan struct{})
		var err1 error
		var resp1 interface{}

		to, err := net.ResolveUDPAddr("udp4", "stun1.l.google.com:19302")
		if !assert.NoError(t, err, "should succeed") {
			return
		}

		// stun1.l.google.com:19302, more at https://gist.github.com/zziuni/3741933#file-stuns-L5
		go func() {
			close(started)
			resp1, err1 = c.SendBindingRequestTo(to)
			close(finished)
		}()

		// block until go routine is started to make two almost parallel requests

		<-started

		resp2, err2 := c.SendBindingRequestTo(to)
		if err2 != nil {
			t.Fatal(err)
		} else {
			t.Log(resp2)
		}

		<-finished
		if err1 != nil {
			t.Fatal(err)
		} else {
			t.Log(resp1)
		}
	})

	t.Run("NewClient should fail if Conn is nil", func(t *testing.T) {
		_, err := NewClient(&ClientConfig{
			LoggerFactory: loggerFactory,
		})
		assert.Error(t, err, "should fail")
	})

	t.Run("SendBindingRequestTo timeout", func(t *testing.T) {
		c, ok := createListeningTestClient(t, loggerFactory)
		if !ok {
			return
		}
		defer c.Close()

		to, err := net.ResolveUDPAddr("udp4", "127.0.0.1:9")
		if !assert.NoError(t, err, "should succeed") {
			return
		}

		c.rto = 10 * time.Millisecond // force short timeout

		_, err = c.SendBindingRequestTo(to)
		log.Debug(err.Error())
	})
}
