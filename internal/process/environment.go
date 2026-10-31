package process

import (
	"os"
	"slices"
	"strings"
)

// Environment is the variable list an invocation reads and hands to its
// children. A nil Environment is the process environment; tests inject their
// own so they need not change the shared process environment.
type Environment []string

// HostEnvironment is the only reader of the real process environment.
func HostEnvironment() Environment { return os.Environ() }

func (e Environment) entries() []string {
	if e == nil {
		return HostEnvironment()
	}
	return e
}

// Lookup returns the last entry for name, as exec's deduplication does.
func (e Environment) Lookup(name string) (string, bool) {
	value, found := "", false
	for _, entry := range e.entries() {
		if v, ok := strings.CutPrefix(entry, name+"="); ok {
			value, found = v, true
		}
	}
	return value, found
}

// Get returns name's value, or "" when it is unset.
func (e Environment) Get(name string) string {
	value, _ := e.Lookup(name)
	return value
}

// Default returns name's value, or fallback when it is unset or empty.
func (e Environment) Default(name, fallback string) string {
	if value := e.Get(name); value != "" {
		return value
	}
	return fallback
}

// List returns a copy suitable for a child process.
func (e Environment) List() []string { return slices.Clone(e.entries()) }

// With returns a copy with set applied and unset removed.
func (e Environment) With(set map[string]string, unset ...string) Environment {
	remove := map[string]bool{}
	for key := range set {
		remove[key] = true
	}
	for _, key := range unset {
		remove[key] = true
	}
	entries := e.entries()
	out := make(Environment, 0, len(entries)+len(set))
	for _, entry := range entries {
		key, _, _ := strings.Cut(entry, "=")
		if !remove[key] {
			out = append(out, entry)
		}
	}
	for key, value := range set {
		out = append(out, key+"="+value)
	}
	return out
}

// Environment copies the child's environment: Env, or else Inherit.
func (p Process) Environment() Environment {
	if p.Env != nil {
		return Environment(p.Env).List()
	}
	return p.Inherit.List()
}

// WithEnvironment returns a copy of p whose child environment has set applied
// and unset removed.
func WithEnvironment(p Process, set map[string]string, unset ...string) Process {
	p.Env = p.Environment().With(set, unset...)
	return p
}
