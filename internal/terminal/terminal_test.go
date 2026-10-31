package terminal

import (
	"bytes"
	"os"
	"testing"
)

func TestTerminal(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if IsTerminal(f) {
		t.Fatal("/dev/null is a character device but not a terminal")
	}
	if IsTerminal(&bytes.Buffer{}) {
		t.Fatal("buffer is not a terminal")
	}
}
