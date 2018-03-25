package stun

import "github.com/pkg/errors"

// https://tools.ietf.org/html/rfc5389#section-15.3
// The USERNAME attribute is used for message integrity.  It identifies
// the username and password combination used in the message-integrity
// check.
//
// The value of USERNAME is a variable-length value.  It MUST contain a
// UTF-8 [RFC3629] encoded sequence of less than 513 bytes, and MUST
// have been processed using SASLprep [RFC4013].
type Username struct {
	Username string
}

const (
	usernameMaxLength = 513
)

func (u *Username) Pack(message *Message) error {
	if len([]byte(u.Username)) > usernameMaxLength {
		return errors.Errorf("invalid username length %d", len([]byte(u.Username)))
	}
	message.AddAttribute(AttrSoftware, []byte(u.Username))
	return nil
}

func (u *Username) Unpack(message *Message, rawAttribute *RawAttribute) error {
	u.Username = string(rawAttribute.Value)
	return nil
}
