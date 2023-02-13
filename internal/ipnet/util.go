// Package ipnet contains helper functions around net and IP
package ipnet

import (
	"encoding/hex"
	"errors"
	"net"
	"strconv"
)

var errFailedToCastAddr = errors.New("failed to cast net.Addr to *net.UDPAddr or *net.TCPAddr")

// IPv6OpenSqBracket is opening square bracket used within the string representation
const IPv6OpenSqBracket = 0x5b

// IPv6CloseSqBracket is closing square bracket used within the string representation
const IPv6CloseSqBracket = 0x5d

// IPv6Delimiter is colon - within host indicative of IPv6 - see net/ipsock.go
const IPv6Delimiter = 0x3a

// IPv4Delimiter  is dot - indicative of IPv4
const IPv4Delimiter = 0x2e

// zoneStartDelimiter is percentage - indicates start of zone
const zoneStartDelimiter = 0x25

// portStartDelimiter is colon - indicates end of zone, start of port - so note potential confusion between this and IPv6 delimiter
const portStartDelimiter = 0x3a

// SetUseTestingNetAddr sets the big switch so code can be switched from testable variant to quick non testable variant.
func SetUseTestingNetAddr(mval bool) {
	testingNetAddr = mval
}

// GetUseTestingNetAddr gets the value of the big switch
func GetUseTestingNetAddr() bool {
	return testingNetAddr
}

// TestingNetAddr creates a big switch so code can be switched from testable variant to quick non testable variant.
var testingNetAddr = false

// AddrIPPort function has the original signature and extracts the IP and Port from a net.Addr
// But whilst the original implementation (if TestingNetAddr is false) is quick it has a serious problem.
// The type cast attempt from the interface back into either a *net.UDPAddr or *.net.TCPAddr means that
// Any other struct which implements the net.Addr interface can't be accepted.
// This prevents mocking entirely. So boundary conditions for particular IP address and port combinations can't be unit tested.
// This isn't great.
// But despite this the type cast is extremely quick (despite much performance tuning of the alternative). So a big switch is provided - at the moment.
func AddrIPPort(a net.Addr) (net.IP, int, error) {
	if testingNetAddr {
		return AddrIPPortNew(a)
	}
	aUDP, ok := a.(*net.UDPAddr)
	if ok {
		return aUDP.IP, aUDP.Port, nil
	}

	aTCP, ok := a.(*net.TCPAddr)
	if ok {
		return aTCP.IP, aTCP.Port, nil
	}

	return nil, 0, errFailedToCastAddr
}

// AddrEqual asserts that two net.Addrs are equal
// Currently only supports UDP but will be extended in the future to support others
func AddrEqual(a, b net.Addr) bool {
	aUDP, ok := a.(*net.UDPAddr)
	if !ok {
		return false
	}

	bUDP, ok := b.(*net.UDPAddr)
	if !ok {
		return false
	}

	return aUDP.IP.Equal(bUDP.IP) && aUDP.Port == bUDP.Port
}

// Assume the slice of bytes contains numbers in base 10
func getByteForString(input []byte) byte {
	alen := len(input)
	mval := 0
	base := 1
	for i := alen - 1; i >= 0; i-- {
		if input[i] > 0x0 {
			mval += (int(input[i]) - 48) * base
			base *= 10
		} else {
			break
		}
	}
	return byte(mval)
}

// FindIPv6AddrPort decodes the string representation used in the net library and put the content into hexadecimal byte slice and int.
// The IPv6 address is stored internally as [ffff:ffff:ffff:ffff:ffff:ffff%zone]:port number by the net.UDPAddr and net.TCPAddr structs
func FindIPv6AddrPort(aBytes []byte, lenaBytes int, zoneStart int, portStart int) (mAddr []byte, mPort int64, merr error) {
	var sectionindex int
	var portEnd int
	endIPAddress := false
	endZone := false
	var zIndex int
	mAddr = make([]byte, 16)
	mZone := make([]byte, lenaBytes)
	for inputindex := 0; inputindex < lenaBytes; inputindex++ {
		switch aBytes[inputindex] {
		case IPv6OpenSqBracket:
			continue
		case IPv6CloseSqBracket:
			endZone = true
			continue
		case IPv6Delimiter:
			continue
		case zoneStartDelimiter:
			endIPAddress = true
			continue
		default:
			if !endIPAddress {
				// the +2 is safe as the IP address is never on the end of the string...
				newVal, err := hex.DecodeString(string(aBytes[inputindex : inputindex+2]))
				if err != nil {
					return []byte(""), 0, errFailedToCastAddr
				}
				mAddr[sectionindex] = newVal[0]
				sectionindex++
				inputindex++
			} else {
				// we're dealing with the port number
				if endZone {
					if portStart == 0 {
						portStart = inputindex
					}
					portEnd = inputindex
					continue
				} else {
					// deal with the zone
					mZone[zIndex] = aBytes[inputindex]
					zIndex++
				}
			}
		}
	}
	mPort, err := strconv.ParseInt(string(aBytes[portStart:portEnd+1]), 10, 0)
	if err != nil {
		return []byte(""), 0, errFailedToCastAddr
	}
	// we want an int from this so the binary.BigEndian isn't an option..
	return mAddr, mPort, nil
}

