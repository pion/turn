package proto

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

const allocRuns = 10

// wasAllocs returns true if f allocates memory.
func wasAllocs(f func()) bool {
	return testing.AllocsPerRun(allocRuns, f) > 0
}

func loadData(tb testing.TB, name string) []byte {
	name = filepath.Join("testdata", name)
	f, err := os.Open(name) // #nosec
	if err != nil {
		tb.Fatal(err)
	}
	defer func() {
		if errClose := f.Close(); errClose != nil {
			tb.Fatal(errClose)
		}
	}()
	v, err := ioutil.ReadAll(f)
	if err != nil {
		tb.Fatal(err)
	}
	return v
}
