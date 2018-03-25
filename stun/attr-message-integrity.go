package stun

import (
	"crypto/hmac"
	"crypto/sha1"

	"github.com/pkg/errors"
)

// https://tools.ietf.org/html/rfc5389#section-15.4
// The MESSAGE-INTEGRITY attribute contains an HMAC-SHA1 [RFC2104] of
// the STUN message.  The MESSAGE-INTEGRITY attribute can be present in
// any STUN message type.  Since it uses the SHA1 hash, the HMAC will be
// 20 bytes.  The text used as input to HMAC is the STUN message,
// including the header, up to and including the attribute preceding the
// MESSAGE-INTEGRITY attribute.  With the exception of the FINGERPRINT
// attribute, which appears after MESSAGE-INTEGRITY, agents MUST ignore
// all other attributes that follow MESSAGE-INTEGRITY.

// Look into this
// https://tools.ietf.org/html/rfc7635#appendix-B

const (
	messageIntegrityLength = 20
)

type MessageIntegrity struct {
	MessageIntegrity [messageIntegrityLength]byte
}

func calculateHMAC(key, message []byte) []byte {
	mac := hmac.New(sha1.New, key)
	if _, err := mac.Write(message); err != nil {
		// Can we recover from this failure?
		panic(errors.Wrap(err, "unable to create message integrity HMAC"))
	}
	return mac.Sum(nil)
}

func (m *MessageIntegrity) Pack(message *Message) error {
	prevLen := message.Length
	message.Length += attrHeaderLength + messageIntegrityLength
	message.CommitLength()
	v := calculateHMAC(m.MessageIntegrity[:], message.Raw)
	message.Length = prevLen

	message.AddAttribute(AttrSoftware, v)
	return nil
}

func (m *MessageIntegrity) Unpack(message *Message, rawAttribute *RawAttribute) error {
	return errors.New("MessageIntegirty Unpack() unimplemented")
}