// FindIPv4AddrPort decodes the string representation used in the net library and put the content into hexadecimal byte slice and int.
// The IPv4 address is stored internally as nnn.nnn.nnn.nnn:port number by the net.UDPAddr and net.TCPAddr structs
func FindIPv4AddrPort(aBytes []byte, lenaBytes int, zoneStart int, portStart int) (mAddr []byte, mPort int64, merr error) {
	var start int
	var octetend int
	var portEnd int
	mAddrStart := 12
	octetbytes := make([]byte, 8)
	mAddr = make([]byte, 16)
	mAddr[10] = 0xff
	mAddr[11] = 0xff
	for octet := 0; octet <= 3; octet++ {
		for i := 0; i <= 3; i++ {
			if aBytes[octetend] != IPv4Delimiter && aBytes[octetend] != zoneStartDelimiter {
				octetend++
			} else {
				// zero the buffer - without reducing the capacity.. which nil does..
				for j := 0; j < 8; j++ {
					octetbytes[j] = byte(0)
				}
				k := 0
				for j := 8 - (octetend - start); j < 8; j++ {
					octetbytes[j] = aBytes[start+k]
					k++
				}
				// the function below is much faster the strconv.ParseInt(string([]byte))
				mval := getByteForString(octetbytes)
				octetend++
				start = octetend
				mAddr[mAddrStart+octet] = mval
				break // force a new octet to be started
			}
		}
	}
	for i := portStart; i <= lenaBytes; i++ {
		portEnd = i
	}
	// we want an int from this so the binary.BigEndian isn't an option..
	mPort, err := strconv.ParseInt(string(aBytes[portStart:portEnd]), 10, 0)
	if err != nil {
		return []byte(""), 0, errFailedToCastAddr
	}
	return mAddr, mPort, nil
}

func findAddrStringRegions(aBytes []byte) (isIPv6 bool, isIPv4 bool, zoneStart int, portStart int, lenaBytes int) {
	zoneStart = len(aBytes)
	lenaBytes = zoneStart
	for i := 0; i < lenaBytes; i++ {
		if i <= zoneStart && !isIPv6 && !isIPv4 {
			if aBytes[i] == IPv6Delimiter {
				isIPv6 = true
			}
			if aBytes[i] == IPv4Delimiter {
				isIPv4 = true
			}
		}
		if aBytes[i] == zoneStartDelimiter {
			zoneStart = i
		}
		if isIPv4 {
			if aBytes[i] == portStartDelimiter {
				portStart = i + 1
				break
			}
		}
	}
	return isIPv6, isIPv4, zoneStart, portStart, lenaBytes
}

// AddrIPPortNew is a new version of the original AddrIPPort.
// It inspects the internal string representation of IPv6 and IPv4 addresses to retrieve the IP address and port.
// It could easily export the zone - but this isn't within the original function signature.
func AddrIPPortNew(a net.Addr) (net.IP, int, error) {
	var mIP []byte
	var err error
	var mPort int64
	var IsIPv6 bool
	var IsIPv4 bool
	var zoneStart int
	var lenaBytes int
	var portStart int
	if a.Network() == "udp" || a.Network() == "tcp" {
		aBytes := []byte(a.String())
		IsIPv6, IsIPv4, zoneStart, portStart, lenaBytes = findAddrStringRegions(aBytes)
		switch {
		case IsIPv6:
			mIP, mPort, err = FindIPv6AddrPort(aBytes, lenaBytes, zoneStart, portStart)
		case IsIPv4:
			mIP, mPort, err = FindIPv4AddrPort(aBytes, lenaBytes, zoneStart, portStart)
		default:
			return nil, 0, errFailedToCastAddr
		}
		if err != nil {
			return nil, 0, errFailedToCastAddr
		}
	} else {
		return nil, 0, errFailedToCastAddr
	}
	return mIP, int(mPort), nil
}
