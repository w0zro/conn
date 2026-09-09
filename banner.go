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

// The start-up screen is a system screen of the old kind: eighty columns
// by twenty-four rows, ruled in double lines, the name set large in the
// upper panel, the station's report in the middle one, and the call in
// the lower. It is monochrome, in whatever the terminal's phosphor is;
// on a terminal, the name and the call are bright.

const (
	screenCols = 80
	screenRows = 24
	tagline    = "EVERY PROJECT AND ITS PROCESSES, ON ONE CONSOLE"
)

// bright and normal are the one attribute the screen uses, and are empty
// off a terminal.
var bright, normal string

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
		clock:    time.Now().UTC().Format("02-Jan-2006  15:04:05") + " Z",
	}
}

// screen renders the start-up screen as rows, centered in a terminal
// width columns wide when it is wider than the screen.
func screen(r report, width int) []string {
	inner := screenCols - 2
	margin := ""
	if width > screenCols {
		margin = strings.Repeat(" ", (width-screenCols)/2)
	}

	var rows []string
	rule := func(l, m, rt string) {
		rows = append(rows, margin+l+strings.Repeat(m, inner)+rt)
	}
	// line frames text between the side rules, at a column from the left
	// rule; negative, it centers the text.
	line := func(text string, col int, attr string) {
		w := utf8.RuneCountInString(text)
		if col < 0 {
			col = (inner - w) / 2
		}
		right := inner - col - w
		if right < 0 {
			right = 0
		}
		rows = append(rows, margin+"║"+strings.Repeat(" ", col)+attr+text+normal+strings.Repeat(" ", right)+"║")
	}
	blank := func() { line("", 0, "") }

	rows = append(rows, "")
	rule("╔", "═", "╗")
	blank()
	name := letters(nameSet)
	col := (inner - utf8.RuneCountInString(name[0])) / 2
	for _, l := range name {
		line(l, col, bright)
	}
	blank()
	line(tagline, -1, "")
	blank()
	rule("╠", "═", "╣")
	blank()
	left := 4
	labels := [][2]string{
		{"CONN VERSION " + r.version, "STATION   " + strings.ToUpper(r.station)},
		{strings.ToUpper(r.clock), "PLATFORM  " + strings.ToUpper(r.platform)},
	}
	for _, l := range labels {
		gap := inner/2 - left - utf8.RuneCountInString(l[0])
		if gap < 2 {
			gap = 2
		}
		line(l[0]+strings.Repeat(" ", gap)+l[1], left, "")
	}
	blank()
	rule("╠", "═", "╣")
	blank()
	line("***  "+strings.ToUpper(greeting)+"  ***", -1, bright)
	blank()
	rule("╚", "═", "╝")
	rows = append(rows, "")
	return rows
}

// stdoutIsTerminal says whether what conn prints is going to a person's
// screen, or to a pipe or file.
func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// terminalWidth is how many columns the terminal on stdout has, or the
// screen's own when it will not say.
func terminalWidth() int {
	var ws struct{ rows, cols, x, y uint16 }
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, os.Stdout.Fd(), syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&ws)))
	if errno != 0 || ws.cols == 0 {
		return screenCols
	}
	return int(ws.cols)
}
