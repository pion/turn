package stun

import (
	"encoding/binary"
	"fmt"

	"github.com/pkg/errors"
)

// MessageClass of 0b00 is a request, a class of 0b01 is an
//   indication, a class of 0b10 is a success response, and a class of
//   0b11 is an error response.
// https://tools.ietf.org/html/rfc5389#section-6
type MessageClass byte

const (
	// ClassRequest describes a request method type
	ClassRequest MessageClass = 0x00
	// ClassIndication describes an indication method type
	ClassIndication MessageClass = 0x01
	// ClassSuccessResponse describes an success response method type
	ClassSuccessResponse MessageClass = 0x02
	// ClassErrorResponse describes an error response method type
	ClassErrorResponse MessageClass = 0x03
)

type Method uint16

const (
	MethodBinding      Method = 0x01
	MethodSharedSecret Method = 0x02
)

var messageClassName = map[MessageClass]string{
	ClassRequest:         "REQUEST",
	ClassIndication:      "INDICATION",
	ClassSuccessResponse: "SUCCESS-RESPONSE",
	ClassErrorResponse:   "ERROR-RESPONSE",
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
	MethodBinding:      "BINDING",
	MethodSharedSecret: "SHARED-SECRET",
}

func (m Method) String() string {
	s, err := methodName[m]
	if !err {
		s = fmt.Sprintf("Unk 0x%x", uint16(m))
	}
	return s
}

//       0                   1                   2                   3
//       0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
//      +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
//      |0 0|     STUN Message Type     |         Message Length        |
//      +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
//      |                         Magic Cookie                          |
//      +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
//      |                                                               |
//      |                     Transaction ID (96 bits)                  |
//      |                                                               |
//      +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
//

const (
	headerStart         int = 0
	headerLength        int = 20
	messageLengthStart  int = 2
	messageLengthLength int = 2
	magicCookieStart    int = 4
	magicCookieLength   int = 4
	transactionIDStart  int = 4
	transactionIDLength int = 16
)

type Message struct {
	Class         MessageClass
	Method        Method
	Length        uint16
	TransactionID []byte
	Attributes    []*RawAttribute
}

// The most significant 2 bits of every STUN message MUST be zeroes.
// This can be used to differentiate STUN packets from other protocols
// when STUN is multiplexed with other protocols on the same port.
// https://tools.ietf.org/html/rfc5389#section-6
func verifyHeaderMostSignificant2Bits(header []byte) bool {
	const (
		mostSig2BitsMask   uint = 0x3 // 0b11
		mostSig2BitsShiftR uint = 6   // R 0b11000000 -> 0b00000011
	)

	return ((uint(header[headerStart]) & mostSig2BitsMask) >> mostSig2BitsShiftR) == 0
}

func verifyMagicCookie(header []byte) error {
	const magicCookie = 0x2112A442
	c := header[magicCookieStart : magicCookieStart+magicCookieLength]
	if binary.BigEndian.Uint32(c) != magicCookie {
		return errors.Errorf("stun header magic cookie invalid; %v != %v (expected)", binary.BigEndian.Uint32(c), magicCookie)
	}
	return nil
}

// The message length MUST contain the size, in bytes, of the message
// not including the 20-byte STUN header.  Since all STUN attributes are
// padded to a multiple of 4 bytes, the last 2 bits of this field are
// always zero.  This provides another way to distinguish STUN packets
// from packets of other protocols.
// https://tools.ietf.org/html/rfc5389#section-6
func getMessageLength(header []byte) (uint16, error) {
	messageLength := binary.BigEndian.Uint16(header[messageLengthStart : messageLengthStart+messageLengthLength])
	if messageLength%4 != 0 {
		return 0, errors.Errorf("stun header message length must be a factor of 4 (%d)", messageLength)
	}

	return messageLength, nil
}

