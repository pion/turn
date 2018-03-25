package stun

import (
	"encoding/binary"
	"hash/crc32"

	"github.com/pkg/errors"
)

// https://tools.ietf.org/html/rfc5389#section-15.10
// The SOFTWARE attribute contains a textual description of the software
//  being used by the agent sending the message
type Fingerprint struct {
	Fingerprint uint32
}

const (
	fingerprintXOR    uint32 = 0x5354554e
	fingerprintLength        = 4
)

func calculateFingerprint(b []byte) uint32 {
	return crc32.ChecksumIEEE(b) ^ fingerprintXOR
}

func (s *Fingerprint) Pack(message *Message) error {
	prevLen := message.Length
	message.Length += attrHeaderLength + fingerprintLength
	message.CommitLength()
	v := make([]byte, fingerprintLength)
	binary.BigEndian.PutUint32(v, calculateFingerprint(message.Raw))
	message.Length = prevLen

	message.AddAttribute(AttrFingerprint, v)
	return nil
}

func (s *Fingerprint) Unpack(message *Message, rawAttribute *RawAttribute) error {

	s.Fingerprint = binary.BigEndian.Uint32(rawAttribute.Value)

	expected := calculateFingerprint(message.Raw[:rawAttribute.Offset])

	if expected != s.Fingerprint {
		return errors.Errorf("fingerprint mismatch %v != %v (expected)", s.Fingerprint, expected)
	}
	return nil
}
