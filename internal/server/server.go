// Package server implements the private API to implement a TURN server
package server

import (
	"fmt"
	"net"

	"github.com/pion/stun"
	"github.com/pion/turn/v2/internal/proto"
)

// Request contains all the state needed to process a single incoming datagram
type Request struct {
	State
	// Current Request State
	SrcAddr net.Addr
	Buff    []byte
}

// HandleRequest processes the give Request
func HandleRequest(r Request) error {
	r.Log.Debugf("received %d bytes of udp from %s on %s", len(r.Buff), r.SrcAddr.String(), r.Conn.LocalAddr().String())

	if proto.IsChannelData(r.Buff) {
		return handleDataPacket(r)
	}

	return handleTURNPacket(r)
}

func handleDataPacket(r Request) error {
	r.Log.Debugf("received DataPacket from %s", r.SrcAddr.String())
	c := proto.ChannelData{Raw: r.Buff}
	if err := c.Decode(); err != nil {
		return fmt.Errorf("%w: %v", errFailedToCreateChannelData, err)
	}

	err := handleChannelData(r, &c)
	if err != nil {
		err = fmt.Errorf("%w from %v: %v", errUnableToHandleChannelData, r.SrcAddr, err)
	}

	return err
}

func handleTURNPacket(r Request) error {
	r.Log.Debug("handleTURNPacket")
	m := &stun.Message{Raw: append([]byte{}, r.Buff...)}
	if err := m.Decode(); err != nil {
		return fmt.Errorf("%w: %v", errFailedToCreateSTUNPacket, err)
	}

	h, err := getMessageHandler(m.Type.Class, m.Type.Method)
	if err != nil {
		return fmt.Errorf("%w %v-%v from %v: %v", errUnhandledSTUNPacket, m.Type.Method, m.Type.Class, r.SrcAddr, err)
	}

	err = h(r, m)
	if err != nil {
		return fmt.Errorf("%w %v-%v from %v: %v", errFailedToHandle, m.Type.Method, m.Type.Class, r.SrcAddr, err)
	}

	return nil
}

func getMessageHandler(class stun.MessageClass, method stun.Method) (func(r Request, m *stun.Message) error, error) {
	switch class {
	case stun.ClassIndication:
		switch method {
		case stun.MethodSend:
			return handleSendIndication, nil
		default:
			return nil, fmt.Errorf("%w: %s", errUnexpectedMethod, method)
		}

	case stun.ClassRequest:
		switch method {
		case stun.MethodAllocate:
			return handleAllocateRequest, nil
		case stun.MethodRefresh:
			return handleRefreshRequest, nil
		case stun.MethodCreatePermission:
			return handleCreatePermissionRequest, nil
		case stun.MethodChannelBind:
			return handleChannelBindRequest, nil
		case stun.MethodBinding:
			return handleBindingRequest, nil
		default:
			return nil, fmt.Errorf("%w: %s", errUnexpectedMethod, method)
		}

	default:
		return nil, fmt.Errorf("%w: %s", errUnexpectedClass, class)
	}
}
