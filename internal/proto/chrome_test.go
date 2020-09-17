package proto

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/pion/stun"
)

func TestChromeAllocRequest(t *testing.T) {
	var (
		r = bytes.NewReader(loadData(t, "01_chromeallocreq.hex"))
		s = bufio.NewScanner(r)

		data     [][]byte
		messages []*stun.Message
	)
	// Decoding hex data into binary.
	for s.Scan() {
		b, err := hex.DecodeString(s.Text())
		if err != nil {
			t.Fatal(err)
		}
		data = append(data, b)
	}
	// All hex streams decoded to raw binary format and stored in data slice.
	// Decoding packets to messages.
	for i, packet := range data {
		m := new(stun.Message)
		if _, err := m.Write(packet); err != nil {
			t.Errorf("Packet %d: %v", i, err)
		}
		messages = append(messages, m)
	}
	if len(messages) != 4 {
		t.Error("unexpected message slice list")
	}
}
