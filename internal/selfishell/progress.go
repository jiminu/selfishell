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

type reportTone string

const (
	reportSuccess reportTone = "32"
	reportWarning reportTone = "33"
	reportPreview reportTone = "36"
	reportInfo    reportTone = ""
)

type progressEvent struct {
	section, message string
	tone             reportTone
}

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

// statusText colors the outcome label while leaving details in the default color.
func statusText(message string, color bool, tone reportTone) string {
	if !color || tone == reportInfo {
		return message
	}
	label, detail, found := strings.Cut(message, ":")
	if found {
		return "\x1b[" + string(tone) + "m" + label + ":\x1b[0m" + detail
	}
	return "\x1b[" + string(tone) + "m" + message + "\x1b[0m"
}

func (c CLI) report(section string, tone reportTone, format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	if c.progress != nil && c.progress.compact {
		c.progress.events = append(c.progress.events, progressEvent{section, message, tone})
		return
	}
	fmt.Fprintln(c.Out, statusText(message, progressColor(c.Out), tone))
}

func (c CLI) complete(message string) {
	if c.progress != nil && c.progress.compact {
		c.progress.completion = message
		return
	}
	fmt.Fprintln(c.Out, statusText(message, progressColor(c.Out), reportSuccess))
}

func (o *PackageOperation) report(tone reportTone, format string, args ...any) {
	if o.Process.progress != nil && o.Process.progress.compact {
		o.Process.progress.events = append(o.Process.progress.events, progressEvent{"Tools", fmt.Sprintf(format, args...), tone})
		return
	}
	color, _ := o.color(o.Process.Out, "\x1b[32m")
	fmt.Fprintln(o.Process.Out, statusText(fmt.Sprintf(format, args...), color != "", tone))
}

func (p *operationProgress) finish() {
	if p == nil {
		return
	}
	p.pause()
	if p.compact {
		if p.completion != "" {
			fmt.Fprintln(p.out, statusText("✓ "+p.completion, p.color, reportSuccess))
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
