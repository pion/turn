package ltcred

// Feedback is a way to receive debug information from the AuthHandler
type Feedback interface {
	Print(...interface{})
}

type noFeedback int

func (noFeedback) Print(...interface{}) {}