//  0                 1
//  2  3  4 5 6 7 8 9 0 1 2 3 4 5
//
// +--+--+-+-+-+-+-+-+-+-+-+-+-+-+
// |M |M |M|M|M|C|M|M|M|C|M|M|M|M|
// |11|10|9|8|7|1|6|5|4|0|3|2|1|0|
// +--+--+-+-+-+-+-+-+-+-+-+-+-+-+
const (
	c0Mask   = 0x10 // 0b10000
	c1Mask   = 0x01 // 0b00001
	c0ShiftR = 4    // R 0b10000 -> 0b00001
	c1ShiftL = 1    // L 0b00001 -> 0b00010

	m0Mask   = 0x0F // 0b00001111
	m4Mask   = 0xE0 // 0b11100000
	m7Mask   = 0x3E // 0b00111110
	m4ShiftR = 1    // R 0b01110000 -> 0b00111000
	m7ShiftL = 5    // L 0b00111110 -> 0b0000011111000000
)

func setMessageType(header []byte, class MessageClass, method Method) {
	m := uint16(method)
	c := uint16(class)

	mt := m & m0Mask
	// Make room for c0
	mt |= (m & (m4Mask >> m4ShiftR)) << 1
	mt |= (m & (m7Mask << 6)) << 2
	mt |= (c & 0x1) << 4
	mt |= (c >> 1) << 8

	binary.BigEndian.PutUint16(header[headerStart:], mt)
}

func getMessageType(header []byte) (MessageClass, Method) {
	mByte0 := header[0]
	mByte1 := header[1]

	c0 := (mByte1 & c0Mask) >> c0ShiftR
	c1 := (mByte0 & c1Mask) << c1ShiftL

	class := MessageClass(c1 | c0)

	var m uint16
	m = (uint16(mByte0) & m7Mask) << m7ShiftL
	m |= uint16(mByte1 & m0Mask)
	m |= uint16((mByte1 & m4Mask) >> m4ShiftR)

	method := Method(m)

	return class, method
}

//  0                   1                   2                   3
//  0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |         Type                  |            Length             |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |                         Value (variable)                ....
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
func getAttribute(attribute []byte) *RawAttribute {
	typ := AttrType(binary.BigEndian.Uint16(attribute))
	len := binary.BigEndian.Uint16(attribute[attrLengthStart : attrLengthStart+attrLengthLength])
	pad := (attrLengthMultiple - (len % attrLengthMultiple)) % attrLengthMultiple
	return &RawAttribute{typ, len, attribute[attrValueStart : attrValueStart+len], pad}
}

// TODO Break this apart, too big
func NewMessage(packet []byte) (*Message, error) {

	if len(packet) < 20 {
		return nil, errors.Errorf("stun header must be at least 20 bytes, was %d", len(packet))
	}

	header := packet[headerStart : headerStart+headerLength]

	if !verifyHeaderMostSignificant2Bits(header) {
		return nil, errors.New("stun header most significant 2 bits must equal 0b00")
	}

	err := verifyMagicCookie(header)
	if err != nil {
		return nil, errors.Wrap(err, "stun header invalid")
	}

	ml, err := getMessageLength(header)
	if err != nil {
		return nil, errors.Wrap(err, "stun header invalid")
	}

	if len(packet) != headerLength+int(ml) {
		return nil, errors.Errorf("stun header length invalid; %d != %d (expected)", headerLength+int(ml), len(packet))
	}

	t := header[transactionIDStart : transactionIDStart+transactionIDLength]

	class, method := getMessageType(header)

	ra := []*RawAttribute{}
	// TODO Check attr length <= attr slice remaining
	attr := packet[headerLength:]
	for len(attr) > 0 {
		a := getAttribute(attr)
		attr = attr[attrValueStart+a.Length+a.Pad:]
		ra = append(ra, a)
	}

	m := Message{}
	m.Class = class
	m.Method = method
	m.Length = ml
	m.TransactionID = t[0:transactionIDLength]
	m.Attributes = ra

	return &m, nil
}

func (m *Message) Pack() []byte {
	l := 0
	for _, v := range m.Attributes {
		l += int(4 + v.Length + v.Pad)
	}
	raw := make([]byte, headerLength+l)

	setMessageType(raw[headerStart:2], m.Class, m.Method)
	binary.BigEndian.PutUint16(raw[messageLengthStart:], uint16(l))
	copy(raw[transactionIDStart:], m.TransactionID)

	attrPos := headerLength
	for _, v := range m.Attributes {
		attrPos += v.Pack(raw[attrPos:])
	}

	return raw
}
