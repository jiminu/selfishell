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
