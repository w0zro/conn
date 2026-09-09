package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

var testReport = report{
	version: "0.7.0",
	build:   "3d7c5de  09-Sep-2026",
	station: "w0zro@station",
	term:    "xterm-256color  truecolor",
	clock:   "08-Sep-2026  23:58:41 Z",
	facts: []item{
		{label: "HOST", value: "station"},
		{label: "SYSTEM", value: "macOS 26.0"},
		{label: "MEMORY", value: "18 GB"},
	},
	checks: []item{
		{label: "TMUX", value: "3.5a", status: nominal},
		{label: "DISK", value: "412 GB FREE OF 994 GB", status: nominal},
	},
}

// The screen fills the terminal it is given, every row framed to its
// width, and reads out the machine, the checks and the call in capitals.
func TestScreenFillsTheTerminal(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {132, 43}, {200, 60}} {
		rows := screen(testReport, size[0], size[1])
		if len(rows) != size[1] {
			t.Errorf("%dx%d: screen is %d rows", size[0], size[1], len(rows))
		}
		for i, l := range rows {
			if w := utf8.RuneCountInString(l); w != size[0] {
				t.Errorf("%dx%d: row %d is %d columns: %q", size[0], size[1], i, w, l)
			}
			if !strings.ContainsAny(l[:3], "║╔╠╚") {
				t.Errorf("row %d is not framed: %q", i, l)
			}
		}
	}
	text := strings.Join(screen(testReport, 132, 43), "\n")
	for _, s := range []string{
		"CONN 0.7.0", "3d7c5de  09-Sep-2026", "STATION  W0ZRO@STATION", "08-Sep-2026  23:58:41 Z",
		"SYSTEM", "HOST ........ STATION", "MEMORY ...... 18 GB",
		"START-UP CHECKS", "TERMINAL .... XTERM-256COLOR  TRUECOLOR  132 X 43", "NOMINAL",
		"TMUX ........ 3.5A", "ALL SYSTEMS NOMINAL", "***  HELLO FROM THE CONN  ***",
	} {
		if !strings.Contains(text, s) {
			t.Errorf("screen lacks %q:\n%s", s, text)
		}
	}
	if strings.Contains(text, "\x1b") {
		t.Errorf("screen carries escapes off a terminal:\n%q", text)
	}
	if strings.Contains(text, "CONSOLE") {
		t.Errorf("the tagline is back:\n%s", text)
	}
}

// In color, every row is painted edge to edge on the desk and ends with
// the terminal's own colors back; the words are the plain screen's.
func TestColoredScreenPaintsEveryRow(t *testing.T) {
	pal = colored()
	defer func() { pal = plain }()
	for _, size := range [][2]int{{80, 24}, {132, 43}} {
		for i, l := range screen(testReport, size[0], size[1]) {
			if !strings.HasPrefix(l, pal.normal) || !strings.HasSuffix(l, pal.end) {
				t.Errorf("%dx%d row %d is not painted from the desk to the end: %q", size[0], size[1], i, l)
			}
			if w := utf8.RuneCountInString(stripEscapes(l)); w != size[0] {
				t.Errorf("%dx%d row %d paints %d columns: %q", size[0], size[1], i, w, stripEscapes(l))
			}
		}
	}
	text := stripEscapes(strings.Join(screen(testReport, 132, 43), "\n"))
	for _, s := range []string{"CONN 0.7.0", "TMUX ........ 3.5A", "NOMINAL", "ALL SYSTEMS NOMINAL", "***  HELLO FROM THE CONN  ***"} {
		if !strings.Contains(text, s) {
			t.Errorf("colored screen lacks %q:\n%s", s, text)
		}
	}
}

