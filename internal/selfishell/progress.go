package selfishell

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
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

type progressEvent struct{ section, message string }

func newProgress(out, stderr io.Writer, paths Paths) *operationProgress {
	return &operationProgress{out: out, stderr: stderr,
		compact: IsTerminal(out) && IsTerminal(stderr) && os.Getenv("TERM") != "dumb" && !progressCI(),
		color:   progressColor(out),
		config:  paths.Config, home: os.Getenv("HOME")}
}

func progressCI() bool { return os.Getenv("CI") != "" && os.Getenv("CI") != "false" }
func progressColor(out io.Writer) bool {
	return IsTerminal(out) && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb" && !progressCI()
}

func (p *operationProgress) prompt() func() {
	p.pause()
	if p != nil && p.suspendSignals != nil {
		return p.suspendSignals()
	}
	return func() {}
}

func (p *operationProgress) pause() {
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
	p.pause()
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

// statusText restores the shell CLI's green actions, yellow skips/conflicts,
// and cyan previews. Only the label is colored; paths retain the default color.
func statusText(message string, color bool) string {
	if !color {
		return message
	}
	code := ""
	for _, prefix := range []string{"Installed", "Updated", "Activated", "Added", "Linked", "Backed up", "Removed", "Restored", "Created", "Set login shell", "Synchronized", "Cleaned up", "Selfishell", "✓"} {
		if strings.HasPrefix(message, prefix) {
			code = "32"
			break
		}
	}
	if strings.HasPrefix(message, "Would ") {
		code = "36"
	}
	if strings.HasPrefix(message, "Skipped ") || strings.HasPrefix(message, "Skipping ") || strings.HasPrefix(message, "Conflict:") || strings.HasPrefix(message, "Warning:") || strings.HasPrefix(message, "Could not ") {
		code = "33"
	}
	if code == "" {
		return message
	}
	label, detail, found := strings.Cut(message, ":")
	if found {
		return "\x1b[" + code + "m" + label + ":\x1b[0m" + detail
	}
	return "\x1b[" + code + "m" + message + "\x1b[0m"
}

func (c CLI) report(section, format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	if c.progress != nil && c.progress.compact {
		c.progress.events = append(c.progress.events, progressEvent{section, message})
		return
	}
	fmt.Fprintln(c.Out, statusText(message, progressColor(c.Out)))
}

func (c CLI) complete(message string) {
	if c.progress != nil && c.progress.compact {
		c.progress.completion = message
		return
	}
	fmt.Fprintln(c.Out, statusText(message, progressColor(c.Out)))
}

func (o *PackageOperation) report(format string, args ...any) {
	if o.Process.progress != nil && o.Process.progress.compact {
		o.Process.progress.events = append(o.Process.progress.events, progressEvent{"Tools", fmt.Sprintf(format, args...)})
		return
	}
	color, _ := o.color(o.Process.Out, "\x1b[32m")
	fmt.Fprintln(o.Process.Out, statusText(fmt.Sprintf(format, args...), color != ""))
}

func (p *operationProgress) finish() {
	if p == nil {
		return
	}
	p.pause()
	if p.compact {
		if p.completion != "" {
			fmt.Fprintln(p.out, statusText("✓ "+p.completion, p.color))
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
				if p.home != "" {
					message = strings.ReplaceAll(message, p.home+"/", "~/")
				}
				fmt.Fprintln(p.out, "  "+statusText(message, p.color))
			}
		}
	}
}
