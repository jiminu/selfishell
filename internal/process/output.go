package process

import (
	"bytes"
	"context"
	"fmt"
	"strings"
)

// Output runs a child through p with captured streams and returns its trimmed
// standard output. A nonzero exit becomes an error carrying the trimmed
// standard error, so callers never read partial output as a result.
func Output(ctx context.Context, p Process, name string, args ...string) (string, error) {
	var out, stderr bytes.Buffer
	p.Out, p.Err = &out, &stderr
	code, err := p.Run(ctx, name, args...)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("%s exited %d: %s", name, code, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(out.String()), nil
}
