package selfishell

import (
	"bytes"
	"strings"
	"testing"
)

func nativeCLI(t *testing.T, root string, args ...string) (int, string, string) {
	t.Helper()
	var out, stderr bytes.Buffer
	code := (CLI{Root: root, In: strings.NewReader(""), Out: &out, Err: &stderr}).Run(args)
	return code, out.String(), stderr.String()
}
