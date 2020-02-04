package main

import (
	"flag"
	"fmt"
	"github.com/pion/stun"
	"github.com/pion/turn/v2/internal/proto"
	"log"
	"math"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/pion/logging"
	"github.com/pion/turn/v2"
)

var (
	turnHost = flag.String("turnHost", "", "TURN Server name.")
	turnPort = flag.Int("turnPort", 3478, "Listening turnPort.")
	user     = flag.String("user", "", "A pair of username and password (e.g. \"user=pass\")")
	realm    = flag.String("realm", "", "Realm")
	peerHost = flag.String("peerHost", "", "Peer Host")
	peerPort = flag.Int("peerPort", 8080, "Peer Port.")
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

	var wg sync.WaitGroup
	wg.Add(1)

	go Read(dataConnection, &wg)
	go Write(dataConnection, &wg)

	go CustomListen(controlClient, dataClient, &wg)
	defer controlClient.Close()

	relayConn := AllocateRequest(controlClient)
	defer HandleRelayConnClose(relayConn)

	CreatePermissionRequest(controlClient, *peerHost, *peerPort)

	select {}
}

func CustomListen(controlClient, dataClient *turn.Client, wg *sync.WaitGroup) {
	buf := make([]byte, math.MaxUint32)
	for {
		n, from, err := controlClient.ReadFrom(buf)
		if err != nil {
			fmt.Print("exiting read loop: %s", err.Error())
			break
		}

		data := buf[:n]
		if stun.IsMessage(data) {
			raw := make([]byte, len(data))
			copy(raw, data)

			msg := &stun.Message{Raw: raw}
			if err := msg.Decode(); err != nil {
				log.Fatalf("failed to decode STUN message: %s", err.Error())
			}

			if msg.Type.Method == stun.MethodConnectionAttempt && msg.Type.Class == stun.ClassIndication {
				var connectionId proto.ConnectionId
				if err := connectionId.GetFrom(msg); err != nil {
					log.Fatal(err)
				}
				ConnectionBindRequest(dataClient, connectionId)
				fmt.Println("Connection received")
				wg.Done()
				continue
			}
		}

		_, err = controlClient.HandleInbound(buf[:n], from)
		if err != nil {
			fmt.Print("exiting read loop: %s", err.Error())
			break
		}
	}
}

func Read(conn net.Conn, wg *sync.WaitGroup) {
	wg.Wait()
	buf := make([]byte, 1024)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			continue
		}

		if stun.IsMessage(buf[:n]) {
			continue
		}

		if len(buf[:n]) > 0 {
			log.Printf("Message from peer %s\n",  buf[:n])
			if err != nil {
				log.Fatal("Could not write to network device connection", err)
			}
		}
	}
}


func Write(client *net.TCPConn,  wg *sync.WaitGroup) {
	wg.Wait()
	for {
		time.Sleep(5 * time.Second)

		bytesWritten, err := client.Write([]byte("Hello from Client"))
		if err != nil {
			log.Fatal("Could not write to connection.")
		}

		if bytesWritten == 0 {
			log.Fatal("No bytes written.")
		}
	}
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


func CreatePermissionRequest(client *turn.Client, peerHost string, peerPort int) {
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
