package turn

import (
	"net"
	"testing"

	"github.com/pion/logging"
)

func TestClient(t *testing.T) {
	loggerFactory := logging.NewDefaultLoggerFactory()

	t.Run("SendSTUNRequest Parallel", func(t *testing.T) {
		c, err := NewClient(&ClientConfig{
			ListeningAddress: "0.0.0.0:0",
			LoggerFactory:    loggerFactory,
		})
		if err != nil {
			t.Fatal(err)
		}

		// simple channel fo go routine start signaling
		started := make(chan struct{})
		finished := make(chan struct{})
		var err1 error
		var resp1 interface{}

		// stun1.l.google.com:19302, more at https://gist.github.com/zziuni/3741933#file-stuns-L5
		go func() {
			close(started)
			resp1, err1 = c.SendSTUNRequest(net.IPv4(74, 125, 143, 127), 19302)
			close(finished)
		}()

		// block until go routine is started to make two almost parallel requests

		<-started

		resp2, err2 := c.SendSTUNRequest(net.IPv4(74, 125, 143, 127), 19302)
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

	t.Run("Listen error", func(t *testing.T) {
		_, err := NewClient(&ClientConfig{
			ListeningAddress: "255.255.255.256:65535",
			LoggerFactory:    loggerFactory,
		})
		if err == nil {
			t.Fatal("listening on 255.255.255.256:65535 should fail")
		}
	})

	/*
		// Unable to perform this test atm because there is no timeout and the test may run infinitely
		t.Run("SendSTUNRequest timeout", func(t *testing.T) {
			c, err := NewClient("0.0.0.0:0")
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.SendSTUNRequest(net.IPv4(255, 255, 255, 255), 65535)
			if err == nil {
				t.Fatal("request to 255.255.255.255:65535 should fail")
			}
		})
	*/
}
