package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAtomicBool(t *testing.T) {
	b0 := NewAtomicBool(false)
	assert.False(t, b0.True(), "should false")
	assert.True(t, b0.False(), "should false")

	b1 := NewAtomicBool(true)
	assert.True(t, b1.True(), "should true")
	assert.False(t, b1.False(), "should true")

	b0.SetToTrue()
	assert.True(t, b0.True(), "should true")
	assert.False(t, b0.False(), "should true")
	b0.SetToFalse()
	assert.False(t, b0.True(), "should false")
	assert.True(t, b0.False(), "should false")
}
