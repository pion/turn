package stun

// https://tools.ietf.org/html/rfc5389#section-15.10
// The SOFTWARE attribute contains a textual description of the software
//  being used by the agent sending the message
type Software struct {
	Software string
}

func (s *Software) Pack(message *Message) error {
	message.AddAttribute(AttrSoftware, []byte(s.Software))
	return nil
}

func (s *Software) Unpack(message *Message, rawAttribute *RawAttribute) error {
	s.Software = string(rawAttribute.Value)
	return nil
}
