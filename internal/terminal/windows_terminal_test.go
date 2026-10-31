package terminal

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/jiminu/selfishell/internal/process"
)

// Opt-in native probes never write Windows settings or font registrations.
func TestWindowsInteropReadOnly(t *testing.T) {
	if os.Getenv("SELFISHELL_TEST_WSL_INTEROP") != "1" {
		t.Skip("requires native WSL interoperability")
	}
	t.Parallel()
	if _, err := exec.LookPath("powershell.exe"); err != nil {
		t.Fatal(err)
	}
	distro := os.Getenv("WSL_DISTRO_NAME")
	if distro == "" {
		t.Skip("requires a native WSL distribution identity")
	}
	p := process.Process{Env: process.WithEnvironment(process.Process{}, map[string]string{"HOME": t.TempDir()}).Env}
	data, err := WindowsScript(context.Background(), p, map[string]string{"operation": "probe", "distro": distro})
	if err != nil {
		t.Fatal(err)
	}
	var probe struct {
		AppData       string   `json:"appData"`
		SettingsPaths []string `json:"settingsPaths"`
		Guids         []string `json:"wslProfileGuids"`
	}
	if err := json.Unmarshal(data, &probe); err != nil || probe.AppData == "" {
		t.Fatal("invalid native probe", string(data), err)
	}
	linux, err := WindowsPath(context.Background(), p, "-u", probe.AppData)
	if err != nil {
		t.Fatal(err)
	}
	windows, err := WindowsPath(context.Background(), p, "-w", linux)
	if err != nil || !strings.EqualFold(windows, probe.AppData) {
		t.Fatal("path round trip", windows, err)
	}
	_, guid, _, err := (Setup{Ctx: context.Background(), Process: p, Distro: distro}).findProfile(probe.SettingsPaths, probe.Guids)
	if err != nil {
		t.Skipf("native settings have no unique visible target: %v", err)
	}
	if !validTerminalGUID(guid) {
		t.Fatal("invalid native profile identity")
	}
}
