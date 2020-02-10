package proto

import (
	"github.com/pion/stun"
	"testing"
)

type TestCase struct {
	protocol Protocol
	expected string
}

func TestRequestedTransport(t *testing.T) {

	testCases := []TestCase{
		{protocol: ProtoTCP, expected: "protocol: TCP"},
		{protocol: ProtoUDP, expected: "protocol: UDP"},
		{protocol: 254, expected: "protocol: 254"},
	}

	for _, testCase := range testCases {

		r := RequestedTransport{
			Protocol: testCase.protocol,
		}

		t.Run("String when set with Protocol type", func(t *testing.T) {
			assertStrings(t, r.String(), testCase.expected)
		})

		t.Run("NoAlloc", func(t *testing.T) {
			m := &stun.Message{}
			if wasAllocs(func() {
				// On stack.
				r.AddTo(m) //nolint
				m.Reset()
			}) {
				t.Error("Unexpected allocations")
			}

			if wasAllocs(func() {
				// On heap.
				r.AddTo(m) //nolint
				m.Reset()
			}) {
				t.Error("Unexpected allocations")
			}
		})
		t.Run("AddTo", func(t *testing.T) {
			m := new(stun.Message)
			if err := r.AddTo(m); err != nil {
				t.Error(err)
			}
			m.WriteHeader()
			t.Run("GetFrom", func(t *testing.T) {
				decoded := new(stun.Message)
				if _, err := decoded.Write(m.Raw); err != nil {
					t.Fatal("failed to decode message:", err)
				}
				req := RequestedTransport{
					Protocol: testCase.protocol,
				}
				if err := req.GetFrom(decoded); err != nil {
					t.Fatal(err)
				}
				if req != r {
					t.Errorf("Decoded %q, expected %q", req, r)
				}
				if wasAllocs(func() {
					r.GetFrom(decoded) //nolint
				}) {
					t.Error("Unexpected allocations")
				}
				t.Run("HandleErr", func(t *testing.T) {
					m := new(stun.Message)
					var handle RequestedTransport
					if err := handle.GetFrom(m); err != stun.ErrAttributeNotFound {
						t.Errorf("%v should be not found", err)
					}
					m.Add(stun.AttrRequestedTransport, []byte{1, 2, 3})
					if !stun.IsAttrSizeInvalid(handle.GetFrom(m)) {
						t.Error("IsAttrSizeInvalid should be true")
					}
				})
			})
		})
	}

}

func assertStrings(t *testing.T, got, want string) {
	t.Helper()

	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}