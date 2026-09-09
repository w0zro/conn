package main

import (
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

var update = flag.Bool("update", false, "write the golden consoles under testdata")

func texts(rows []row) string {
	var b []string
	for _, r := range rows {
		b = append(b, r.text)
	}
	return strings.Join(b, "\n")
}

// golden holds a rendering to the file of record under testdata. Run
// the tests with -update to write what the console renders now, and
// read the diff before committing it: the file is the design.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	got += "\n"
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to write it)", err)
	}
	if string(want) != got {
		t.Errorf("%s differs from the golden file; run with -update if the change is meant:\n%s", name, got)
	}
}

// The console at 120 by 40, all nominal, is the file of record.
func TestConsoleMatchesTheGolden(t *testing.T) {
	golden(t, "console-120x40.txt", texts(screen(compose(testStation, testNow), 120, 40, plain)))
}

// The console with faults, and the one off a terminal, are files of
// record too.
func TestFaultedConsolesMatchTheGolden(t *testing.T) {
	st := testStation
	st.volume.free = 6_800_000_000
	st.machine.power = power{source: "battery", percent: 7, state: "discharging", remaining: "0:31"}
	golden(t, "console-faults-100x36.txt", texts(screen(compose(st, testNow), 100, 36, plain)))
	golden(t, "console-piped.txt", texts(screen(compose(st, testNow), 0, 0, plain)))
	golden(t, "console-small-60x24.txt", texts(screen(compose(st, testNow), 60, 24, plain)))
	golden(t, "console-small-100x12.txt", texts(screen(compose(st, testNow), 100, 12, plain)))
}

// The layout holds where the eye checks it: the session column at half
// the measure, every status flush with its right edge, a path in its own
// case, no row past the width.
func TestConsoleLaysOut(t *testing.T) {
	r := compose(testStation, testNow)
	rows := screen(r, 120, 40, plain)
	measure, rightCol, _ := columns(120)
	text := texts(rows)
	for _, s := range []string{
		"CONN 0.7.0 (devel)", "STATION  W0ZRO@STATION", "09-SEP-2026  02:58:41 Z", "4af550d · 09-SEP-2026 · MODIFIED",
		"HOST ...... STATION", "CWD ....... ~/projects/w0zro/conn", "SHELL ..... ZSH 5.9",
		"SCREEN .... 120×40 · XTERM-256COLOR · TRUECOLOR", "STATE ..... ~/.local/state/conn",
		"ALL SYSTEMS NOMINAL", prompt,
	} {
		if !strings.Contains(text, s) {
			t.Errorf("console lacks %q:\n%s", s, text)
		}
	}
	if strings.Contains(text, "\x1b") || strings.Contains(text, "NOT NOMINAL") {
		t.Errorf("console carries an escape or a fault:\n%s", text)
	}
	if len(rows) != 40 || !strings.Contains(rows[39].text, prompt) {
		t.Errorf("%d rows; the last is %q", len(rows), rows[len(rows)-1].text)
	}
	for _, row := range rows {
		if strings.HasSuffix(row.text, nominal) && strings.Contains(row.text, "...") && utf8.RuneCountInString(row.text) != margin+measure {
			t.Errorf("status is not flush with column %d: %q", margin+measure, row.text)
		}
		if i := strings.Index(row.text, "USER ..."); i >= 0 && utf8.RuneCountInString(row.text[:i]) != margin+rightCol {
			t.Errorf("session column is not at %d: %q", margin+rightCol, row.text)
		}
		if w := utf8.RuneCountInString(row.text); w > 120 {
			t.Errorf("row is %d columns: %q", w, row.text)
		}
	}
	if rowsNeeded(r) != len(body(r, 120, check{}, plain))+2 {
		t.Errorf("rowsNeeded %d is not the body and two", rowsNeeded(r))
	}
}

// A fault lights the chip, flush right, and the count in the verdict.
func TestFaultsLightTheConsole(t *testing.T) {
	st := testStation
	st.volume.free = 6_800_000_000
	rows := screen(compose(st, testNow), 120, 40, plain)
	text := texts(rows)
	if !strings.Contains(text, "6.8 GB FREE OF 995 GB") || !strings.Contains(text, " LOW") || !strings.Contains(text, "1 SYSTEM NOT NOMINAL") {
		t.Errorf("fault not lit:\n%s", text)
	}
	measure, _, _ := columns(120)
	for _, row := range rows {
		// In plain text the chip's trailing space is trimmed with the row's.
		if strings.HasSuffix(row.text, " LOW") && utf8.RuneCountInString(row.text) != margin+measure-1 {
			t.Errorf("chip is not flush with column %d: %q", margin+measure, row.text)
		}
	}
	st.machine.power.percent, st.machine.power.state = 5, "discharging"
	if text := texts(screen(compose(st, testNow), 120, 40, plain)); !strings.Contains(text, "2 SYSTEMS NOT NOMINAL") {
		t.Errorf("two faults not counted:\n%s", text)
	}
}

