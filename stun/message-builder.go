package stun

import "github.com/pkg/errors"

type MessageBuilder struct {
	attributes []*Attribute
}

func Build(class MessageClass, method Method, transactionID []byte, attrs ...Attribute) (*Message, error) {
	m := &Message{
		Class:         class,
		Method:        method,
		TransactionID: transactionID,
	}

	for _, v := range attrs {
		ra, err := v.Pack(m)
		if err != nil {
			return nil, errors.Wrap(err, "failed packing attribute")
		}
		m.Attributes = append(m.Attributes, ra)
	}

	return m, nil
}
