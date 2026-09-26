package releasebuild

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInvalidVersionDoesNotCreateOutput(t *testing.T) {
	for _, version := range []string{"", "v1.2.3", "01.2.3", "1.2.3+local"} {
		output := filepath.Join(t.TempDir(), "absent")
		if err := Build(context.Background(), ".", version, output); err == nil || !strings.Contains(err.Error(), "semantic version") {
			t.Errorf("version %q: error %v", version, err)
		}
		if _, err := os.Lstat(output); !os.IsNotExist(err) {
			t.Errorf("version %q created output: %v", version, err)
		}
	}
}
