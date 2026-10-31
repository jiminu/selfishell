package report

// Tone is the outcome color of a report line, as an ANSI SGR code.
type Tone string

// Report tones.
const (
	Success Tone = "32"
	Warning Tone = "33"
	Preview Tone = "36"
	Info    Tone = ""
)

// Reporter receives a subsystem's report lines under a section heading; the
// core renders them, or queues them behind its progress spinner.
type Reporter interface {
	Report(section string, tone Tone, format string, args ...any)
}

// ReportFunc adapts a function, such as a bound core method, to Reporter.
type ReportFunc func(section string, tone Tone, format string, args ...any)

// Report calls f.
func (f ReportFunc) Report(section string, tone Tone, format string, args ...any) {
	f(section, tone, format, args...)
}
