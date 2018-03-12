package stun

import (
	"encoding/binary"
	"fmt"
)

// AttrType represents an attribute type
// https://tools.ietf.org/html/rfc5389#section-15
type AttrType uint16

// STUN Attribute Registry

// Comprehension-required range (0x0000-0x7FFF)
const (
	AttrMappedAddress     AttrType = 0x0001
	AttrResponseAddress   AttrType = 0x0002 // Invalid
	AttrChangeAddress     AttrType = 0x0003 // Invalid
	AttrSourceAddress     AttrType = 0x0004 // Invalid
	AttrChangedAddress    AttrType = 0x0005 // Invalid
	AttrUsername          AttrType = 0x0006
	AttrPassword          AttrType = 0x0007 // Invalid
	AttrMessageIntegrity  AttrType = 0x0008
	AttrErrorCode         AttrType = 0x0009
	AttrUnknownAttributes AttrType = 0x000A
	AttrReflectedFrom     AttrType = 0x000B // Invalid
	AttrRealm             AttrType = 0x0014
	AttrNonce             AttrType = 0x0015
	AttrXORMappedAddress  AttrType = 0x0020
)

// Comprehension-optional range (0x8000-0xFFFF):
const (
	AttrSoftware        AttrType = 0x8022
	AttrAlternateServer AttrType = 0x8023
	AttrFingerprint     AttrType = 0x8028
)

// https://tools.ietf.org/html/rfc5245#section-19.1
// ICE STUN Attributes
const (
	AttrPriority       AttrType = 0x0024
	AttrUseCandidate   AttrType = 0x0025
	AttrIceControlled  AttrType = 0x8029
	AttrIceControlling AttrType = 0x802A
)

var attrNames = map[AttrType]string{
	// - Comprehension-Required
	AttrMappedAddress:     "MAPPED-ADDRESS",
	AttrResponseAddress:   "RESPONSE-ADDRESS",
	AttrChangeAddress:     "CHANGE-ADDRESS",
	AttrSourceAddress:     "SOURCE-ADDRESS",
	AttrChangedAddress:    "CHANGED-ADDRESS",
	AttrUsername:          "USERNAME",
	AttrPassword:          "PASSWORD",
	AttrErrorCode:         "ERROR-CODE",
	AttrMessageIntegrity:  "MESSAGE-INTEGRITY",
	AttrUnknownAttributes: "UNKNOWN-ATTRIBUTES",
	AttrReflectedFrom:     "REFLECTED-FROM",
	AttrRealm:             "REALM",
	AttrNonce:             "NONCE",
	AttrXORMappedAddress:  "XOR-MAPPED-ADDRESS",
	// - Comprehension-Optional
	AttrSoftware:        "SOFTWARE",
	AttrAlternateServer: "ALTERNATE-SERVER",
	AttrFingerprint:     "FINGERPRINT",
	// - ICE STUN
	AttrPriority:       "PRIORITY",
	AttrUseCandidate:   "USE-CANDIDATE",
	AttrIceControlled:  "ICE-CONTROLLED",
	AttrIceControlling: "ICE-CONTROLLING",
}

const (
	attrLengthStart    = 2
	attrLengthLength   = 2
	attrValueStart     = 4
	attrLengthMultiple = 4
)

func getPadding(len int) int {
	return (attrLengthMultiple - (len % attrLengthMultiple)) % attrLengthMultiple
}

func (t AttrType) String() string {
	s, ok := attrNames[t]
	if !ok {
		// Just return hex representation of unknown attribute type.
		return fmt.Sprintf("0x%x", uint16(t))
	}
	return s
}

// RawAttribute represents an unprocessed view of the TLV structure
// for an attribute
// https://tools.ietf.org/html/rfc5389#section-15
type RawAttribute struct {
	Type   AttrType
	Length uint16
	Value  []byte
	Pad    uint16
}

func (r *RawAttribute) Pack(attribute []byte) int {
	binary.BigEndian.PutUint16(attribute, uint16(r.Type))
	binary.BigEndian.PutUint16(attribute[attrLengthStart:attrLengthStart+attrLengthLength], uint16(r.Length))
	copy(attribute[attrValueStart:], r.Value)
	return 4 + len(r.Value)
}

func (r *RawAttribute) Unpack(attribute []byte) *RawAttribute {
	typ := AttrType(binary.BigEndian.Uint16(attribute))
	len := binary.BigEndian.Uint16(attribute[attrLengthStart : attrLengthStart+attrLengthLength])
	pad := (attrLengthMultiple - (len % attrLengthMultiple)) % attrLengthMultiple
	return &RawAttribute{typ, len, attribute[attrValueStart : attrValueStart+len], pad}
}

type Attribute interface {
	Pack(message *Message) (*RawAttribute, error)
	Unpack(message *Message, rawAttribute *RawAttribute) error
}
