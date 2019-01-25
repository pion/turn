package ipnet

import (
	"net"

	"golang.org/x/net/ipv4"
)

func setControlMessage(conn *ipv4.PacketConn) error {
	// ipv4.FlagDst is not supported on Windows
	return nil
}

func createControlMessage(conn *ipv4.PacketConn, ipcm *ipv4.ControlMessage) *ControlMessage {
	// Fall back on the LocalAddr of the connection
	return &ControlMessage{
		Dst: conn.LocalAddr().(*net.UDPAddr).IP,
	}
}