// A terminal the console will not fit gets the small console: nothing
// clipped, the size it needs in view, the prompt when there is a row
// for it. Off a terminal there is no screen to check, and no prompt.
func TestSmallAndPipedConsoles(t *testing.T) {
	r := compose(testStation, testNow)
	need := rowsNeeded(r)
	for _, c := range []struct{ w, h int }{{60, 24}, {100, 12}, {100, need - 1}, {79, 50}, {20, 5}} {
		rows := screen(r, c.w, c.h, plain)
		text := texts(rows)
		if !strings.Contains(text, " SMALL") || !strings.Contains(text, "NEEDS 80×"+strconv.Itoa(need)) {
			t.Errorf("%dx%d is not called small:\n%s", c.w, c.h, text)
		}
		if len(rows) != c.h {
			t.Errorf("%dx%d renders %d rows", c.w, c.h, len(rows))
		}
		for _, row := range rows {
			if w := utf8.RuneCountInString(row.text); w > c.w {
				t.Errorf("%dx%d has a row %d wide: %q", c.w, c.h, w, row.text)
			}
		}
		if c.h >= 6 && !strings.Contains(rows[c.h-1].text, prompt) {
			t.Errorf("%dx%d has no prompt on the bottom row:\n%s", c.w, c.h, text)
		}
	}
	if rows := screen(r, 100, need, plain); strings.Contains(texts(rows), "SMALL") || len(rows) != need {
		t.Errorf("a terminal of exactly the rows needed is small, or %d rows", len(rows))
	}
	piped := texts(screen(r, 0, 0, plain))
	if !strings.Contains(piped, "SCREEN .... NO TERMINAL") || !strings.Contains(piped, " UNCHECKED") || strings.Contains(piped, prompt) {
		t.Errorf("off a terminal:\n%s", piped)
	}
}

// In color, every row is painted edge to edge on the ground and ends with
// the terminal's own colors back; the words are the plain console's.
func TestColoredConsolePaintsEveryRow(t *testing.T) {
	p := colored()
	r := compose(testStation, testNow)
	rows := screen(r, 120, 40, p)
	for i, row := range rows {
		if !strings.HasPrefix(row.text, p.normal) || !strings.HasSuffix(row.text, p.end) {
			t.Errorf("row %d is not painted from the ground to the end: %q", i, row.text)
		}
		if w := utf8.RuneCountInString(stripEscapes(row.text)); w != 120 {
			t.Errorf("row %d paints %d columns: %q", i, w, stripEscapes(row.text))
		}
	}
	plainText := texts(screen(r, 120, 40, plain))
	for i, row := range rows {
		if want := strings.Split(plainText, "\n")[i]; strings.TrimRight(stripEscapes(row.text), " ") != want {
			t.Errorf("row %d reads %q in color and %q plain", i, strings.TrimRight(stripEscapes(row.text), " "), want)
		}
	}
	for i, row := range screen(r, 60, 24, p) {
		if w := utf8.RuneCountInString(stripEscapes(row.text)); w != 60 {
			t.Errorf("small row %d paints %d columns", i, w)
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

// A value that will not fit is cut at its end; a path gives up its
// middle and keeps its name.
func TestPathsShortenFromTheMiddle(t *testing.T) {
	exe := "/var/folders/51/kkgwpd9j2r53trb71lnmfp6r0000gn/T/go-build3688959642/b001/exe/conn"
	for _, c := range []struct {
		in   string
		w    int
		want string
	}{
		{exe + " · 5.5 MB", 90, exe + " · 5.5 MB"},
		{exe + " · 5.5 MB", 60, "/var/…/T/go-build3688959642/b001/exe/conn · 5.5 MB"},
		{exe + " · 5.5 MB", 30, "/var/…/b001/exe/conn · 5.5 MB"},
		{exe + " · 5.5 MB", 22, "/var/…/conn · 5.5 MB"},
		{exe, 8, "…xe/conn"},
		{"~/projects/w0zro/conn/conn", 20, "~/…/w0zro/conn/conn"},
		{"~/projects/w0zro/conn", 40, "~/projects/w0zro/conn"},
		{"APPLE M3 PRO · 11 CORES (5P + 6E)", 12, "APPLE M3 PR…"},
		{"/a/b", 1, ""},
	} {
		if got := fit(c.in, c.w); got != c.want || utf8.RuneCountInString(got) > c.w {
			t.Errorf("fit(%q, %d) = %q, want %q", c.in, c.w, got, c.want)
		}
	}
}
