package main

import (
	"os"
	"os/user"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
	"unsafe"
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

// The start-up screen is the station coming on. A dark panel is painted
// across the terminal, the name comes up large, the station reports what
// it is running and where, and then conn is on the loop and calls hello.

// tagline is what conn is for, in the manual's words.
const tagline = "every project and its processes, on one console"

// The palette is the site's: the dark of the ground, paper for the text,
// grey for the small print, red for the bar along the top and the rule
// along the bottom, and green for the light that says conn is on the loop.
type palette struct {
	ground, paper, grey, red, redInk, green, reset string
}

var (
	colored = palette{
		ground: "\x1b[48;2;25;27;31m",
		paper:  "\x1b[38;2;241;235;222m",
		grey:   "\x1b[38;2;141;132;116m",
		red:    "\x1b[48;2;189;58;29m",
		redInk: "\x1b[38;2;189;58;29m",
		green:  "\x1b[38;2;46;125;79m",
		reset:  "\x1b[0m",
	}
	plain palette
)

// A report is what the station says of itself: the build, who is on it
// and where, the platform, and the clock in Zulu.
type report struct {
	version, station, platform, clock string
}

// stationReport reads the report from the machine.
func stationReport() report {
	who := "someone"
	if u, err := user.Current(); err == nil && u.Username != "" {
		who = u.Username
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "somewhere"
	}
	host, _, _ = strings.Cut(host, ".")
	return report{
		version:  buildVersion(),
		station:  who + "@" + host,
		platform: runtime.GOOS + "/" + runtime.GOARCH,
		clock:    time.Now().UTC().Format("2006-01-02 15:04:05Z"),
	}
}

// screen renders the start-up screen as rows, each painted edge to edge
// when the palette has a ground, for a terminal width columns wide.
func screen(p palette, r report, width int) []string {
	name := letters(nameSet)
	block := utf8.RuneCountInString(name[0])
	if width < block+4 {
		name = []string{strings.Join(strings.Split(nameSet, ""), " ")}
		block = utf8.RuneCountInString(name[0])
	}
	if w := utf8.RuneCountInString(tagline); w > block {
		block = w
	}
	left := (width - block) / 2
	if left < 1 {
		left = 1
	}

	var rows []string
	row := func(text string, cells int) {
		pad := width - left - cells
		if pad < 0 {
			pad = 0
		}
		rows = append(rows, p.ground+p.paper+strings.Repeat(" ", left)+text+strings.Repeat(" ", pad)+p.reset)
	}
	blank := func() { row("", 0) }

	if p != plain {
		rows = append(rows, p.red+strings.Repeat(" ", width)+p.reset)
	}
	blank()
	blank()
	for _, l := range name {
		row(l, utf8.RuneCountInString(l))
	}
	blank()
	row(p.grey+tagline+p.paper, utf8.RuneCountInString(tagline))
	blank()
	blank()
	for _, f := range []struct{ label, value string }{
		{"VERSION", r.version},
		{"STATION", r.station},
		{"PLATFORM", r.platform},
		{"CLOCK", r.clock},
	} {
		label := f.label + strings.Repeat(" ", 10-len(f.label))
		row(p.grey+label+p.paper+f.value, 10+utf8.RuneCountInString(f.value))
	}
	blank()
	row(p.green+"●"+p.paper+"  "+greeting, 3+len(greeting))
	blank()
	blank()
	if p != plain {
		rows = append(rows, p.ground+p.redInk+strings.Repeat("▀", width)+p.reset)
	}

	if p == plain {
		for i := range rows {
			rows[i] = strings.TrimRight(rows[i], " ")
		}
	}
	return rows
}

// stdoutIsTerminal says whether what conn prints is going to a person's
// screen, where the paint belongs, or to a pipe or file, where it does
// not. NO_COLOR, set to anything, asks for none either way.
func stdoutIsTerminal() bool {
	if _, set := os.LookupEnv("NO_COLOR"); set {
		return false
	}
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// terminalWidth is how many columns the terminal on stdout has, or 80 when
// it will not say.
func terminalWidth() int {
	var ws struct{ rows, cols, x, y uint16 }
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, os.Stdout.Fd(), syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&ws)))
	if errno != 0 || ws.cols == 0 {
		return 80
	}
	return int(ws.cols)
}
