package server

import (
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/gortc/turn"
	"github.com/pion/turn/internal/allocation"
	"github.com/pion/turn/internal/ipnet"
)

func TestServer(t *testing.T) {
	s := NewServer("realm", turn.DefaultLifetime, nil)
	l, _ := net.ListenPacket("udp4", "0.0.0.0:0")
	conn, _ := ipnet.NewPacketConn("udp4", l)
	s.connection = conn

	tt := []struct {
		name string
		f    func(*testing.T, *Server)
	}{
		{"AllocationTimeout", subTestAllocationTimeout},
		{"ServerClose", subTestServerClose},
	}

	for _, tc := range tt {
		f := tc.f
		t.Run(tc.name, func(t *testing.T) {
			f(t, s)
		})
	}
}

func subTestAllocationTimeout(t *testing.T, s *Server) {
	lifetime := time.Second

	dstAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:3500")
	allocations := make([]*allocation.Allocation, 10)

	for index := range allocations {
		srcAddr, _ := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.0:%d", 3501+index))

		fiveTuple := &allocation.FiveTuple{
			Protocol: allocation.UDP,
			SrcAddr:  srcAddr,
			DstAddr:  dstAddr,
		}

		alloc, err := s.manager.CreateAllocation(fiveTuple, s.connection, 0, lifetime)

		if err != nil {
			t.Errorf("Failed to create allocation with %v", fiveTuple)
		}

		allocations[index] = alloc
	}

	// make all allocations timeout
	time.Sleep(lifetime + time.Second)

	for _, alloc := range allocations {
		if !isClosed(alloc.RelaySocket) {
			t.Error("Allocation relay socket should be closed if timeout")
		}
	}
}

func subTestServerClose(t *testing.T, s *Server) {
	dstAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:3500")
	srcAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:3510")

	alloc, _ := s.manager.CreateAllocation(&allocation.FiveTuple{
		Protocol: allocation.UDP,
		SrcAddr:  srcAddr,
		DstAddr:  dstAddr,
	}, s.connection, 0, turn.DefaultLifetime)

	_ = s.Close()

	if !isClosed(alloc.RelaySocket) {
		t.Error("Allocation relay socket should be closed if server closed.")
	}
}

// Hack way to check conn's close status
func isClosed(conn io.Closer) bool {
	closeErr := conn.Close()
	return closeErr != nil && strings.Contains(closeErr.Error(), "use of closed network connection")
}
