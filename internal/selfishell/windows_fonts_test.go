package selfishell

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/jiminu/selfishell/internal/testutil"
)

type fontRegistrationRequest struct {
	Operation             string   `json:"operation"`
	Path                  string   `json:"path"`
	PreviousPath          string   `json:"previousPath"`
	AlternatePreviousPath string   `json:"alternatePreviousPath"`
	PreviousPaths         []string `json:"previousPaths"`
}

// Simulate the Windows registration boundary while exercising real downloads,
// ownership records, versioned activation, and pending-journal recovery in Go.
func TestWindowsFontRegistrationProcess(t *testing.T) {
	if os.Getenv("SELFISHELL_TEST_FONT_PROCESS") != "1" {
		return
	}
	// PowerShell uses Windows' 32,767-character process command-line limit.
	if len(os.Args[len(os.Args)-1])+90 > 32767 {
		fmt.Fprintln(os.Stderr, "Windows command line length exceeded")
		os.Exit(1)
	}
	raw, err := base64.StdEncoding.DecodeString(os.Args[len(os.Args)-1])
	if err != nil || len(raw)%2 != 0 {
		t.Fatal("invalid PowerShell command", err)
	}
	words := make([]uint16, len(raw)/2)
	for i := range words {
		words[i] = binary.LittleEndian.Uint16(raw[i*2:])
	}
	_, encoded, ok := strings.Cut(string(utf16.Decode(words)), "FromBase64String('")
	if !ok {
		t.Fatal("missing Windows request")
	}
	encoded, _, _ = strings.Cut(encoded, "')")
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	var request fontRegistrationRequest
	if err := json.Unmarshal(data, &request); err != nil {
		t.Fatal(err)
	}
	if request.Operation == "font-status" {
		registrations := map[string]string{}
		if data, err := os.ReadFile(os.Getenv("HOME") + "/font-registration"); err == nil {
			registrations["Selfishell jetbrainsmono-regular (TrueType)"] = string(data)
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if err := json.NewEncoder(os.Stdout).Encode(windowsFontStatus{Registrations: registrations}); err != nil {
			t.Fatal(err)
		}
		os.Exit(0)
	}
	if request.Operation == "font-register" {
		home := os.Getenv("HOME")
		if _, err := os.Stat(home + "/fail-registration"); err == nil {
			os.Exit(1)
		}
		existing, err := os.ReadFile(home + "/font-registration")
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		owned := []string{request.Path, request.PreviousPath, request.AlternatePreviousPath}
		owned = append(owned, request.PreviousPaths...)
		if len(existing) != 0 && !slices.Contains(owned, string(existing)) {
			fmt.Fprintln(os.Stderr, "Existing Windows font registration is user data")
			os.Exit(1)
		}
		if err := testutil.WriteFile(home+"/font-registration", []byte(request.Path), 0600); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println(`{}`)
	os.Exit(0)
}

func TestWindowsFontRepeatedRegistrationFailuresRetainOwnership(t *testing.T) {
	root, home, paths, windowsHome := windowsTerminalFixture(t)
	blockOK(t, root, "install", "--skip-packages", "--windows-terminal", "--yes")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SELFISHELL_TEST_FONT_EXE", executable)
	for name, body := range map[string]string{
		"curl":           "exec /usr/bin/curl \"$@\"",
		"wslpath":        "printf '%s\\n' \"$2\"",
		"powershell.exe": "SELFISHELL_TEST_FONT_PROCESS=1 exec \"$SELFISHELL_TEST_FONT_EXE\" -test.run=^TestWindowsFontRegistrationProcess$ -- \"$@\"",
	} {
		if err := testutil.WriteFile(home+"/tools/"+name, []byte("#!/bin/sh\n"+body+"\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	payload := []byte("approved font")
	source, manifest := home+"/font-source", home+"/font-manifest"
	if err := testutil.WriteFile(source, payload, 0600); err != nil {
		t.Fatal(err)
	}
	install := func(version string) error {
		record := fmt.Sprintf("download jetbrainsmono-regular %s linux all file://%s %x .local/share/selfishell/fonts/Regular.ttf font\n", version, source, sha256.Sum256(payload))
		if err := testutil.WriteFile(manifest, []byte(record), 0600); err != nil {
			t.Fatal(err)
		}
		op := &PackageOperation{Process: Process{Out: io.Discard, Err: io.Discard}}
		return op.InstallDirect(context.Background(), paths, manifest, "required", "jetbrainsmono-regular", "ubuntu-wsl", "amd64", false)
	}
	if err := install("1"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.WriteFile(home+"/fail-registration", nil, 0600); err != nil {
		t.Fatal(err)
	}
	for pin := 2; pin <= 61; pin++ {
		version := fmt.Sprint(pin)
		if err := install(version); err == nil {
			t.Fatal("registration failure was ignored", version)
		}
		blockEqual(t, home+"/font-registration", []byte(windowsHome+"/Microsoft/Windows/Fonts/Selfishell/1/Regular.ttf"))
		if version == "3" {
			// Existing journals only stored the two legacy candidate fields.
			journal := paths.State + "/pending-fonts/jetbrainsmono-regular"
			var legacy map[string]json.RawMessage
			if err := json.Unmarshal(blockRead(t, journal), &legacy); err != nil {
				t.Fatal(err)
			}
			delete(legacy, "previousPaths")
			data, err := json.Marshal(legacy)
			if err != nil {
				t.Fatal(err)
			}
			if err := testutil.WriteFile(journal, data, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := os.Remove(home + "/fail-registration"); err != nil {
		t.Fatal(err)
	}
	if err := install("62"); err != nil {
		t.Fatal("lost ownership of the still-registered font", err)
	}
	blockEqual(t, home+"/font-registration", []byte(windowsHome+"/Microsoft/Windows/Fonts/Selfishell/62/Regular.ttf"))
	if _, err := os.Stat(paths.State + "/pending-fonts/jetbrainsmono-regular"); !os.IsNotExist(err) {
		t.Fatal("successful recovery retained pending state", err)
	}
	foreign := home + "/user-font.ttf"
	if err := testutil.WriteFile(home+"/font-registration", []byte(foreign), 0600); err != nil {
		t.Fatal(err)
	}
	if err := install("63"); err == nil || !strings.Contains(err.Error(), "user data") {
		t.Fatal("overwrote an unrelated font registration", err)
	}
	blockEqual(t, home+"/font-registration", []byte(foreign))
}
