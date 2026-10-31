package selfishell

import "github.com/jiminu/selfishell/internal/process"

// setEnv replaces t.Setenv: a test changes only its own environment, so it can
// run in parallel. A subtest copies it first (env := env); With never shares entries.
func setEnv(e *process.Environment, key, value string) {
	*e = e.With(map[string]string{key: value})
}
