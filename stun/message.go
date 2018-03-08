package stun

import "fmt"

// MessageClass of 0b00 is a request, a class of 0b01 is an
//   indication, a class of 0b10 is a success response, and a class of
//   0b11 is an error response.
// https://tools.ietf.org/html/rfc5389#section-6
type MessageClass byte

const (
	ClassRequest         MessageClass = 0x00
	ClassIndication      MessageClass = 0x01
	ClassSuccessResponse MessageClass = 0x02
	ClassErrorResponse   MessageClass = 0x03
)

type Method byte

const (
	MethodBinding      Method = 0x01
	MethodSharedSecret Method = 0x02
)

var messageClassName = map[MessageClass]string{
	ClassRequest:         "request",
	ClassIndication:      "indication",
	ClassSuccessResponse: "success response",
	ClassErrorResponse:   "error response",
}

func (m MessageClass) String() string {
	s, err := messageClassName[m]
	if !err {
		// Falling back to hex representation.
		s = fmt.Sprintf("Unk 0x%x", uint16(m))
	}
	return s
}

var methodName = map[Method]string{
	MethodBinding:      "binding",
	MethodSharedSecret: "shared secret",
}

func (m Method) String() string {
	s, err := methodName[m]
	if !err {
		// Falling back to hex representation.
		s = fmt.Sprintf("Unk 0x%x", uint16(m))
	}
	return s
}
