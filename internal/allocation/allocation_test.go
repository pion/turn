package allocation

import (
	"net"
	"testing"
)

func TestAllocation(t *testing.T) {
	tt := []struct {
		name string
		f    func(*testing.T)
	}{
		{"GetPermission", subTestGetPermission},
		{"AddPermission", subTestAddPermission},
		{"RemovePermission", subTestRemovePermission},
	}

	for _, tc := range tt {
		f := tc.f
		t.Run(tc.name, func(t *testing.T) {
			f(t)
		})
	}
}

func subTestGetPermission(t *testing.T) {
	a := NewAllocation(nil, nil, nil)

	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:3478")
	addr2, _ := net.ResolveUDPAddr("udp", "127.0.0.1:3479")
	addr3, _ := net.ResolveUDPAddr("udp", "127.0.0.1:3480")

	p := &Permission{
		Addr: addr,
	}
	p2 := &Permission{
		Addr: addr2,
	}

	a.AddPermission(p)
	a.AddPermission(p2)

	foundP1 := a.GetPermission(addr.String())
	if foundP1 != p {
		t.Error("Got permission is not same as the the added.")
	}

	foundP2 := a.GetPermission(addr2.String())
	if foundP2 != p2 {
		t.Error("Got permission is not same as the the added.")
	}

	foundP3 := a.GetPermission(addr3.String())
	if foundP3 != nil {
		t.Error("Got permission should be nil if not added.")
	}
}

func subTestAddPermission(t *testing.T) {
	a := NewAllocation(nil, nil, nil)

	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:3478")
	p := &Permission{
		Addr: addr,
	}

	a.AddPermission(p)
	if p.allocation != a {
		t.Error("Permission's allocation should be the adder.")
	}

	foundPermission := a.GetPermission(p.Addr.String())
	if foundPermission != p {
		t.Error("Got permission is not same as the the added.")
	}
}

func subTestRemovePermission(t *testing.T) {
	a := NewAllocation(nil, nil, nil)

	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:3478")
	p := &Permission{
		Addr: addr,
	}

	a.AddPermission(p)

	foundPermission := a.GetPermission(p.Addr.String())
	if foundPermission != p {
		t.Error("Got permission is not same as the the added.")
	}

	a.RemovePermission(p.Addr.String())

	foundPermission = a.GetPermission(p.Addr.String())
	if foundPermission != nil {
		t.Error("Got permission should be nil after removed.")
	}
}
