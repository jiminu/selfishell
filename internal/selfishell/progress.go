package selfishell

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// operationProgress owns terminal animation, successful actions and one private
// subprocess log for an install/update. Dry runs never create one.
type operationProgress struct {
	out, stderr    io.Writer
	compact, color bool
	stop, done     chan struct{}
	label          string
	events         []progressEvent
	completion     string
	logDir         string
	log            *os.File
	logFailed      bool
	config, home   string
	suspendSignals func() func()
}

type progressEvent struct{ section, message string }

func newProgress(out, stderr io.Writer, paths Paths) *operationProgress {
	return &operationProgress{out: out, stderr: stderr,
		compact: IsTerminal(out) && IsTerminal(stderr) && os.Getenv("TERM") != "dumb" && !progressCI(),
		color:   progressColor(out),
		logDir:  filepath.Join(paths.State, "logs"), config: paths.Config, home: os.Getenv("HOME")}
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
	if !p.compact || p.logFailed {
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
	if p.log != nil {
		name := p.log.Name()
		p.log.Close()
		fmt.Fprintln(p.out, "\nLog: "+name)
	}
}

func (p *operationProgress) rememberOutput(output string) {
	if p == nil || !p.compact || output == "" {
		return
	}
	if file, _ := p.capture(); file != nil {
		fmt.Fprint(file, output)
	} else {
		p.pause()
		fmt.Fprint(p.stderr, output)
	}
}

// capture only applies to inherited display streams. Queries with buffers or
// discarded output must retain their original destinations and semantics.
func (p *operationProgress) capture() (*os.File, int64) {
	if p.logFailed {
		return nil, 0
	}
	if p.log == nil {
		err := os.MkdirAll(p.logDir, 0700)
		if err == nil {
			p.log, err = os.CreateTemp(p.logDir, "operation-*.log")
		}
		if err != nil {
			p.pause()
			fmt.Fprintln(p.stderr, "selfishell: warning: Could not open operation log; showing tool output.")
			p.logFailed = true
			return nil, 0
		}
	}
	offset, err := p.log.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, 0
	}
	fmt.Fprintln(p.log, "\n"+p.label)
	return p.log, offset
}

func (p *operationProgress) failedOutput(offset int64) {
	p.pause()
	info, err := p.log.Stat()
	if err != nil {
		return
	}
	if info.Size()-offset > 8192 {
		offset = info.Size() - 8192
	}
	_, _ = io.Copy(p.stderr, io.NewSectionReader(p.log, offset, info.Size()-offset))
}
