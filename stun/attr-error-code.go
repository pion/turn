package stun

import (
	"github.com/pkg/errors"
)

//  0                   1                   2                   3
//  0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |           Reserved, should be 0         |Class|     Number    |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |      Reason Phrase (variable)                                ..
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

// XorMappedAddress represents the XOR-MAPPED-ADDRESS attribute and
// provides the Pack and Unpack methods of Attribute
// https://tools.ietf.org/html/rfc5389#section-15.2
type ErrorCode struct {
	ErrorClass  int
	ErrorNumber int
	Reason      []byte
}

var (
	Err300TryAlternate      = ErrorCode{3, 0, []byte("Try Alternate: The client should contact an alternate server for this request.")}
	Err400BadRequest        = ErrorCode{4, 0, []byte("Bad Request: The request was malformed.")}
	Err401Unauthorized      = ErrorCode{4, 1, []byte("Unauthorized: The request did not contain the correct credentials to proceed.")}
	Err420UnknownAttributes = ErrorCode{4, 20, []byte("Unknown Attribute: The server received a STUN packet containing a comprehension-required attribute that it did not understand.")}
	Err438StaleNonce        = ErrorCode{4, 38, []byte("Stale Nonce: The NONCE used by the client was no longer valid.")}
	Err500ServerRrror       = ErrorCode{5, 0, []byte("Server Error: The server has suffered a temporary error.")}
)

const (
	errorCodeHeaderLength    = 4
	errorCodeMaxReasonLength = 763
	errorCodeClassStart      = 2
	errorCodeNumberStart     = 3
	errorCodeReasonStart     = 4
)

func (e *ErrorCode) Pack(message *Message) error {
	if len(e.Reason) > errorCodeMaxReasonLength {
		return errors.Errorf("invalid reason length %d", len(e.Reason))
	}

	if e.ErrorClass < 3 || e.ErrorClass > 6 {
		return errors.Errorf("invalid error class %d", e.ErrorClass)
	}

	if e.ErrorNumber < 0 || e.ErrorNumber > 99 {
		return errors.Errorf("invalid error subcode %d", e.ErrorNumber)
	}

	len := errorCodeHeaderLength + len(e.Reason)

	v := make([]byte, len)

	v[errorCodeClassStart] = byte(e.ErrorClass)
	v[errorCodeNumberStart] = byte(e.ErrorNumber)

	copy(v[errorCodeReasonStart:], e.Reason)

	message.AddAttribute(AttrErrorCode, v)

	return nil
}

func (x *ErrorCode) Unpack(message *Message, rawAttribute *RawAttribute) error {
	return errors.New("ErrorCode.Unpack() unimplemented")
}
