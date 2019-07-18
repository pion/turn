// +build gofuzz

package proto

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func corpus(t *testing.T, function, typ string) [][]byte {
	var data [][]byte
	p := filepath.Join("fuzz", function, typ)
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("does not exist")
		}
		t.Fatal(err)
	}
	list, err := f.Readdir(-1)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range list {
		if strings.Contains(d.Name(), ".") {
			// Skipping non-raw files.
			continue
		}
		df, err := os.Open(filepath.Join(p, d.Name()))
		if err != nil {
			t.Fatal(err)
		}
		buf := make([]byte, 5000)
		n, _ := df.Read(buf)
		data = append(data, buf[:n])
		df.Close()
	}
	return data
}

func TestFuzzSetters_Crashers(t *testing.T) {
	for _, buf := range corpus(t, "turn-setters", "crashers") {
		FuzzSetters(buf)
	}
}

func TestFuzzSetters_Coverage(t *testing.T) {
	for _, buf := range corpus(t, "turn-setters", "corpus") {
		FuzzSetters(buf)
	}
}
