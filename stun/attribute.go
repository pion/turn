package stun

import (
	"fmt"
)

// AttrType represents an attribute type
// https://tools.ietf.org/html/rfc5389#section-15
type AttrType uint16

// STUN Attribute Registry

// Comprehension-required range (0x0000-0x7FFF)
const (
	AttrMappedAddress      AttrType = 0x0001 // STUN
	AttrResponseAddress    AttrType = 0x0002 // STUN Invalid
	AttrChangeAddress      AttrType = 0x0003 // STUN Invalid
	AttrSourceAddress      AttrType = 0x0004 // STUN Invalid
	AttrChangedAddress     AttrType = 0x0005 // STUN Invalid
	AttrUsername           AttrType = 0x0006 // STUN
	AttrPassword           AttrType = 0x0007 // STUN Invalid
	AttrMessageIntegrity   AttrType = 0x0008 // STUN
	AttrErrorCode          AttrType = 0x0009 // STUN
	AttrUnknownAttributes  AttrType = 0x000A // STUN
	AttrReflectedFrom      AttrType = 0x000B // STUN Invalid
	AttrChannelNumber      AttrType = 0x000C // TURN
	AttrLifetime           AttrType = 0x000D // TURN
	AttrBandwidth          AttrType = 0x0010 // TURN Invalid
	AttrXORPeerAddress     AttrType = 0x0012 // TURN
	AttrData               AttrType = 0x0013 // TURN
	AttrXORRelayedAddress  AttrType = 0x0016 // TURN
	AttrEvenPort           AttrType = 0x0018 // TURN
	AttrRequestedTransport AttrType = 0x0019 // TURN
	AttrDontFragment       AttrType = 0x001A // TURN
	AttrTimerVal           AttrType = 0x0021 // TURN
	AttrReservationToken   AttrType = 0x0022 // TURN
	AttrRealm              AttrType = 0x0014 // STUN
	AttrNonce              AttrType = 0x0015 // STUN
	AttrXORMappedAddress   AttrType = 0x0020 // STUN
)

// Comprehension-optional range (0x8000-0xFFFF):
const (
	AttrSoftware        AttrType = 0x8022 // STUN
	AttrAlternateServer AttrType = 0x8023 // STUN
	AttrFingerprint     AttrType = 0x8028 // STUN
)

// https://tools.ietf.org/html/rfc5245#section-19.1
// ICE STUN Attributes
const (
	AttrPriority       AttrType = 0x0024 // STUN ICE
	AttrUseCandidate   AttrType = 0x0025 // STUN ICE
	AttrIceControlled  AttrType = 0x8029 // STUN ICE
	AttrIceControlling AttrType = 0x802A // STUN ICE
)

var attrNames = map[AttrType]string{
	AttrMappedAddress:      "MAPPED-ADDRESS",
	AttrResponseAddress:    "RESPONSE-ADDRESS",
	AttrChangeAddress:      "CHANGE-ADDRESS",
	AttrSourceAddress:      "SOURCE-ADDRESS",
	AttrChangedAddress:     "CHANGED-ADDRESS",
	AttrUsername:           "USERNAME",
	AttrPassword:           "PASSWORD",
	AttrErrorCode:          "ERROR-CODE",
	AttrMessageIntegrity:   "MESSAGE-INTEGRITY",
	AttrUnknownAttributes:  "UNKNOWN-ATTRIBUTES",
	AttrReflectedFrom:      "REFLECTED-FROM",
	AttrChannelNumber:      "CHANNEL-NUMBER",
	AttrLifetime:           "LIFETIME",
	AttrBandwidth:          "BANDWIDTH",
	AttrXORPeerAddress:     "XOR-PEER-ADDRESS",
	AttrData:               "DATA",
	AttrXORRelayedAddress:  "XOR-RELAYED-ADDRESS",
	AttrEvenPort:           "EVEN-PORT",
	AttrRequestedTransport: "REQUESTED-TRANSPORT",
	AttrDontFragment:       "DONT-FRAGMENT",
	AttrTimerVal:           "TIMER-VAL",
	AttrReservationToken:   "RESERVATION-TOKEN",
	AttrRealm:              "REALM",
	AttrNonce:              "NONCE",
	AttrXORMappedAddress:   "XOR-MAPPED-ADDRESS",
	AttrSoftware:           "SOFTWARE",
	AttrAlternateServer:    "ALTERNATE-SERVER",
	AttrFingerprint:        "FINGERPRINT",
	AttrPriority:           "PRIORITY",
	AttrUseCandidate:       "USE-CANDIDATE",
	AttrIceControlled:      "ICE-CONTROLLED",
	AttrIceControlling:     "ICE-CONTROLLING",
}

const (
	attrHeaderLength   = 4
	attrLengthStart    = 2
	attrLengthLength   = 2
	attrValueStart     = 4
	attrLengthMultiple = 4
)

func GetAttrPadding(len int) int {
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
	Offset int
}

type Attribute interface {
	Pack(message *Message) error
	Unpack(message *Message, rawAttribute *RawAttribute) error
}
