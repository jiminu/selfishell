package benchmark

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/jiminu/selfishell/internal/pty"
)

// Stats preserves millisecond precision; callers round only for display.
type Stats struct{ Mean, P50, P95, Max float64 }

func Summarize(samples []float64) (Stats, error) {
	if len(samples) == 0 {
		return Stats{}, errors.New("no samples")
	}
	ordered := append([]float64(nil), samples...)
	sort.Float64s(ordered)
	sum := 0.0
	for _, value := range samples {
		sum += value
	}
	rank := func(q float64) float64 { return ordered[int(math.Ceil(float64(len(ordered))*q))-1] }
	return Stats{Mean: sum / float64(len(samples)), P50: rank(.5), P95: rank(.95), Max: ordered[len(ordered)-1]}, nil
}

// PromptSamples contains raw milliseconds from one warmup plus iterations measured shells.
type PromptSamples struct{ First, Command []float64 }

// PromptEnvironment builds the child environment without inheriting caller tools or activation.
func PromptEnvironment(root, home, zdotdir, platformConfig, path string, inherited []string) []string {
	lang := "en_US.UTF-8"
	wsl := ""
	for _, pair := range inherited {
		if v, ok := strings.CutPrefix(pair, "LANG="); ok {
			lang = v
		}
		if v, ok := strings.CutPrefix(pair, "WSL_DISTRO_NAME="); ok {
			wsl = v
		}
	}
	env := []string{
		"HOME=" + home, "ZDOTDIR=" + zdotdir, "LANG=" + lang,
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "XDG_DATA_HOME=" + filepath.Join(home, ".local/share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local/state"), "XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"MISE_DATA_DIR=" + filepath.Join(home, ".local/share/mise"), "MISE_CACHE_DIR=" + filepath.Join(home, ".cache/mise"),
		"MISE_STATE_DIR=" + filepath.Join(home, ".local/state/mise"), "MISE_GLOBAL_CONFIG_FILE=" + filepath.Join(home, ".config/mise/config.toml"),
		"MISE_OFFLINE=1", "STARSHIP_CONFIG=" + filepath.Join(root, "config/shared/starship.toml"),
		"SELFISHELL_UPDATE_NOTICE=0", "SELFISHELL_BENCHMARK_PLATFORM_CONFIG=" + platformConfig,
		"PATH=" + path, "SHELL=/bin/zsh", "TERM=xterm-256color",
	}
	if wsl != "" {
		env = append(env, "WSL_DISTRO_NAME="+wsl)
	}
	return env
}

// MeasurePrompt measures one scenario. cwd must be absolute; env is the private child environment.
func MeasurePrompt(ctx context.Context, cwd string, env []string, iterations int) (PromptSamples, error) {
	if iterations < 1 {
		return PromptSamples{}, errors.New("iterations must be positive")
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return PromptSamples{}, err
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return PromptSamples{}, err
	}
	env = append(append([]string(nil), env...), "MISE_CEILING_PATHS="+abs)
	samples := PromptSamples{First: make([]float64, 0, iterations), Command: make([]float64, 0, iterations*3)}
	for iteration := 0; iteration <= iterations; iteration++ {
		first, commands, err := measureShell(ctx, abs, env)
		if err != nil {
			return PromptSamples{}, fmt.Errorf("prompt shell %d: %w", iteration, err)
		}
		if iteration > 0 {
			samples.First = append(samples.First, first)
			samples.Command = append(samples.Command, commands[:]...)
		}
	}
	return samples, nil
}

type readResult struct {
	data []byte
	err  error
}

func measureShell(parent context.Context, cwd string, env []string) (float64, [3]float64, error) {
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	master, slave, err := pty.Open()
	if err != nil {
		return 0, [3]float64{}, err
	}
	defer master.Close()
	if err := pty.Resize(slave, 32, 160); err != nil {
		slave.Close()
		return 0, [3]float64{}, err
	}
	cmd := exec.Command("/bin/zsh", "-d", "-i")
	cmd.Dir = cwd
	cmd.Env = env
	cmd.Stdin = slave
	cmd.Stdout = slave
	cmd.Stderr = slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	start := time.Now()
	if err := cmd.Start(); err != nil {
		slave.Close()
		return 0, [3]float64{}, err
	}
	slave.Close()
	read := make(chan readResult, 8)
	go func() {
		defer close(read)
		buf := make([]byte, 4096)
		for {
			n, e := master.Read(buf)
			if n > 0 {
				b := append([]byte(nil), buf[:n]...)
				select {
				case read <- readResult{data: b}:
				case <-ctx.Done():
					return
				}
			}
			if e != nil {
				select {
				case read <- readResult{err: e}:
				case <-ctx.Done():
				}
				return
			}
		}
	}()
	waited := false
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	defer func() {
		// A shell may have left children in its session; terminate the whole group.
		if !waited {
			_ = syscall.Kill(cmd.Process.Pid, syscall.SIGHUP)
			select {
			case <-time.After(100 * time.Millisecond):
			case <-wait:
				waited = true
			}
		}
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if !waited {
			_ = <-wait
		}
	}()
	if err := waitForPrompt(ctx, read, 1); err != nil {
		select {
		case childErr := <-wait:
			waited = true
			return 0, [3]float64{}, fmt.Errorf("%w; shell exit: %v", err, childErr)
		default:
			return 0, [3]float64{}, err
		}
	}
	first := float64(time.Since(start)) / float64(time.Millisecond)
	settle := time.NewTimer(100 * time.Millisecond)
	select {
	case <-settle.C:
	case <-ctx.Done():
		settle.Stop()
		return 0, [3]float64{}, ctx.Err()
	}
	var commands [3]float64
	for count := 2; count <= 4; count++ {
		start = time.Now()
		if _, err := master.Write([]byte(":\n")); err != nil {
			return 0, [3]float64{}, fmt.Errorf("command %d write: %w", count, err)
		}
		if err := waitForPrompt(ctx, read, count); err != nil {
			return 0, [3]float64{}, err
		}
		commands[count-2] = float64(time.Since(start)) / float64(time.Millisecond)
	}
	if _, err := master.Write([]byte("exit\n")); err != nil {
		return 0, [3]float64{}, fmt.Errorf("exit write: %w", err)
	}
	exitTimer := time.NewTimer(5 * time.Second)
	defer exitTimer.Stop()
	select {
	case err := <-wait:
		waited = true
		if err != nil {
			return 0, [3]float64{}, fmt.Errorf("shell exit: %w", err)
		}
	case <-exitTimer.C:
		return 0, [3]float64{}, errors.New("shell did not exit after measurement")
	case <-ctx.Done():
		return 0, [3]float64{}, ctx.Err()
	}
	return first, commands, nil
}

func waitForPrompt(ctx context.Context, read <-chan readResult, count int) error {
	marker := fmt.Sprintf("__SFS_READY_%d__", count)
	tail := ""
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("prompt %d timed out or canceled: %w; tail %q", count, ctx.Err(), tail)
		case result, ok := <-read:
			if !ok {
				return fmt.Errorf("shell exited before prompt %d; tail %q", count, tail)
			}
			tail += string(result.data)
			if strings.Contains(tail, marker) {
				return nil
			}
			if len(tail) > 2000 {
				tail = tail[len(tail)-2000:]
			}
			if result.err != nil {
				if errors.Is(result.err, io.EOF) || errors.Is(result.err, syscall.EIO) {
					return fmt.Errorf("shell exited before prompt %d: %w; tail %q", count, result.err, tail)
				}
				return fmt.Errorf("prompt %d read: %w; tail %q", count, result.err, tail)
			}
		}
	}
}
