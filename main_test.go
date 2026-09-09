package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

var testReport = report{version: "0.7.0", station: "w0zro@station", platform: "darwin/arm64", clock: "08-Sep-2026  23:58:41 Z"}

// The screen is eighty by twenty-four, every row framed to the same
// width, and it reports the station and calls hello, in capitals.
func TestScreenIsEightyByTwentyFour(t *testing.T) {
	rows := screen(testReport)
	if len(rows) != screenRows {
		t.Errorf("screen is %d rows, not %d:\n%s", len(rows), screenRows, strings.Join(rows, "\n"))
	}
	for i, l := range rows {
		if w := utf8.RuneCountInString(l); w != screenCols {
			t.Errorf("row %d is %d columns: %q", i, w, l)
		}
		if !strings.HasPrefix(l, "║") && !strings.HasPrefix(l, "╔") && !strings.HasPrefix(l, "╠") && !strings.HasPrefix(l, "╚") {
			t.Errorf("row %d is not framed: %q", i, l)
		}
	}
	text := strings.Join(rows, "\n")
	for _, s := range []string{tagline, "CONN VERSION 0.7.0", "STATION   W0ZRO@STATION", "PLATFORM  DARWIN/ARM64", "08-SEP-2026  23:58:41 Z", "***  HELLO FROM THE CONN  ***"} {
		if !strings.Contains(text, s) {
			t.Errorf("screen lacks %q:\n%s", s, text)
		}
	}
	if strings.Contains(text, "\x1b") {
		t.Errorf("screen carries escapes off a terminal:\n%q", text)
	}
}

// A larger terminal centers the screen; a smaller one gets it flush with
// the top left corner.
func TestScreenIsPlacedInTheTerminal(t *testing.T) {
	rows := screen(testReport)
	big := strings.Split(place(rows, len(rows), 120, 40), "\n")
	if len(big) != 8+screenRows {
		t.Errorf("120x40 places %d rows, not %d", len(big), 8+screenRows)
	}
	for _, l := range big[8:] {
		if !strings.HasPrefix(l, strings.Repeat(" ", 20)) || strings.HasPrefix(l, strings.Repeat(" ", 21)) {
			t.Errorf("row is not centered in 120: %q", l)
		}
	}
	small := strings.Split(place(rows, len(rows), 60, 20), "\n")
	if len(small) != screenRows || strings.HasPrefix(small[0], " ") {
		t.Errorf("60x20 does not place the screen flush: %d rows, first %q", len(small), small[0])
	}
}

// The screen comes on a row at a time, and the clock keeps time.
func TestProgramPaintsThenKeepsTime(t *testing.T) {
	m := newModel()
	m.report = testReport
	m.width, m.height = screenCols, screenRows
	for i := 0; i < screenRows; i++ {
		if got := strings.Count(m.View().Content, "\n"); got != max(i-1, 0) {
			t.Errorf("after %d rows the view has %d", i, got+1)
		}
		next, _ := m.Update(paintMsg{})
		m = next.(model)
	}
	if !strings.Contains(m.View().Content, "HELLO FROM THE CONN") {
		t.Errorf("the screen never finished:\n%s", m.View().Content)
	}
	next, cmd := m.Update(clockMsg{})
	m = next.(model)
	if m.report.clock == testReport.clock || cmd == nil {
		t.Errorf("the clock did not turn: %q", m.report.clock)
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
