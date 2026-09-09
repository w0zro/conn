package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// The cover ends with the call, fits an eighty-column terminal with its
// indent, and carries no color when none was asked for.
func TestBannerIsTheCoverAndTheCall(t *testing.T) {
	b := banner(plain)
	if !strings.HasSuffix(strings.TrimRight(b, "\n"), greeting) {
		t.Errorf("banner does not end with %q:\n%s", greeting, b)
	}
	if strings.Contains(b, "\x1b") {
		t.Errorf("plain banner carries escapes:\n%q", b)
	}
	for _, l := range strings.Split(b, "\n") {
		if w := utf8.RuneCountInString(l); w > 80 {
			t.Errorf("line is %d columns: %q", w, l)
		}
		if l != strings.TrimRight(l, " ") && !strings.Contains(l, band) {
			t.Errorf("line has trailing space: %q", l)
		}
	}
}

// The colored cover says the same thing as the plain one, in color.
func TestColoredBannerReadsThePlainOne(t *testing.T) {
	c := banner(colored)
	for _, s := range []string{memorandum, band, distribution, greeting, "REV " + buildVersion()} {
		if !strings.Contains(c, s) {
			t.Errorf("colored banner lacks %q", s)
		}
	}
	if !strings.HasSuffix(strings.TrimRight(c, "\n"), greeting) {
		t.Errorf("colored banner does not end with the call")
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
	rows := letters(name)
	if len(rows) != 6 {
		t.Fatalf("%q sets to %d rows, not 6", name, len(rows))
	}
	for i, row := range rows {
		if w := utf8.RuneCountInString(row); w != utf8.RuneCountInString(rows[0]) {
			t.Errorf("row %d is %d wide, row 0 is %d", i, w, utf8.RuneCountInString(rows[0]))
		}
	}
}
