package proto

import (
	"encoding/binary"
	"github.com/pion/stun"
)

//The CONNECTION-ID attribute uniquely identifies a peer data
//connection.  It is a 32-bit unsigned integral value.

// RFC 6022 Section 6.6.2.1
type ConnectionId uint32

// AddTo adds CONNECTION-ID to message.
func (c ConnectionId) AddTo(m *stun.Message) error {
	v := make([]byte, 4)
	binary.BigEndian.PutUint32(v, uint32(c))
	m.Add(stun.AttrConnectionID, v)
	return nil
}

// GetFrom decodes CONNECTION-ID from message.
func (c *ConnectionId) GetFrom(m *stun.Message) error {
	v, err := m.Get(stun.AttrConnectionID)
	if err != nil {
		return err
	}
	*c = ConnectionId(binary.BigEndian.Uint32(v))
	return nil
}

func (c ConnectionId) ToByteArray() []byte {
	v := make([]byte, 4)
	binary.BigEndian.PutUint32(v, uint32(c))
	return v
}