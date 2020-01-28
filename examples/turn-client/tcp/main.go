package main

import (
	"flag"
	"fmt"
	"github.com/pion/turn/v2/internal/proto"
	"log"
	"net"
	"strings"
	"time"

	"github.com/pion/logging"
	"github.com/pion/turn/v2"
)

func main() {
	turnHost := flag.String("turnHost", "", "TURN Server name.")
	turnPort := flag.Int("turnPort", 3478, "Listening turnPort.")
	user := flag.String("user", "", "A pair of username and password (e.g. \"user=pass\")")
	realm := flag.String("realm", "pion.ly", "Realm (defaults to \"pion.ly\")")
	ping := flag.Bool("ping", false, "Run ping test")
	peerHost := flag.String("peerHost", "", "Peer Host")
	peerPort := flag.Int("peerPort", 8080, "Peer Port.")
	flag.Parse()

	if len(*turnHost) == 0 {
		log.Fatalf("'turnHost' is required")
	}

	if len(*user) == 0 {
		log.Fatalf("'user' is required")
	}

	if len(*peerHost) == 0 {
		log.Fatalf("'peerHost' is required")
	}

	// Dial TURN Server
	turnServerString := fmt.Sprintf("%s:%d", *turnHost, *turnPort)

	coturnAddr, err := net.ResolveTCPAddr("tcp4", turnServerString)
	if err != nil {
		log.Fatal(err)
	}

	conn, err := net.DialTCP("tcp",nil, coturnAddr)
	if err != nil {
		panic(err)
	}

	cred := strings.Split(*user, "=")

	// Start a new TURN Client and wrap our net.Conn in a STUNConn
	// This allows us to simulate datagram based communication over a net.Conn
	cfg := &turn.ClientConfig{
		STUNServerAddr: turnServerString,
		TURNServerAddr: turnServerString,
		Conn:           turn.NewSTUNConn(conn),
		Username:       cred[0],
		Password:       cred[1],
		Realm:          *realm,
		LoggerFactory:  logging.NewDefaultLoggerFactory(),
		TransportProtocol: proto.ProtoTCP,
	}

	client, err := turn.NewClient(cfg)
	if err != nil {
		panic(err)
	}
	defer client.Close()

	// Start listening on the conn provided.
	err = client.Listen()
	if err != nil {
		panic(err)
	}

	// Allocate a relay socket on the TURN server. On success, it
	// will return a net.PacketConn which represents the remote
	// socket.
	relayConn, err := client.Allocate()
	if err != nil {
		panic(err)
	}
	defer func() {
		if closeErr := relayConn.Close(); closeErr != nil {
			panic(closeErr)
		}
	}()

	//// The relayConn's local address is actually the transport
	//// address assigned on the TURN server.
	log.Printf("relayed-address=%s", relayConn.LocalAddr().String())
	log.Printf("relayed-protocol=%s", relayConn.LocalAddr().Network())

	peerAddress, err := net.ResolveTCPAddr("tcp4", fmt.Sprintf("%s:%d", *peerHost, *peerPort))
	if err != nil {
		panic(err)
	}

	connectionId, err := client.SendConnectRequest(*peerAddress)
	if err != nil {
		panic(err)
	}
	log.Printf("connectionId=%d", connectionId)

	dataConnection, err := client.SendConnectionBindRequest(connectionId)
	if err != nil {
		panic(err)
	}
	log.Printf("dataConnection=%s", dataConnection)

	// If you provided `-ping`, perform a ping test agaist the
	// relayConn we have just allocated.
	if *ping {
		//err = doPingTest(client, relayConn)
		//if err != nil {
		//	panic(err)
		//}
	} else {
		for i := 0; i < 120 ; i++  {
			time.Sleep(1 *time.Second)
		}
	}
}

func doPingTest(client *turn.Client, relayConn net.PacketConn) error {
	// Send BindingRequest to learn our external IP
	mappedAddr, err := client.SendBindingRequest()
	if err != nil {
		return err
	}

	// Set up pinger socket (pingerConn)
	pingerConn, err := net.ListenPacket("udp4", "0.0.0.0:0")
	if err != nil {
		panic(err)
	}
	defer func() {
		if closeErr := pingerConn.Close(); closeErr != nil {
			panic(closeErr)
		}
	}()

	// Punch a UDP hole for the relayConn by sending a data to the mappedAddr.
	// This will trigger a TURN client to generate a permission request to the
	// TURN server. After this, packets from the IP address will be accepted by
	// the TURN server.
	_, err = relayConn.WriteTo([]byte("Hello"), mappedAddr)
	if err != nil {
		return err
	}

	// Start read-loop on pingerConn
	go func() {
		buf := make([]byte, 1500)
		for {
			n, from, pingerErr := pingerConn.ReadFrom(buf)
			if pingerErr != nil {
				break
			}

			msg := string(buf[:n])
			if sentAt, pingerErr := time.Parse(time.RFC3339Nano, msg); pingerErr == nil {
				rtt := time.Since(sentAt)
				log.Printf("%d bytes from from %s time=%d ms\n", n, from.String(), int(rtt.Seconds()*1000))
			}
		}
	}()

	// Start read-loop on relayConn
	go func() {
		buf := make([]byte, 1500)
		for {
			n, from, readerErr := relayConn.ReadFrom(buf)
			if readerErr != nil {
				break
			}

			// Echo back
			if _, readerErr = relayConn.WriteTo(buf[:n], from); readerErr != nil {
				break
			}
		}
	}()

	time.Sleep(500 * time.Millisecond)

	// Send 10 packets from relayConn to the echo server
	for i := 0; i < 10; i++ {
		msg := time.Now().Format(time.RFC3339Nano)
		_, err = pingerConn.WriteTo([]byte(msg), relayConn.LocalAddr())
		if err != nil {
			return err
		}

		// For simplicity, this example does not wait for the pong (reply).
		// Instead, sleep 1 second.
		time.Sleep(time.Second)
	}

	return nil
}
