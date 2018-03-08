package stun

import "fmt"

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

var attrNames = map[AttrType]string{
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
	AttrSoftware:          "SOFTWARE",
	AttrAlternateServer:   "ALTERNATE-SERVER",
	AttrFingerprint:       "FINGERPRINT",
}

func (t AttrType) String() string {
	s, ok := attrNames[t]
	if !ok {
		// Just return hex representation of unknown attribute type.
		return fmt.Sprintf("0x%x", uint16(t))
	}
	return s
}
