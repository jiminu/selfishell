package selfishell

import (
	"bytes"
	"testing"

	"github.com/jiminu/selfishell/internal/pty"
)

func TestTerminalWidthTracksRealTerminalSize(t *testing.T) {
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer master.Close()
	defer slave.Close()
	for _, columns := range []uint16{40, 120, 0} {
		if err := pty.Resize(slave, 24, columns); err != nil {
			t.Fatal(err)
		}
		want := int(columns)
		if want == 0 {
			want = 80
		}
		if got := terminalWidth(slave); got != want {
			t.Fatalf("terminal columns %d: got %d, want %d", columns, got, want)
		}
	}
	if got := terminalWidth(&bytes.Buffer{}); got != 80 {
		t.Fatalf("plain stream width: %d", got)
	}
}
