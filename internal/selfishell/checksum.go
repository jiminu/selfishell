package selfishell

import (
	"bytes"
	"context"
	"fmt"
	"hash/crc32"
	"io"
	"math/bits"
	"os"
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
	sum, err := checksumReader(ctx, file)
	if err != nil {
		return "", fmt.Errorf("checksum %s: %w", path, err)
	}
	return sum, nil
}

func checksumBytes(data []byte) (string, error) {
	return checksumReader(context.Background(), bytes.NewReader(data))
}

func checksumReader(ctx context.Context, input io.Reader) (string, error) {
	// POSIX cksum uses the IEEE polynomial with an initial zero register,
	// most-significant bit first. Reverse each byte to use Go's reflected CRC,
	// then reverse the result; Update already applies the final complement.
	crc := ^uint32(0)
	var size uint64
	var buf [32 * 1024]byte
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, err := input.Read(buf[:])
		size += uint64(n)
		for i := range buf[:n] {
			buf[i] = bits.Reverse8(buf[i])
		}
		crc = crc32.Update(crc, crc32.IEEETable, buf[:n])
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	// Append the byte length, least-significant byte first, without leading zeros.
	for length := size; length != 0; length >>= 8 {
		buf[0] = bits.Reverse8(byte(length))
		crc = crc32.Update(crc, crc32.IEEETable, buf[:1])
	}
	return fmt.Sprintf("%d:%d", bits.Reverse32(crc), size), nil
}
