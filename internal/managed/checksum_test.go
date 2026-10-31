package managed

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"testing/iotest"

	"github.com/jiminu/selfishell/internal/testutil"
)

func TestChecksum(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// The host utility is an independent oracle, not a runtime dependency.
	cksum, err := exec.LookPath("cksum")
	if err != nil {
		t.Fatal(err)
	}
	vectors := [][]byte{nil, []byte("a\r\nb\r\n"), []byte("no final newline"), {0, 255, 128, 0, 10}, bytes.Repeat([]byte{0, 1, 128, 255, 13, 10}, 10000)}
	for _, size := range []int{1, 255, 256, 257, 32767, 32768, 32769, 65535, 65536, 65537, 1048577} {
		data := make([]byte, size)
		for i := range data {
			data[i] = byte(i*31 + 7)
		}
		vectors = append(vectors, data)
	}
	for _, data := range vectors {
		path := dir + "/input with spaces"
		if err := testutil.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(cksum)
		cmd.Stdin = bytes.NewReader(data)
		output, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		fields := strings.Fields(string(output))
		want := fields[0] + ":" + fields[1]
		got, err := Checksum(context.Background(), path)
		if err != nil || got != want {
			t.Fatalf("%d bytes: %q %v want %q", len(data), got, err, want)
		}
		original := bytes.Clone(data)
		got, err = ChecksumBytes(data)
		if err != nil || got != want || !bytes.Equal(data, original) {
			t.Fatalf("%d bytes in memory: %q %v want %q; mutated=%v", len(data), got, err, want, !bytes.Equal(data, original))
		}
	}
	if got, err := Checksum(context.Background(), dir+"/missing"); got != "" || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing: %q %v", got, err)
	}
	if got, err := Checksum(context.Background(), dir); got != "" || err == nil {
		t.Fatalf("directory: %q %v", got, err)
	}
	link := dir + "/link"
	if err := os.Symlink(dir+"/input with spaces", link); err != nil {
		t.Fatal(err)
	}
	if got, err := Checksum(context.Background(), link); got != "" || err == nil {
		t.Fatalf("replaced link: %q %v", got, err)
	}
	fifo := dir + "/fifo"
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := Checksum(context.Background(), fifo); got != "" || err == nil {
		t.Fatalf("fifo: %q %v", got, err)
	}
}

func TestChecksumCanceled(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := dir + "/input"
	if err := testutil.WriteFile(path, []byte("bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := Checksum(ctx, path); got != "" || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled checksum: %q %v", got, err)
	}
}

func TestChecksumReader(t *testing.T) {
	for _, input := range []io.Reader{
		strings.NewReader("123456789"),
		iotest.OneByteReader(strings.NewReader("123456789")),
		iotest.DataErrReader(strings.NewReader("123456789")),
	} {
		if got, err := checksumReader(context.Background(), input); err != nil || got != "930766865:9" {
			t.Fatalf("reader checksum: %q %v", got, err)
		}
	}
	file, err := os.CreateTemp(t.TempDir(), "closed")
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	input := io.MultiReader(strings.NewReader("partial bytes"), file)
	if got, err := checksumReader(context.Background(), input); got != "" || !errors.Is(err, os.ErrClosed) {
		t.Fatalf("partial read failure returned checksum: %q %v", got, err)
	}
	for _, input := range []io.Reader{
		strings.NewReader("bytes"),
		iotest.DataErrReader(strings.NewReader("bytes")),
	} {
		ctx, cancel := context.WithCancel(context.Background())
		got, err := checksumReader(ctx, checksumCancelReader{input, cancel})
		cancel()
		if got != "" || !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled during read: %q %v", got, err)
		}
	}
}

type checksumCancelReader struct {
	io.Reader
	cancel context.CancelFunc
}

func (r checksumCancelReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.cancel()
	return n, err
}
