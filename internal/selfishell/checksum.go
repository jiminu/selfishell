package selfishell

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// Checksum retains POSIX cksum CRC:SIZE semantics, including binary bytes.
// Managed-path links and special files are user data, not checksum candidates.
func Checksum(ctx context.Context, path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("checksum requires a regular file: %s", path)
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("checksum requires a regular file: %s", path)
	}
	var out, stderr bytes.Buffer
	status, err := (Process{In: file, Out: &out, Err: &stderr}).Run(ctx, "cksum")
	if err != nil {
		return "", fmt.Errorf("checksum %s: %w", path, err)
	}
	if status != 0 {
		return "", fmt.Errorf("checksum %s: cksum exited %d: %s", path, status, strings.TrimSpace(stderr.String()))
	}
	return parseChecksum(out.String(), path)
}

func checksumBytes(data []byte) (string, error) {
	var out, stderr bytes.Buffer
	code, err := (Process{In: bytes.NewReader(data), Out: &out, Err: &stderr}).Run(context.Background(), "cksum")
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("cksum exited %d: %s", code, stderr.String())
	}
	return parseChecksum(out.String(), "bytes")
}

func parseChecksum(output, path string) (string, error) {
	fields := strings.Fields(output)
	if len(fields) != 2 {
		return "", fmt.Errorf("invalid cksum output for %s", path)
	}
	for index, bits := range []int{32, 64} {
		if strings.IndexFunc(fields[index], func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return "", fmt.Errorf("invalid cksum output for %s", path)
		}
		if _, err := strconv.ParseUint(fields[index], 10, bits); err != nil {
			return "", fmt.Errorf("invalid cksum output for %s: %w", path, err)
		}
	}
	return fields[0] + ":" + fields[1], nil
}
