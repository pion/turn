package proto

import "testing"

func TestParseURI(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		out  URI
	}{
		{
			name: "default",
			in:   "turn:example.org",
			out: URI{
				Host:   "example.org",
				Scheme: Scheme,
			},
		},
		{
			name: "secure",
			in:   "turns:example.org",
			out: URI{
				Host:   "example.org",
				Scheme: SchemeSecure,
			},
		},
		{
			name: "with port",
			in:   "turn:example.org:8000",
			out: URI{
				Host:   "example.org",
				Scheme: Scheme,
				Port:   8000,
			},
		},
		{
			name: "with port and transport",
			in:   "turn:example.org:8000?transport=tcp",
			out: URI{
				Host:      "example.org",
				Scheme:    Scheme,
				Port:      8000,
				Transport: TransportTCP,
			},
		},
		{
			name: "with transport",
			in:   "turn:example.org?transport=udp",
			out: URI{
				Host:      "example.org",
				Scheme:    Scheme,
				Transport: TransportUDP,
			},
		},
		{
			name: "with port and custom transport",
			in:   "turns:example.org:8000?transport=quic",
			out: URI{
				Host:      "example.org",
				Scheme:    SchemeSecure,
				Port:      8000,
				Transport: "quic",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, parseErr := ParseURI(tc.in)
			if parseErr != nil {
				t.Fatal(parseErr)
			}
			if out != tc.out {
				t.Errorf("%s != %s", out, tc.out)
			}
		})
	}
	t.Run("MustFail", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			in   string
		}{
			{
				name: "hierarchical",
				in:   "turn://example.org",
			},
			{
				name: "bad port",
				in:   "turn:example.org:port",
			},
			{
				name: "bad scheme",
				in:   "tcp:example.org",
			},
			{
				name: "invalid uri scheme",
				in:   "turn_s:test",
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				_, parseErr := ParseURI(tc.in)
				if parseErr == nil {
					t.Fatal("should fail, but did not")
				}
			})
		}
	})
}

func TestURI_String(t *testing.T) {
	for _, tc := range []struct {
		name string
		uri  URI
		out  string
	}{
		{
			name: "blank",
			out:  ":",
		},
		{
			name: "simple",
			uri: URI{
				Host:   "example.org",
				Scheme: Scheme,
			},
			out: "turn:example.org",
		},
		{
			name: "secure",
			uri: URI{
				Host:   "example.org",
				Scheme: SchemeSecure,
			},
			out: "turns:example.org",
		},
		{
			name: "secure with port",
			uri: URI{
				Host:   "example.org",
				Scheme: SchemeSecure,
				Port:   443,
			},
			out: "turns:example.org:443",
		},
		{
			name: "secure with transport",
			uri: URI{
				Host:      "example.org",
				Scheme:    SchemeSecure,
				Port:      443,
				Transport: "tcp",
			},
			out: "turns:example.org:443?transport=tcp",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if v := tc.uri.String(); v != tc.out {
				t.Errorf("%q != %q", v, tc.out)
			}
		})
	}
}
