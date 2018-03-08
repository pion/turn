package stun

import (
	"github.com/pkg/errors"
	"golang.org/x/net/ipv4"
)
import "encoding/binary"

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
	headerStart         int  = 0
	mostSig2BitsMask    uint = 0x3
	mostSig2BitsShift   uint = 6
	headerLength        int  = 20
	messageLengthStart  int  = 2
	messageLengthLength int  = 2
	magicCookieStart    int  = 4
	magicCookieLength   int  = 4
	transactionIDStart  int  = 8
	transactionIDLength int  = 12
)

type Message struct {
	Length uint16
}

// https://tools.ietf.org/html/rfc5389#section-6
// The most significant 2 bits of every STUN message MUST be zeroes.
// This can be used to differentiate STUN packets from other protocols
// when STUN is multiplexed with other protocols on the same port.
func verifyHeaderMostSignificant2Bits(header []byte) bool {
	return ((uint(header[headerStart]) & mostSig2BitsMask) >> mostSig2BitsShift) == 0
}

// https://tools.ietf.org/html/rfc5389#section-6
// The message length MUST contain the size, in bytes, of the message
// not including the 20-byte STUN header.  Since all STUN attributes are
// padded to a multiple of 4 bytes, the last 2 bits of this field are
// always zero.  This provides another way to distinguish STUN packets
// from packets of other protocols.
func getMessageLength(header []byte) (uint16, error) {
	messageLength := binary.BigEndian.Uint16(header[messageLengthStart : messageLengthStart+messageLengthLength])
	if messageLength%4 != 0 {
		return 0, errors.Errorf("stun header message length must be a factor of 4 (%d)", messageLength)
	}

	return messageLength, nil
}

func NewMessage(packet []byte) (*Message, error) {

	if len(packet) < 20 {
		return nil, errors.Errorf("stun header must be at least 20 bytes, was %d", len(packet))
	}

	header := packet[headerStart : headerStart+headerLength]

	if !verifyHeaderMostSignificant2Bits(header) {
		return nil, errors.New("stun header most significant 2 bits must equal 0b00")
	}

	ml, err := getMessageLength(header)
	if err != nil {
		return nil, err
	}

	m := Message{}
	m.Length = ml

	return &m, nil
}
