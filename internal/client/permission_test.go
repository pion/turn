// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package client

import (
	"net"
	"sort"
	"strings"
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
		perm3 := &permission{st: permStateIdle}
		udpAddr1, _ := net.ResolveUDPAddr("udp", "1.2.3.4:5000")
		udpAddr2, _ := net.ResolveUDPAddr("udp", "5.6.7.8:8888")
		tcpAddr, _ := net.ResolveTCPAddr("tcp", "7.8.9.10:5000")
		assert.True(t, pm.insert(udpAddr1, perm1))
		assert.Equal(t, 1, len(pm.permMap))
		assert.True(t, pm.insert(udpAddr2, perm2))
		assert.Equal(t, 2, len(pm.permMap))
		assert.True(t, pm.insert(tcpAddr, perm3))
		assert.Equal(t, 3, len(pm.permMap))

		p, ok := pm.find(udpAddr1)
		assert.True(t, ok)
		assert.Equal(t, perm1, p)
		assert.Equal(t, permStateIdle, p.st)

		p, ok = pm.find(udpAddr2)
		assert.True(t, ok)
		assert.Equal(t, perm2, p)
		assert.Equal(t, permStatePermitted, p.st)

		p, ok = pm.find(tcpAddr)
		assert.True(t, ok)
		assert.Equal(t, perm3, p)
		assert.Equal(t, permStateIdle, p.st)

		addrs := pm.addrs()
		ips := []net.IP{}

		for _, addr := range addrs {
			switch addr.(type) {
			case *net.UDPAddr:
				addr, err := net.ResolveUDPAddr(addr.Network(), addr.String())
				assert.NoError(t, err)

				ips = append(ips, addr.IP)
			case *net.TCPAddr:
				addr, err := net.ResolveTCPAddr(addr.Network(), addr.String())
				assert.NoError(t, err)

				ips = append(ips, addr.IP)
			}
		}

		assert.Equal(t, 3, len(ips))
		sort.Slice(ips, func(i, j int) bool {
			return strings.Compare(ips[i].String(), ips[j].String()) < 0
		})

		assert.True(t, ips[0].Equal(udpAddr1.IP))
		assert.True(t, ips[1].Equal(udpAddr2.IP))
		assert.True(t, ips[2].Equal(tcpAddr.IP))

		pm.delete(tcpAddr)
		assert.Equal(t, 2, len(pm.permMap))
		pm.delete(udpAddr1)
		assert.Equal(t, 1, len(pm.permMap))
		pm.delete(udpAddr2)
		assert.Equal(t, 0, len(pm.permMap))
	})
}
