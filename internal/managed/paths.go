package managed

import "fmt"

// Paths are Selfishell's configuration, state, resource-record, cache and data
// directories.
type Paths struct{ Config, State, Resources, Cache, Data string }

// UserPaths only resolves names. Discovery must not create managed state.
// getenv is the caller's lookup, such as Environment.Get.
func UserPaths(getenv func(string) string) (Paths, error) {
	home := getenv("HOME")
	if home == "" {
		return Paths{}, fmt.Errorf("HOME must be set")
	}
	value := func(name, fallback string) string {
		if v := getenv(name); v != "" {
			return v
		}
		return fallback
	}
	// Keep the caller's spelling: cleaning symlink/.. can select another directory.
	config := value("XDG_CONFIG_HOME", home+"/.config") + "/selfishell"
	state := value("XDG_STATE_HOME", home+"/.local/state") + "/selfishell"
	return Paths{Config: config, State: state, Resources: state + "/resources", Cache: value("XDG_CACHE_HOME", home+"/.cache") + "/selfishell", Data: value("XDG_DATA_HOME", home+"/.local/share") + "/selfishell"}, nil
}
