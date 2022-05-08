package client

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPermission(t *testing.T) {
	t.Run("Getter and setter", func(t *testing.T) {
		perm := &permission{}

		assert.Equal(t, permStateIdle, perm.state())
		perm.setState(permStatePermitted)
		assert.Equal(t, permStatePermitted, perm.state())
	})
}

func TestPermissionMap(t *testing.T) {
	t.Run("Basic operations", func(t *testing.T) {
		pm := newPermissionMap()
		assert.NotNil(t, pm)
		assert.NotNil(t, pm.permMap)

		perm1 := &permission{st: permStateIdle}
		perm2 := &permission{st: permStatePermitted}
		udpAddr1, _ := net.ResolveUDPAddr("udp", "1.2.3.4:5000")
		udpAddr2, _ := net.ResolveUDPAddr("udp", "5.6.7.8:8888")
		tcpAddr, _ := net.ResolveTCPAddr("tcp", "1.2.3.4:5000")
		assert.True(t, pm.insert(udpAddr1, perm1))
		assert.Equal(t, 1, len(pm.permMap))
		assert.True(t, pm.insert(udpAddr2, perm2))
		assert.Equal(t, 2, len(pm.permMap))
		assert.False(t, pm.insert(tcpAddr, perm1))
		assert.Equal(t, 2, len(pm.permMap))

		p, ok := pm.find(udpAddr1)
		assert.True(t, ok)
		assert.Equal(t, perm1, p)
		assert.Equal(t, permStateIdle, p.st)

		p, ok = pm.find(udpAddr2)
		assert.True(t, ok)
		assert.Equal(t, perm2, p)
		assert.Equal(t, permStatePermitted, p.st)

		_, ok = pm.find(tcpAddr)
		assert.False(t, ok)

		addrs := pm.addrs()
		ips := []net.IP{}
		for _, addr := range addrs {
			udpAddr, err := net.ResolveUDPAddr(addr.Network(), addr.String())
			assert.NoError(t, err)
			assert.Equal(t, 0, udpAddr.Port)
			ips = append(ips, udpAddr.IP)
		}

		assert.Equal(t, 2, len(ips))
		if ips[0].Equal(udpAddr1.IP) {
			assert.True(t, ips[1].Equal(udpAddr2.IP))
		} else {
			assert.True(t, ips[0].Equal(udpAddr2.IP))
			assert.True(t, ips[1].Equal(udpAddr1.IP))
		}

		pm.delete(tcpAddr)
		assert.Equal(t, 2, len(pm.permMap))
		pm.delete(udpAddr1)
		assert.Equal(t, 1, len(pm.permMap))
		pm.delete(udpAddr2)
		assert.Equal(t, 0, len(pm.permMap))
	})
}