// stripEscapes drops the color sequences, leaving the cells.
func stripEscapes(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// A body taller than the terminal is cut above the footer, which stays.
func TestScreenKeepsTheFooterWhenShort(t *testing.T) {
	rows := screen(testReport, 80, 24)
	last := strings.Join(rows[len(rows)-footerRows:], "\n")
	if !strings.Contains(last, "HELLO FROM THE CONN") || !strings.HasPrefix(rows[len(rows)-1], "╚") {
		t.Errorf("footer is not the last %d rows:\n%s", footerRows, last)
	}
}

// A check that failed is counted, and said, at the foot.
func TestFaultsAreSummed(t *testing.T) {
	r := testReport
	r.checks = append(r.checks, item{label: "GIT", value: "NOT FOUND", status: "MISSING", fault: true})
	text := strings.Join(screen(r, 100, 40), "\n")
	if !strings.Contains(text, "1 SYSTEM NOT NOMINAL") || !strings.Contains(text, "MISSING") {
		t.Errorf("fault not summed:\n%s", text)
	}
	small := strings.Join(screen(r, 60, 20), "\n")
	if !strings.Contains(small, "SMALL") || !strings.Contains(small, "2 SYSTEMS NOT NOMINAL") {
		t.Errorf("small terminal not a fault:\n%s", small)
	}
}

// The screen comes on a row at a time, and the clock keeps time.
func TestProgramPaintsThenKeepsTime(t *testing.T) {
	m := newModel()
	m.report = testReport
	m.width, m.height = 100, 40
	for i := 0; i < 40-footerRows; i++ {
		if got := strings.Count(m.View().Content, "\n"); got != max(i-1, 0) {
			t.Errorf("after %d rows the view has %d", i, got+1)
		}
		next, _ := m.Update(paintMsg{})
		m = next.(model)
	}
	if v := m.View().Content; !strings.Contains(v, "HELLO FROM THE CONN") || strings.Count(v, "\n") != 39 {
		t.Errorf("the screen did not finish:\n%s", v)
	}
	next, cmd := m.Update(clockMsg{})
	m = next.(model)
	if m.report.clock == testReport.clock || cmd == nil {
		t.Errorf("the clock did not turn: %q", m.report.clock)
	}
}

// The machine can be read without a fuss, and what it says is in shape.
func TestStationReportReadsTheMachine(t *testing.T) {
	r := stationReport()
	if r.version == "" || r.station == "" || r.clock == "" {
		t.Errorf("identification incomplete: %+v", r)
	}
	if len(r.facts) < 5 {
		t.Errorf("only %d facts: %+v", len(r.facts), r.facts)
	}
	for _, c := range r.checks {
		if c.label == "" || c.value == "" || c.status == "" {
			t.Errorf("check incomplete: %+v", c)
		}
	}
	if gigabytes(18<<30, 1<<30) != "18 GB" || gigabytes(994_662_584_320, 1e9) != "995 GB" {
		t.Errorf("sizes: %q %q", gigabytes(18<<30, 1<<30), gigabytes(994_662_584_320, 1e9))
	}
	if firstVersion("tmux 3.5a") != "3.5a" || firstVersion("git version 2.50.1") != "2.50.1" {
		t.Errorf("versions: %q %q", firstVersion("tmux 3.5a"), firstVersion("git version 2.50.1"))
	}
}

// The name is set from its own letters, every letter the same height,
// and the word to rows of one width.
func TestLettersSetEvenly(t *testing.T) {
	for r, g := range glyphs {
		if len(g) != 7 {
			t.Errorf("%c is %d rows tall, not 7", r, len(g))
		}
		for i, row := range g {
			if len(row) != len(g[0]) {
				t.Errorf("%c row %d is %d wide, row 0 is %d", r, i, len(row), len(g[0]))
			}
		}
	}
	rows := letters(nameSet)
	if len(rows) != 7 {
		t.Fatalf("%q sets to %d rows, not 7", nameSet, len(rows))
	}
	for i, row := range rows {
		if w := utf8.RuneCountInString(row); w != utf8.RuneCountInString(rows[0]) {
			t.Errorf("row %d is %d wide, row 0 is %d", i, w, utf8.RuneCountInString(rows[0]))
		}
		if strings.Trim(row, "CON ") != "" {
			t.Errorf("row %d is drawn in something other than the letters: %q", i, row)
		}
	}
}
