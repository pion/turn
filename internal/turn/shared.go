package turnServer

import (
	"crypto/md5"
	"fmt"
	"io"
	"math/rand"
	"strconv"
	"time"
)

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
