package lifecycle

import "github.com/jiminu/selfishell/internal/process"

// setEnv copies the internal/selfishell test helper.
func setEnv(e *process.Environment, key, value string) {
	*e = e.With(map[string]string{key: value})
}
