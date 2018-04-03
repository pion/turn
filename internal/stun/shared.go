package stunServer

import (
	"crypto/md5"
	"fmt"
	"io"
	"math/rand"
	"net"
	"strconv"
	"time"
)

func netAddrIPPort(addr net.Addr) (net.IP, int, error) {
	host, portStr, err := net.SplitHostPort(addr.String())
	if err != nil {
		return nil, 0, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, 0, err
	}

	return net.ParseIP(host), port, nil
}

// Is there really no stdlib for this?
func min(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}

func randSeq(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// TODO, include time info support stale nonces
func buildNonce() string {
	h := md5.New()
	now := time.Now().Unix()
	_, _ = io.WriteString(h, strconv.FormatInt(now, 10))
	_, _ = io.WriteString(h, strconv.FormatInt(rand.Int63(), 10))
	return fmt.Sprintf("%x", h.Sum(nil))
}
