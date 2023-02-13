package ipnet

import (
	"fmt"
	"net"
	"testing"
)

type mockNetAddrIF interface {
	Network() string
	String() string
}
type mockNetAddrImpl struct {
	IP            net.IP
	Port          int
	Zone          string
	handleString  func() string
	handleNetwork func() string
}

func newMockNetAddr() *mockNetAddrImpl {
	mNetAddrImpl := mockNetAddrImpl{}
	return &mNetAddrImpl
}

func (m *mockNetAddrImpl) String() string {
	return m.handleString()
}

func (m *mockNetAddrImpl) Network() string {
	return m.handleNetwork()
}

func Test_getByteForString1(t *testing.T) {
	tests := []struct {
		supply []byte
		want   byte
	}{
		{supply: []byte("135"), want: 0x87},
		{supply: []byte{0x0, 0x32, 0x33, 0x35}, want: 0xeb}, // 235
	}
	for i, tt := range tests {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			if got := getByteForString(tt.supply); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIPv4UDPPort(t *testing.T) {
	var mUDPAddrIF, mTCPAddrIF net.Addr
	mUDPAddr := net.UDPAddr{
		IP:   net.IPv4(byte(0xfe), byte(0x87), byte(0xd6), byte(0xc2)),
		Port: 3456,
		Zone: "unknown",
	}
	mTCPAddr := net.TCPAddr{
		IP:   net.IPv4(byte(125), byte(154), byte(86), byte(178)),
		Port: 26700,
		Zone: "unknown",
	}
	mTCPAddrIF = &mTCPAddr
	mUDPAddrIF = &mUDPAddr
	tests := []struct {
		supply      net.Addr
		wantAddress net.IP
		wantPort    int
	}{
		{supply: mUDPAddrIF, wantAddress: net.IPv4(byte(0xfe), byte(0x87), byte(0xd6), byte(0xc2)), wantPort: 3456},
		{supply: mTCPAddrIF, wantAddress: net.IPv4(byte(125), byte(154), byte(86), byte(178)), wantPort: 26700},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			gotIP, gotPort, err := AddrIPPortNew(tt.supply)
			if !gotIP.Equal(tt.wantAddress) {
				t.Errorf("got %s, want %s", gotIP, tt.wantAddress)
			}
			if gotPort != tt.wantPort {
				t.Errorf("got %d, want %d", gotPort, tt.wantPort)
			}
			if err != nil {
				t.Errorf("got %s, ", err.Error())
			}
		},
		)
	}
}

func TestIPv6UDPPort(t *testing.T) {
	var mUDPAddrIF net.Addr
	var mUDPAddrIF2 net.Addr
	// use the function from the networking library to create the net.IP variable
	mSetIPAddr := net.ParseIP("FE87:d6c2:1020:3a30:fedd:cd20:5060:7080")
	mUDPAddr := net.UDPAddr{
		IP:   mSetIPAddr,
		Port: 31781,
		Zone: "unknown",
	}
	mUDPAddr2 := net.UDPAddr{
		IP:   mSetIPAddr,
		Port: 25,
		Zone: "unknown",
	}
	mUDPAddrIF = &mUDPAddr
	mUDPAddrIF2 = &mUDPAddr2
	tests := []struct {
		supply      net.Addr
		wantAddress net.IP
		wantPort    int
	}{
		{supply: mUDPAddrIF, wantAddress: net.ParseIP("FE87:d6c2:1020:3a30:fedd:cd20:5060:7080"), wantPort: 31781},
		{supply: mUDPAddrIF2, wantAddress: net.ParseIP("FE87:d6c2:1020:3a30:fedd:cd20:5060:7080"), wantPort: 25},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			gotIP, gotPort, err := AddrIPPortNew(tt.supply)
			if !gotIP.Equal(tt.wantAddress) {
				t.Errorf("got %s, want %s", gotIP, tt.wantAddress)
			}
			if gotPort != tt.wantPort {
				t.Errorf("got %d, want %d", gotPort, tt.wantPort)
			}
			if err != nil {
				t.Errorf("got %s, ", err.Error())
			}
		},
		)
	}
}

func NoBenchmarkAddrIPv4IPPortNew(b *testing.B) {
	var mUDPAddrIF net.Addr
	mUDPAddr := net.UDPAddr{
		IP:   net.IPv4(byte(0xfe), byte(0x87), byte(0xd6), byte(0xc2)),
		Port: 3456,
		Zone: "unknown",
	}
	mUDPAddrIF = &mUDPAddr
	for n := 0; n < b.N; n++ {
		_, _, err := AddrIPPortNew(mUDPAddrIF)
		if err != nil {
			break
		}
	}
}

func BenchmarkIPv6UDPPort(b *testing.B) {
	var mUDPAddrIF net.Addr
	// use the function from the networking library to create the net.IP variable
	mSetIPAddr := net.ParseIP("FE87:d6c2:1020:3a30:fedd:cd20:5060:7080")
	mUDPAddr := net.UDPAddr{
		IP:   mSetIPAddr,
		Port: 31781,
		Zone: "unknown",
	}
	mUDPAddrIF = &mUDPAddr
	for n := 0; n < b.N; n++ {
		_, _, err := AddrIPPortNew(mUDPAddrIF)
		if err != nil {
			break
		}
	}
}

func NoBenchmarkAddrIPPortOld(b *testing.B) {
	var mUDPAddrIF net.Addr
	mUDPAddr := net.UDPAddr{
		IP:   net.IPv4(byte(0xfe), byte(0x87), byte(0xd6), byte(0xc2)),
		Port: 3456,
		Zone: "unknown",
	}
	mUDPAddrIF = &mUDPAddr
	for n := 0; n < b.N; n++ {
		_, _, err := AddrIPPortNew(mUDPAddrIF)
		if err != nil {
			break
		}
	}
}

func TestMock(t *testing.T) {
	var mMockAddrIF mockNetAddrIF
	mmockAddrImpl := newMockNetAddr()
	mMockAddrIF = mmockAddrImpl
	mmockAddrImpl.handleNetwork = func() string {
		return "tcp"
	}
	mmockAddrImpl.handleString = func() string {
		return "192.168.0.1%no particular zone:80"
	}
	tests := []struct {
		supply      net.Addr
		wantAddress net.IP
		wantPort    int
	}{
		{supply: mMockAddrIF, wantAddress: net.IPv4(byte(192), byte(168), byte(0), byte(1)), wantPort: 80},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			gotIP, gotPort, err := AddrIPPortNew(tt.supply)
			if !gotIP.Equal(tt.wantAddress) {
				t.Errorf("got %s, want %s", gotIP, tt.wantAddress)
			}
			if gotPort != tt.wantPort {
				t.Errorf("got %d, want %d", gotPort, tt.wantPort)
			}
			if err != nil {
				t.Errorf("got %s, ", err.Error())
			}
		},
		)
	}
}
