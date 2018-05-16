package allocation

import (
	"net"
	"sync"
	"time"

	"github.com/pions/pkg/stun"
)

type reservation struct {
	token string
	port  int
}

var reservationsLock sync.RWMutex
var reservations []*reservation

// CreateReservation stores the reservation for the token+port
func CreateReservation(reservationToken string, port int) {
	time.AfterFunc(30*time.Second, func() {
		reservationsLock.Lock()
		defer reservationsLock.Unlock()
		for i := len(reservations) - 1; i >= 0; i-- {
			if reservations[i].token == reservationToken {
				reservations = append(reservations[:i], reservations[i+1:]...)
				return
			}
		}
	})

	reservationsLock.Lock()
	reservations = append(reservations, &reservation{
		token: reservationToken,
		port:  port,
	})
	reservationsLock.Unlock()
}

// GetReservation returns the port for a given reservation if it exists
func GetReservation(reservationToken string) (int, bool) {
	reservationsLock.RLock()
	defer reservationsLock.RUnlock()

	for _, r := range reservations {
		if r.token == reservationToken {
			return r.port, true
		}
	}
	return 0, false
}

// GetRandomEvenPort returns a random unallocated udp4 port
func GetRandomEvenPort() (int, error) {
	listener, err := net.ListenPacket("udp4", "0.0.0.0:0")
	if err != nil {
		return 0, err
	}

	addr, err := stun.NewTransportAddr(listener.LocalAddr())
	if err != nil {
		return 0, err
	} else if err := listener.Close(); err != nil {
		return 0, err
	} else if addr.Port%2 == 1 {
		return GetRandomEvenPort()
	}

	return addr.Port, nil
}
