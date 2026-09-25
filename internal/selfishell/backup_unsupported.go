//go:build !darwin && (!linux || (!amd64 && !arm64))

package selfishell

import "fmt"

func moveBackupNoReplace(source, destination string) error {
	return fmt.Errorf("exclusive backup move is unsupported on this platform")
}
