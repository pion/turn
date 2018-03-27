package stun

import "github.com/pkg/errors"

// https://tools.ietf.org/html/rfc5389#section-15.7
// The REALM attribute may be present in requests and responses.  It
// contains text that meets the grammar for "realm-value" as described
// in RFC 3261 [RFC3261] but without the double quotes and their
// surrounding whitespace.  That is, it is an unquoted realm-value (and
// is therefore a sequence of qdtext or quoted-pair).  It MUST be a
// UTF-8 [RFC3629] encoded sequence of less than 128 characters (which
// can be as long as 763 bytes), and MUST have been processed using
// SASLprep [RFC4013].
//
// Presence of the REALM attribute in a request indicates that long-term
// credentials are being used for authentication.  Presence in certain
// error responses indicates that the server wishes the client to use a
// long-term credential for authentication.
type Realm struct {
	Realm string
}

const (
	realmMaxLength = 763
)

func (r *Realm) Pack(message *Message) error {
	if len([]byte(r.Realm)) > realmMaxLength {
		return errors.Errorf("invalid realm length %d", len([]byte(r.Realm)))
	}
	message.AddAttribute(AttrRealm, []byte(r.Realm))
	return nil
}

func (r *Realm) Unpack(message *Message, rawAttribute *RawAttribute) error {
	r.Realm = string(rawAttribute.Value)
	return nil
}
