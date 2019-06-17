package turn

import (
	"net"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/stretchr/testify/assert"
)

func TestServer(t *testing.T) {
	loggerFactory := logging.NewDefaultLoggerFactory()
	log := loggerFactory.NewLogger("test")

	t.Run("simple", func(t *testing.T) {
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
			LoggerFactory: loggerFactory,
		})

		doneCh := make(chan struct{})

		go func() {
			log.Debug("start listening...")
			err := server.Listen("127.0.0.1", 3478)
			if err != nil {
				t.Logf("Listen returned with err: %v", err)
			}
			close(doneCh)
		}()

		// make sure the server is listening before running
		// the client.
		time.Sleep(100 * time.Microsecond)

		log.Debug("creating a client.")
		client, err := NewClient(&ClientConfig{
			ListeningAddress: "0.0.0.0:0",
			LoggerFactory:    loggerFactory,
		})
		if !assert.NoError(t, err, "should succeed") {
			return
		}

		log.Debug("sending a binding request.")
		resp, err := client.SendSTUNRequest(net.IPv4(127, 0, 0, 1), 3478)
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
