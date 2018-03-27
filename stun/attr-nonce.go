package stun

import "github.com/pkg/errors"

// https://tools.ietf.org/html/rfc5389#section-15.8
// The NONCE attribute may be present in requests and responses.  It
// contains a sequence of qdtext or quoted-pair, which are defined in
// RFC 3261 [RFC3261].  Note that this means that the NONCE attribute
// will not contain actual quote characters.  See RFC 2617 [RFC2617],
// Section 4.3, for guidance on selection of nonce values in a server.
//
// It MUST be less than 128 characters (which can be as long as 763
// bytes).
type Nonce struct {
	Nonce string
}

const (
	nonceMaxLength = 763
)

func (n *Nonce) Pack(message *Message) error {
	if len([]byte(n.Nonce)) > nonceMaxLength {
		return errors.Errorf("invalid nonce length %d", len([]byte(n.Nonce)))
	}
	message.AddAttribute(AttrNonce, []byte(n.Nonce))
	return nil
}

func (n *Nonce) Unpack(message *Message, rawAttribute *RawAttribute) error {
	n.Nonce = string(rawAttribute.Value)
	return nil
}
