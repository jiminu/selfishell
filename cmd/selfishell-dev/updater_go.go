package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var goPatchVersion = regexp.MustCompile(`^1\.[0-9]+\.[0-9]+$`)
var goDirective = regexp.MustCompile(`(?m)^go (1\.[0-9]+\.[0-9]+)[ \t]*$`)
var goDirectiveStart = regexp.MustCompile(`(?m)^[ \t]*go(?:[ \t]|$)`)
var localGoPin = regexp.MustCompile(`(?m)^[ \t]*go[ \t]*=[ \t]*"([^"]+)"[ \t]*(?:#.*)?$`)
var localGoPinStart = regexp.MustCompile(`(?m)^[ \t]*go[ \t]*=`)

func goToolchainVersion(mod string) (string, error) {
	matches := goDirective.FindAllStringSubmatch(mod, -1)
	if len(matches) != 1 || len(goDirectiveStart.FindAllString(mod, -1)) != 1 {
		return "", fmt.Errorf("expected one exact Go patch version in go.mod")
	}
	return matches[0][1], nil
}

func goReleaseLine(version string) string {
	return version[:strings.LastIndexByte(version, '.')+1]
}

func latestGoPatch(data []byte, current string) (string, error) {
	var releases []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	}
	if err := json.Unmarshal(data, &releases); err != nil {
		return "", fmt.Errorf("invalid Go release metadata: %w", err)
	}
	selected := ""
	for _, release := range releases {
		version, ok := strings.CutPrefix(release.Version, "go")
		if !ok || !release.Stable || !goPatchVersion.MatchString(version) || goReleaseLine(version) != goReleaseLine(current) {
			continue
		}
		if selected == "" || newerToolVersion(version, selected) {
			selected = version
		}
	}
	if selected == "" {
		return "", fmt.Errorf("no supported Go patch release for %s; review the Go release line", current)
	}
	return selected, nil
}

func updateGoToolchain(mod, mise *dependencyEdit, candidate string) error {
	current, err := goToolchainVersion(mod.after)
	if err != nil {
		return err
	}
	pins := localGoPin.FindAllStringSubmatch(mise.after, -1)
	if len(pins) != 1 || len(localGoPinStart.FindAllString(mise.after, -1)) != 1 || pins[0][1] != current {
		return fmt.Errorf("root mise.toml Go pin must match go.mod")
	}
	if goReleaseLine(candidate) != goReleaseLine(current) {
		return fmt.Errorf("Go release line changes require maintainer review: %s -> %s", current, candidate)
	}
	if !newerToolVersion(candidate, current) {
		return nil
	}
	mise.after, err = updateMisePin(mise.after, "go", candidate)
	if err != nil {
		return err
	}
	mod.after = goDirective.ReplaceAllString(mod.after, "go "+candidate)
	return nil
}
