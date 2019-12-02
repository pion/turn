package server

import (
	// #nosec

	"crypto/md5"
	"fmt"
	"io"
	"math/rand"
	"net"
	"strconv"
	"time"

	"github.com/pion/stun"
	"github.com/pion/turn/internal/proto"
)

const (
	maximumAllocationLifetime = time.Hour // https://tools.ietf.org/html/rfc5766#section-6.2 defines 3600 seconds recommendation
)

func randSeq(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// TODO, include time info support stale nonces
func buildNonce() (string, error) {
	/* #nosec */
	h := md5.New()
	if _, err := io.WriteString(h, strconv.FormatInt(time.Now().Unix(), 10)); err != nil {
		return "", fmt.Errorf("failed generate nonce: %v", err)
	}
	if _, err := io.WriteString(h, strconv.FormatInt(rand.Int63(), 10)); err != nil {
		return "", fmt.Errorf("failed generate nonce: %v", err)
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func buildAndSend(conn net.PacketConn, dst net.Addr, attrs ...stun.Setter) error {
	msg, err := stun.Build(attrs...)
	if err != nil {
		return err
	}
	_, err = conn.WriteTo(msg.Raw, dst)
	return err
}

// Send a STUN packet and return the original error to the caller
func buildAndSendErr(conn net.PacketConn, dst net.Addr, err error, attrs ...stun.Setter) error {
	if sendErr := buildAndSend(conn, dst, attrs...); sendErr != nil {
		err = fmt.Errorf("Failed to send error message %w %w", sendErr, err)
	}
	return err
}

func buildMsg(transactionID [stun.TransactionIDSize]byte, msgType stun.MessageType, additional ...stun.Setter) []stun.Setter {
	return append([]stun.Setter{&stun.Message{TransactionID: transactionID}, msgType}, additional...)
}

func authenticateRequest(r Request, m *stun.Message, callingMethod stun.Method) (stun.MessageIntegrity, []stun.Setter, error) {
	if !m.Contains(stun.AttrMessageIntegrity) {
		nonce, err := buildNonce()
		if err != nil {
			return stun.MessageIntegrity{}, nil, err
		}

		return nil, buildMsg(m.TransactionID,
			stun.NewType(callingMethod, stun.ClassErrorResponse),
			&stun.ErrorCodeAttribute{Code: stun.CodeUnauthorized},
			stun.NewNonce(nonce),
			stun.NewRealm(r.Realm),
		), nil

	}

	var ourKey [16]byte
	nonceAttr := &stun.Nonce{}
	usernameAttr := &stun.Username{}
	realmAttr := &stun.Realm{}
	badRequestMsg := buildMsg(m.TransactionID, stun.NewType(callingMethod, stun.ClassErrorResponse), &stun.ErrorCodeAttribute{Code: stun.CodeBadRequest})

	if err := realmAttr.GetFrom(m); err != nil {
		return nil, badRequestMsg, err
	}

	if err := nonceAttr.GetFrom(m); err != nil {
		return nil, badRequestMsg, err
	}

	if err := usernameAttr.GetFrom(m); err != nil {
		return nil, badRequestMsg, err
	}

	password, ok := r.AuthHandler(usernameAttr.String(), r.SrcAddr)
	if !ok {
		return nil, badRequestMsg, fmt.Errorf("No user exists for %s", usernameAttr.String())
	}

	/* #nosec */
	ourKey = md5.Sum([]byte(usernameAttr.String() + ":" + realmAttr.String() + ":" + password))
	if err := stun.MessageIntegrity(ourKey[:]).Check(m); err != nil {
		return nil, badRequestMsg, err
	}

	return stun.NewLongTermIntegrity(usernameAttr.String(), realmAttr.String(), password), nil, nil
}

func allocationLifeTime(m *stun.Message) time.Duration {
	lifetimeDuration := proto.DefaultLifetime

	var lifetime proto.Lifetime
	if err := lifetime.GetFrom(m); err == nil {
		if lifetime.Duration < maximumAllocationLifetime {
			lifetimeDuration = lifetime.Duration
		}
	}

	return lifetimeDuration
}
