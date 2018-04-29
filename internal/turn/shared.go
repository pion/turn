package turnServer

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"io"
	"math/rand"
	"strconv"
	"time"

	"github.com/pions/pkg/stun"
	"github.com/pkg/errors"
)

const MESSAGE_INTEGRITY_LENGTH = 24

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

func assertMessageIntegrity(m *stun.Message, theirMi *stun.RawAttribute, ourKey [16]byte) error {
	ourMi, err := stun.MessageIntegrityCalculateHMAC(ourKey[:], m.Raw[:len(m.Raw)-MESSAGE_INTEGRITY_LENGTH])
	if err != nil {
		return err
	}

	if bytes.Compare(ourMi, theirMi.Value) != 0 {
		return errors.Errorf("MessageIntegrity mismatch %x %x", ourKey, theirMi.Value)
	} else {
		return nil
	}
}
