package turn

import (
	"encoding/binary"

	"gitlab.com/pions/pion/turn/stun"
	"gitlab.com/pions/pion/vendor.orig/github.com/pkg/errors"
)

type Lifetime struct {
	Duration uint32
}

func (x *Lifetime) Pack(message *stun.Message) (*stun.RawAttribute, error) {
	ra := stun.RawAttribute{
		Type:   stun.AttrLifetime,
		Length: 4,
		Pad:    0,
	}
	v := make([]byte, 4)

	binary.BigEndian.PutUint32(v, x.Duration)

	return &ra, nil
}

func (x *Lifetime) Unpack(message *stun.Message, rawAttribute *stun.RawAttribute) error {
	v := rawAttribute.Value

	if len(v) != 4 {
		return errors.Errorf("invalid lifetime length %d != %d (expected)", len(v), 4)
	}

	x.Duration = binary.BigEndian.Uint32(v)

	return nil
}
