package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

var testReport = report{version: "0.7.0", station: "w0zro@station", platform: "darwin/arm64", clock: "2026-09-08 23:58:41Z"}

// The plain screen fits the width it was asked for, carries no escapes and
// no trailing space, reports the station, and calls hello.
func TestPlainScreenReportsAndCalls(t *testing.T) {
	rows := screen(plain, testReport, 80)
	text := strings.Join(rows, "\n")
	for _, s := range []string{tagline, "VERSION   0.7.0", "STATION   w0zro@station", "PLATFORM  darwin/arm64", "CLOCK     2026-09-08 23:58:41Z", greeting} {
		if !strings.Contains(text, s) {
			t.Errorf("screen lacks %q:\n%s", s, text)
		}
	}
	if !strings.HasSuffix(strings.TrimSpace(text), greeting) {
		t.Errorf("screen does not end with the call:\n%s", text)
	}
	for _, l := range rows {
		if strings.Contains(l, "\x1b") {
			t.Errorf("plain row carries escapes: %q", l)
		}
		if w := utf8.RuneCountInString(l); w > 80 {
			t.Errorf("row is %d columns: %q", w, l)
		}
		if l != strings.TrimRight(l, " ") {
			t.Errorf("row has trailing space: %q", l)
		}
	}
}

// The painted screen covers every column of every row, so the panel has
// straight edges, and says what the plain one says.
func TestPaintedScreenCoversTheWidth(t *testing.T) {
	for _, width := range []int{60, 80, 132} {
		for i, l := range screen(colored, testReport, width) {
			if !strings.HasSuffix(l, colored.reset) {
				t.Errorf("width %d row %d does not reset: %q", width, i, l)
			}
			if w := utf8.RuneCountInString(stripEscapes(l)); w != width {
				t.Errorf("width %d row %d paints %d columns: %q", width, i, w, l)
			}
		}
	}
	text := strings.Join(screen(colored, testReport, 80), "\n")
	if !strings.Contains(text, greeting) || !strings.Contains(text, "0.7.0") {
		t.Errorf("painted screen lost its words:\n%s", text)
	}
}

// A terminal too narrow for the letters gets the name spelled out instead.
func TestNarrowScreenSpellsTheName(t *testing.T) {
	text := strings.Join(screen(plain, testReport, 50), "\n")
	if !strings.Contains(text, "C O N N") {
		t.Errorf("narrow screen does not spell the name:\n%s", text)
	}
	for _, l := range strings.Split(text, "\n") {
		if w := utf8.RuneCountInString(l); w > 50 {
			t.Errorf("row is %d columns: %q", w, l)
		}
	}
}

// Every letter is the same height, and the word sets to rows of one width.
func TestLettersSetEvenly(t *testing.T) {
	for r, g := range glyphs {
		if len(g) != 12 {
			t.Errorf("%c is %d pixels tall, not 12", r, len(g))
		}
		for i, row := range g {
			if len(row) != len(g[0]) {
				t.Errorf("%c row %d is %d wide, row 0 is %d", r, i, len(row), len(g[0]))
			}
		}
	}
	rows := letters(nameSet)
	if len(rows) != 6 {
		t.Fatalf("%q sets to %d rows, not 6", nameSet, len(rows))
	}
	for i, row := range rows {
		if w := utf8.RuneCountInString(row); w != utf8.RuneCountInString(rows[0]) {
			t.Errorf("row %d is %d wide, row 0 is %d", i, w, utf8.RuneCountInString(rows[0]))
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
