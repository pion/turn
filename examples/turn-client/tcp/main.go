package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"github.com/pion/stun"
	"github.com/pion/turn/v2/internal/proto"
	"log"
	"math"
	"net"
	"strings"
	"time"

	"github.com/pion/logging"
	"github.com/pion/turn/v2"
)

var (
	turnHost = 	flag.String("turnHost", "", "TURN Server name.")
	turnPort = 	flag.Int("turnPort", 3478, "Listening turnPort.")
	user = 		flag.String("user", "", "A pair of username and password (e.g. \"user=pass\")")
	realm = 	flag.String("realm", "", "Realm")
	peerHost = 	flag.String("peerHost", "", "Peer Host")
	peerPort = 	flag.Int("peerPort", 8080, "Peer Port.")
)

func main() {
	flag.Parse()
	ValidateFlags(turnHost, user, peerHost)

	turnServerAddr, err := net.ResolveTCPAddr("tcp4", fmt.Sprintf("%s:%d", *turnHost, *turnPort))
	if err != nil {
		log.Fatal(err)
	}

	controlLocalTransportAddr, err := net.ResolveTCPAddr("tcp4", ":8081")
	if err != nil {
		log.Fatal(err)
	}
	controlConnection, err := net.DialTCP("tcp", controlLocalTransportAddr, turnServerAddr)
	if err != nil {
		log.Fatal(err)
	}
	controlClient := CreateClient(
		controlConnection,
		turnServerAddr,
		user,
		realm,
	)
	err = controlClient.Listen()
	if err != nil {
		log.Fatal(err)
	}
	defer controlClient.Close()

	relayConn := AllocateRequest(controlClient)
	defer HandleRelayConnClose(relayConn)

	CreatePermissionRequest(controlClient, *peerHost, *peerPort)
	connectionId := WaitForConnection(relayConn)

	dataLocalTransportAddr, err := net.ResolveTCPAddr("tcp4", ":8888")
	if err != nil {
		log.Fatal(err)
	}
	dataConnection, err := net.DialTCP("tcp", dataLocalTransportAddr, turnServerAddr)
	if err != nil {
		log.Fatal(err)
	}
	dataClient := CreateClient(
		dataConnection,
		turnServerAddr,
		user,
		realm,
	)

	ConnectionBindRequest(dataClient, proto.ConnectionId(connectionId))

	go ReadFromDataConnection(dataConnection)
	go WriteToDataConnection(dataClient, dataLocalTransportAddr)
}

func ReadFromDataConnection(conn *net.TCPConn) {
	buf := make([]byte, math.MaxUint16)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			continue
		}

		if stun.IsMessage(buf[:n]) {
			continue;
		}

		if len(buf[:n]) > 0 {
			log.Printf("%s\n", string(buf[:n]))
		}
	}
}

func WriteToDataConnection(client *turn.Client, localDataAddr *net.TCPAddr) {
	for {
		time.Sleep(5 * time.Second)

		bytesWritten, err := client.WriteTo([]byte("Hello from Client"), localDataAddr)
		if err != nil {
			log.Fatal("Could not write to connection.")
		}

		if bytesWritten == 0 {
			log.Fatal("No bytes written.")
		}
	}
}

func WaitForConnection(relayConn net.PacketConn) uint32 {
	buf := make([]byte, 1500)
	n, _, readerErr := relayConn.ReadFrom(buf)
	if readerErr != nil {
		panic(readerErr)
	}

	log.Println("Connection Received ConnectionID:", binary.BigEndian.Uint32(buf[:n]))

	return binary.BigEndian.Uint32(buf[:n])
}

func CreateClient(conn *net.TCPConn, turnServerAddr *net.TCPAddr, user, realm *string) *turn.Client {
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
		log.Fatal(err)
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
		log.Fatal(err)
	}
}

func ConnectRequest(client *turn.Client, peerAddress *net.TCPAddr) proto.ConnectionId {
	connectionId, err := client.SendConnectRequest(*peerAddress)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("connectionId=%d", connectionId)
	return connectionId
}

func CreatePermissionRequest(client *turn.Client, peerHost string, peerPort int)  {
	peerAddress, err := net.ResolveTCPAddr("tcp4", fmt.Sprintf("%s:%d", peerHost, peerPort))
	if err != nil {
		log.Fatal(err)
	}

	err = client.SendCreatePermissionRequest(peerAddress)
	if err != nil {
		log.Fatal(err)
	}
}

func AllocateRequest(client *turn.Client) net.PacketConn {
	relayConn, err := client.Allocate()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("RELAY-ADDRESS=%s", relayConn.LocalAddr().String())
	return relayConn
}