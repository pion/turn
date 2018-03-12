package stun

type MessageBuilder struct {
	attributes []*Attribute
}

//func (mb *MessageBuilder) AddAttribute(attrs ...*Attribute) *MessageBuilder {
//	mb.attributes = append(mb.attributes, attrs...)
//}
//
//func (mb *MessageBuilder) Build(class MessageClass, method Method, transactionID []byte) {
//
//}
