package server

import (
	"crypto/md5" // #nosec
	"net"
	"strings"

	"github.com/pion/stun"
	"github.com/pkg/errors"
)

// context is used to hold the context of a request
type context struct {
	conn    net.PacketConn
	srcAddr net.Addr
	msg     *stun.Message

	realm       string
	authHandler AuthHandler

	messageIntegrity stun.MessageIntegrity
}

// newContext creates a new instance of Context.
func newContext(conn net.PacketConn, srcAddr net.Addr, m *stun.Message, realm string, authHandler AuthHandler) *context {
	return &context{
		conn:        conn,
		srcAddr:     srcAddr,
		msg:         m,
		realm:       realm,
		authHandler: authHandler,
	}
}

// authenticate by AttrMessageIntegrity and return error
func (ctx *context) authenticate() error {
	if !ctx.msg.Contains(stun.AttrMessageIntegrity) {
		nonce, err := buildNonce()
		if err != nil {
			return err
		}

		sendErr := ctx.buildAndSend(
			stun.ClassErrorResponse,
			&stun.ErrorCodeAttribute{Code: stun.CodeUnauthorized},
			stun.NewNonce(nonce),
			stun.NewRealm(ctx.realm))

		if sendErr != nil {
			return sendErr
		}

		return errors.New("Empty message integrity attribute")
	}

	var ourKey [16]byte
	nonceAttr := &stun.Nonce{}
	usernameAttr := &stun.Username{}
	realmAttr := &stun.Realm{}

	if err := realmAttr.GetFrom(ctx.msg); err != nil {
		return ctx.respondWithError(err, stun.CodeBadRequest)
	}

	if err := nonceAttr.GetFrom(ctx.msg); err != nil {
		return ctx.respondWithError(err, stun.CodeBadRequest)
	}

	if err := usernameAttr.GetFrom(ctx.msg); err != nil {
		return ctx.respondWithError(err, stun.CodeBadRequest)
	}

	password, ok := ctx.authHandler(usernameAttr.String(), ctx.srcAddr)
	if !ok {
		return ctx.respondWithError(errors.Errorf("No user exists for %s", usernameAttr.String()), stun.CodeBadRequest)
	}

	/* #nosec */
	ourKey = md5.Sum([]byte(usernameAttr.String() + ":" + realmAttr.String() + ":" + password))
	if err := assertMessageIntegrity(ctx.msg, ourKey[:]); err != nil {
		return ctx.respondWithError(err, stun.CodeBadRequest)
	}

	ctx.messageIntegrity = stun.NewLongTermIntegrity(usernameAttr.String(), realmAttr.String(), password)

	return nil
}

// respond is a helper method to generate a successful response.
func (ctx *context) respond(attrs ...stun.Setter) error {
	return ctx.buildAndSend(stun.ClassSuccessResponse, attrs...)
}

// respondWithError is a helper method to generate an error response.
func (ctx *context) respondWithError(err error, errorCode stun.ErrorCode, attrs ...stun.Setter) error {
	attrs = append(attrs, &stun.ErrorCodeAttribute{Code: errorCode})
	if sendErr := ctx.buildAndSend(stun.ClassErrorResponse,
		attrs...,
	); sendErr != nil {
		err = errors.Errorf(strings.Join([]string{sendErr.Error(), err.Error()}, "\n"))
	}

	return err
}

// buildAndSend is a helper method to generate a stun response.
func (ctx *context) buildAndSend(class stun.MessageClass, attrs ...stun.Setter) error {
	attrs = append([]stun.Setter{&stun.Message{TransactionID: ctx.msg.TransactionID}, stun.NewType(ctx.msg.Type.Method, class)}, attrs...)

	if ctx.messageIntegrity != nil {
		attrs = append(attrs, ctx.messageIntegrity)
	}

	return buildAndSend(ctx.conn, ctx.srcAddr, attrs...)
}
