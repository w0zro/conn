package main

import (
	"os"
	"runtime/debug"
	"strings"
)

// version is stamped by the release build. A build that came another way
// answers from the module system instead, which go install fills with the
// tag and a plain go build leaves as (devel).
var version string

// buildVersion is the version this build reports, bare: 0.7.0 for a
// release; (devel), or a tag with commits and a dirty mark after it, for a
// build that is not one.
func buildVersion() string {
	v := version
	if v == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			v = info.Main.Version
		}
	}
	if v == "" {
		v = "unknown"
	}
	return strings.TrimPrefix(v, "v")
}

// The banner is the cover of the manual, set for the terminal: the
// memorandum's header with the revision in effect, the name in heavy
// letters, the red band with what conn is for, the distribution statement,
// and then the call.
const (
	bannerWidth  = 68
	bannerIndent = "  "
	memorandum   = "TECHNICAL MEMORANDUM · TM-CONN-01"
	band         = "EVERY PROJECT AND ITS PROCESSES, ON ONE CONSOLE"
	distribution = "DISTRIBUTION STATEMENT A · APPROVED FOR PUBLIC RELEASE · MIT"
)

// The palette is the site's: paper, the red of the band, the green of the
// stamp, and the grey of the small print. The ink is the terminal's own
// foreground, bold, so the letters read on a dark ground and a light one.
type palette struct {
	ink, paper, red, green, grey, reset string
}

var (
	colored = palette{
		ink:   "\x1b[1m",
		paper: "\x1b[38;2;241;235;222m",
		red:   "\x1b[48;2;189;58;29m",
		green: "\x1b[38;2;46;125;79m",
		grey:  "\x1b[38;2;141;132;116m",
		reset: "\x1b[0m",
	}
	plain palette
)

// banner renders the cover. It is colored when asked; the caller decides
// from where the output is going.
func banner(p palette) string {
	rev := "REV " + buildVersion()
	stamp := "IN EFFECT"
	gap := bannerWidth - len([]rune(memorandum)) - len([]rune(rev)) - len(stamp) - 3
	if gap < 1 {
		gap = 1
	}
	rule := p.grey + strings.Repeat("─", bannerWidth) + p.reset

	var b strings.Builder
	line := func(s string) {
		if s != "" {
			b.WriteString(bannerIndent)
			b.WriteString(s)
		}
		b.WriteByte('\n')
	}
	center := func(s string, width int) string {
		return strings.Repeat(" ", (bannerWidth-width)/2) + s
	}
	line("")
	line(p.grey + memorandum + p.reset + strings.Repeat(" ", gap) +
		p.ink + rev + p.reset + " " + p.green + "·" + p.reset + " " + p.green + stamp + p.reset)
	line(rule)
	line("")
	for _, row := range letters(name) {
		line(center(p.ink+row+p.reset, len([]rune(row))))
	}
	line("")
	line(center(p.red+p.paper+p.ink+" "+band+" "+p.reset, len([]rune(band))+2))
	line("")
	line(p.grey + distribution + p.reset)
	line(rule)
	line(greeting)
	line("")
	return b.String()
}

// stdoutIsTerminal says whether what conn prints is going to a person's
// screen, where color belongs, or to a pipe or file, where it does not.
// NO_COLOR, set to anything, asks for none either way.
func stdoutIsTerminal() bool {
	if _, set := os.LookupEnv("NO_COLOR"); set {
		return false
	}
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
