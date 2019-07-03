package server

import (
	"net"
	"testing"
	"time"

	"github.com/gortc/turn"
	"github.com/pion/logging"
	"github.com/stretchr/testify/assert"

	"github.com/pion/turn/client"
)

func TestServer(t *testing.T) {
	loggerFactory := logging.NewDefaultLoggerFactory()
	log := loggerFactory.NewLogger("test")

	t.Run("simple", func(t *testing.T) {
		credMap := map[string]string{}
		credMap["user"] = "pass"

		server := NewServer(&Config{
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
		stunClient, err := client.NewClient(&client.Config{
			ListeningAddress: "0.0.0.0:0",
			LoggerFactory:    loggerFactory,
		})
		if !assert.NoError(t, err, "should succeed") {
			return
		}

		log.Debug("sending a binding request.")
		resp, err := stunClient.SendSTUNRequest(net.IPv4(127, 0, 0, 1), 3478)
		assert.NoError(t, err, "should succeed")
		t.Logf("resp: %v", resp)

		log.Debug("now closing the server...")

		// Close server
		err = server.Close()
		assert.NoError(t, err, "should succeed")
		log.Debug("wainting the server to terminate...")
		<-doneCh
	})
}
