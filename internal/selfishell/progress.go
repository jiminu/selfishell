package selfishell

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jiminu/selfishell/internal/managed"
	"github.com/jiminu/selfishell/internal/process"
	"github.com/jiminu/selfishell/internal/report"
)

// operationProgress owns terminal animation and completed action summaries.
type operationProgress struct {
	out, stderr    io.Writer
	compact, color bool
	stop, done     chan struct{}
	label          string
	events         []progressEvent
	completion     string
	config, home   string
	suspendSignals func() func()
}

type progressEvent struct {
	section, message string
	tone             report.Tone
}

func newProgress(out, stderr io.Writer, paths managed.Paths, env process.Environment) *operationProgress {
	return &operationProgress{out: out, stderr: stderr,
		compact: IsTerminal(out) && IsTerminal(stderr) && env.Get("TERM") != "dumb" && !progressCI(env),
		color:   progressColor(out, env),
		config:  paths.Config, home: env.Get("HOME")}
}

func progressCI(env process.Environment) bool { return env.Get("CI") != "" && env.Get("CI") != "false" }
func progressColor(out io.Writer, env process.Environment) bool {
	return IsTerminal(out) && env.Get("NO_COLOR") == "" && env.Get("TERM") != "dumb" && !progressCI(env)
}

// progressOf returns the spinner a Process carries, or nil. Compare its result,
// not the field, with nil: a nil spinner is still a non-nil interface value.
func progressOf(p process.Process) *operationProgress {
	progress, _ := p.Progress.(*operationProgress)
	return progress
}

// Compact implements process.Progress.
func (p *operationProgress) Compact() bool { return p != nil && p.compact }

// Streams implements process.Progress.
func (p *operationProgress) Streams() (io.Writer, io.Writer) { return p.out, p.stderr }

// Resume implements process.Progress: it restarts the last stage.
func (p *operationProgress) Resume() {
	if p.stop == nil && p.label != "" {
		p.stage(p.label)
	}
}

func (p *operationProgress) prompt() func() {
	p.Pause()
	if p != nil && p.suspendSignals != nil {
		return p.suspendSignals()
	}
	return func() {}
}

// Pause stops the animation and clears its line; it implements process.Progress.
func (p *operationProgress) Pause() {
	if p == nil || p.stop == nil {
		return
	}
	close(p.stop)
	<-p.done
	p.stop = nil
	fmt.Fprint(p.out, "\r\x1b[2K")
}

func (p *operationProgress) stage(label string) {
	if p == nil {
		return
	}
	p.Pause()
	p.label = label
	if !p.compact {
		fmt.Fprintln(p.out, label+"…")
		return
	}
	stop, done := make(chan struct{}), make(chan struct{})
	p.stop, p.done = stop, done
	fmt.Fprintf(p.out, "⠋ %s…", label)
	go func() {
		defer close(done)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		frames := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
		for n := 1; ; n++ {
			select {
			case <-stop:
				return
			case <-ticker.C:
				fmt.Fprintf(p.out, "\r\x1b[2K%c %s…", frames[n%len(frames)], label)
			}
		}
	}()
}

// statusText colors the outcome label while leaving details in the default color.
func statusText(message string, color bool, tone report.Tone) string {
	if !color || tone == report.Info {
		return message
	}
	label, detail, found := strings.Cut(message, ":")
	if found {
		return "\x1b[" + string(tone) + "m" + label + ":\x1b[0m" + detail
	}
	return "\x1b[" + string(tone) + "m" + message + "\x1b[0m"
}

func (c CLI) report(section string, tone report.Tone, format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	if c.progress != nil && c.progress.compact {
		c.progress.events = append(c.progress.events, progressEvent{section, message, tone})
		return
	}
	fmt.Fprintln(c.Out, statusText(message, progressColor(c.Out, c.Env), tone))
}

func (c CLI) complete(message string) {
	if c.progress != nil && c.progress.compact {
		c.progress.completion = message
		return
	}
	fmt.Fprintln(c.Out, statusText(message, progressColor(c.Out, c.Env), report.Success))
}

func (o *PackageOperation) report(tone report.Tone, format string, args ...any) {
	o.reportIn("Tools", tone, format, args...)
}

func (o *PackageOperation) note(format string, args ...any) {
	o.reportIn("Notes", report.Info, format, args...)
}

func (o *PackageOperation) reportIn(section string, tone report.Tone, format string, args ...any) {
	if progressOf(o.Process) != nil && progressOf(o.Process).compact {
		progressOf(o.Process).events = append(progressOf(o.Process).events, progressEvent{section, fmt.Sprintf(format, args...), tone})
		return
	}
	color, _ := o.color(o.Process.Out, "\x1b[32m")
	fmt.Fprintln(o.Process.Out, statusText(fmt.Sprintf(format, args...), color != "", tone))
}

func (p *operationProgress) finish() {
	if p == nil {
		return
	}
	p.Pause()
	if p.compact {
		if p.completion != "" {
			fmt.Fprintln(p.out, statusText("✓ "+p.completion, p.color, report.Success))
		} else if len(p.events) > 0 {
			fmt.Fprintln(p.out, "Completed changes before stopping:")
		}
		for _, section := range []string{"Tools", "Configuration", "Notes"} {
			shown := false
			for _, event := range p.events {
				if event.section != section {
					continue
				}
				if !shown {
					fmt.Fprintln(p.out, "\n"+section)
					shown = true
				}
				message := event.message
				if p.config != "" {
					message = strings.ReplaceAll(message, p.config+"/", "")
				}
				message = displayHome(message, p.home)
				fmt.Fprintln(p.out, "  "+strings.ReplaceAll(statusText(message, p.color, event.tone), "\n", "\n  "))
			}
		}
	}
}

// wrapWords breaks only between items, retaining each exact tool/version pin.
func wrapWords(words []string, width int) string {
	var out strings.Builder
	column := 0
	for _, word := range words {
		if column > 0 {
			if column+2+len(word) > width {
				out.WriteByte('\n')
				column = 0
			} else {
				out.WriteString("  ")
				column += 2
			}
		}
		out.WriteString(word)
		column += len(word)
	}
	return out.String()
}
