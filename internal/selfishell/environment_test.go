package selfishell

// set replaces t.Setenv: a test changes only its own environment, so it can run
// in parallel. A subtest copies it first (env := env); With never shares entries.
func (e *Environment) set(key, value string) {
	*e = e.With(map[string]string{key: value})
}
