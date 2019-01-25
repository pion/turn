// +build darwin dragonfly freebsd linux netbsd openbsd solaris

package ipnet

import (
	"github.com/pkg/errors"
	"golang.org/x/net/ipv4"
)

func setControlMessage(conn *ipv4.PacketConn) error {
	if err := conn.SetControlMessage(ipv4.FlagDst, true); err != nil {
		return errors.Wrap(err, "failed to SetControlMessage ipv4.FlagDst")
	}

	return nil
}

func createControlMessage(conn *ipv4.PacketConn, ipcm *ipv4.ControlMessage) *ControlMessage {
	return &ControlMessage{
		Dst: ipcm.Dst,
	}
}
