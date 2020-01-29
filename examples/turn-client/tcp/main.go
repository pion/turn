package main

import (
	"encoding/binary"
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

var (
	turnHost = flag.String("turnHost", "", "TURN Server name.")
	turnPort = flag.Int("turnPort", 3478, "Listening turnPort.")
	user = flag.String("user", "", "A pair of username and password (e.g. \"user=pass\")")
	realm = flag.String("realm", "pion.ly", "Realm (defaults to \"pion.ly\")")
	ping = flag.Bool("ping", false, "Run ping test")
	peerHost = flag.String("peerHost", "", "Peer Host")
	peerPort = flag.Int("peerPort", 8080, "Peer Port.")
)

func main() {
	flag.Parse()
	ValidateFlags(turnHost, user, peerHost)

	peerAddress, err := net.ResolveTCPAddr("tcp4", fmt.Sprintf("%s:%d", *peerHost, *peerPort))
	if err != nil {
		panic(err)
	}

	localControlAddr, err := net.ResolveTCPAddr("tcp4", ":8081")
	if err != nil {
		log.Fatal(err)
	}

	localDataAddr, err := net.ResolveTCPAddr("tcp4", ":8888")
	if err != nil {
		log.Fatal(err)
	}

	turnServerString := fmt.Sprintf("%s:%d", *turnHost, *turnPort)
	turnServerAddr, err := net.ResolveTCPAddr("tcp4", turnServerString)
	if err != nil {
		log.Fatal(err)
	}

	controlClient := CreateClient(
		localControlAddr,
		turnServerAddr,
		user,
		realm,
	)
	defer controlClient.Close()

	err = controlClient.Listen()
	if err != nil {
		panic(err)
	}

	relayConn := AllocateRequest(controlClient)
	defer HandleRelayConnClose(relayConn)

	CreatePermissionRequest(controlClient, peerAddress)
	connectionId := WaitForConnection(relayConn)

	dataClient := CreateClient(
		localDataAddr,
		turnServerAddr,
		user,
		realm,
	)

	err = dataClient.Listen()
	if err != nil {
		panic(err)
	}

	ConnectionBindRequest(dataClient, proto.ConnectionId(connectionId))

	for {
		bytesWritten, err := dataClient.WriteTo([]byte("Hello from Client"), localDataAddr)
		if err != nil {
			log.Fatal("Could not write to connection.")
		}

		if bytesWritten == 0 {
			log.Fatal("No bytes written.")
		}
		time.Sleep(1*time.Second)
	}
}

func WaitForConnection(relayConn net.PacketConn) uint32 {
	buf := make([]byte, 1500)
	n, _, readerErr := relayConn.ReadFrom(buf)
	if readerErr != nil {
		panic(readerErr)
	}

	fmt.Print("\nConnectionID:", binary.BigEndian.Uint32(buf[:n]))

	return binary.BigEndian.Uint32(buf[:n])
}

func CreateClient(localAddr, turnServerAddr *net.TCPAddr, user, realm *string) *turn.Client {

	conn, err := net.DialTCP("tcp", localAddr, turnServerAddr)
	if err != nil {
		panic(err)
	}

	cred := strings.Split(*user, "=")
	cfg := &turn.ClientConfig{
		STUNServerAddr:    turnServerAddr.String(),
		TURNServerAddr:    turnServerAddr.String(),
		Conn:              turn.NewSTUNConn(conn),
		Username:          cred[0],
		Password:          cred[1],
		Realm:             *realm,
		LoggerFactory:     logging.NewDefaultLoggerFactory(),
		TransportProtocol: proto.ProtoTCP,
	}

	client, err := turn.NewClient(cfg)
	if err != nil {
		panic(err)
	}
	return client
}

func ValidateFlags(turnHost *string, user *string, peerHost *string) {
	if len(*turnHost) == 0 {
		log.Fatalf("'turnHost' is required")
	}
	if len(*user) == 0 {
		log.Fatalf("'user' is required")
	}
	if len(*peerHost) == 0 {
		log.Fatalf("'peerHost' is required")
	}
}

func HandleRelayConnClose(relayConn net.PacketConn) {
	if closeErr := relayConn.Close(); closeErr != nil {
		panic(closeErr)
	}
}

func ConnectionBindRequest(client *turn.Client, connectionId proto.ConnectionId) {
	err := client.SendConnectionBindRequest(connectionId)
	if err != nil {
		panic(err)
	}
}

func ConnectRequest(client *turn.Client, peerAddress *net.TCPAddr) proto.ConnectionId {
	connectionId, err := client.SendConnectRequest(*peerAddress)
	if err != nil {
		panic(err)
	}
	log.Printf("connectionId=%d", connectionId)
	return connectionId
}

func CreatePermissionRequest(client *turn.Client, peerAddress *net.TCPAddr)  {
	err := client.SendCreatePermissionRequest(peerAddress)
	if err != nil {
		panic(err)
	}
}

func AllocateRequest(client *turn.Client) net.PacketConn {
	relayConn, err := client.Allocate()
	if err != nil {
		panic(err)
	}
	log.Printf("RELAY-ADDRESS=%s", relayConn.LocalAddr().String())
	return relayConn
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
