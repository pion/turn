package proto

import (
	"github.com/pion/stun"
)

// DontFragmentAttr represents DONT-FRAGMENT attribute.
type DontFragmentAttr struct{}

const dontFragmentSize = 0

// AddTo adds DONT-FRAGMENT attribute to message.
func (DontFragmentAttr) AddTo(m *stun.Message) error {
	m.Add(stun.AttrDontFragment, nil)
	return nil
}

// GetFrom decodes DONT-FRAGMENT from message.
func (d *DontFragmentAttr) GetFrom(m *stun.Message) error {
	v, err := m.Get(stun.AttrDontFragment)
	if err != nil {
		return err
	}
	if err = stun.CheckSize(stun.AttrDontFragment, len(v), dontFragmentSize); err != nil {
		return err
	}
	return nil
}

// IsSet returns true if DONT-FRAGMENT attribute is set.
func (DontFragmentAttr) IsSet(m *stun.Message) bool {
	_, err := m.Get(stun.AttrDontFragment)
	return err == nil
}
