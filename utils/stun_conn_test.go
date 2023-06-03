//go:build !js
// +build !js

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConsumeSingleTURNFrame(t *testing.T) {
	type testCase struct {
		data []byte
		err  error
	}
	cases := map[string]testCase{
		"channel data":                          {data: []byte{0x40, 0x01, 0x00, 0x08, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}, err: nil},
		"partial data less than channel header": {data: []byte{1}, err: errIncompleteTURNFrame},
		"partial stun message":                  {data: []byte{0x0, 0x16, 0x02, 0xDC, 0x21, 0x12, 0xA4, 0x42, 0x0, 0x0, 0x0}, err: errIncompleteTURNFrame},
		"stun message":                          {data: []byte{0x0, 0x16, 0x00, 0x02, 0x21, 0x12, 0xA4, 0x42, 0xf7, 0x43, 0x81, 0xa3, 0xc9, 0xcd, 0x88, 0x89, 0x70, 0x58, 0xac, 0x73, 0x0, 0x0}},
	}

	for name, cs := range cases {
		c := cs
		t.Run(name, func(t *testing.T) {
			n, e := consumeSingleTURNFrame(c.data)
			assert.Equal(t, c.err, e)
			if e == nil {
				assert.Equal(t, len(c.data), n)
			}
		})
	}
}
