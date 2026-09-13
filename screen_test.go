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

// Off a terminal the page keeps every value whole. There is no screen
// to cut it to and nobody watching who could widen one: what comes out
// of the pipe is filed or pasted into a report, and a reading elided
// to fit a width nobody chose is a reading lost.
func TestThePipedConsoleElidesNothing(t *testing.T) {
	st := testStation
	st.login.exe = "/Users/w0zro/Library/Caches/go-build/3c/3c7f8105cf5abd1baa5f58b9cf8c907eed6bf5ff84596c80c85d92931375d3fa-d/conn"
	st.machine.kernel = "Darwin 25.6.0 and then some words to push it past eighty columns"
	text := texts(screen(compose(st, testNow), 0, 0, plain))
	if strings.Contains(text, "…") {
		t.Errorf("the piped console elided something:\n%s", text)
	}
	// A path keeps its own case; every other value is set in capitals.
	for _, want := range []string{"~" + st.login.exe[len("/Users/w0zro"):], "16 KB PAGES"} {
		if !strings.Contains(text, want) {
			t.Errorf("the piped console lost %q:\n%s", want, text)
		}
	}
	// A terminal is a width somebody chose, and the page is cut to it.
	if narrow := texts(screen(compose(st, testNow), 80, 40, plain)); !strings.Contains(narrow, "…") {
		t.Error("an eighty column terminal elided nothing")
	}
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
		"SYSTEM .... MACOS 26.6.2 (25G83)", "CWD ....... ~/projects/w0zro/conn", "SHELL ..... ZSH 5.9",
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
	if !strings.Contains(text, "6.8 GB FREE") || !strings.Contains(text, " LOW") || !strings.Contains(text, "1 SYSTEM NOT NOMINAL") {
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

// UNCHECKED is not the same claim NOMINAL is — there was nothing to
// check against — so it stands out in the color something waiting already
// does, apart from a nominal row's gray.
func TestUncheckedStandsOutFromNominal(t *testing.T) {
	p := colored()
	st := testStation
	st.machine.cpus = 0 // LOAD has no core count to check against
	text := texts(screen(compose(st, testNow), 120, 40, p))
	if !strings.Contains(text, p.waiting+"UNCHECKED") {
		t.Errorf("UNCHECKED is not painted waiting:\n%s", stripEscapes(text))
	}
	if !strings.Contains(text, p.gray+"NOMINAL") {
		t.Errorf("NOMINAL is not painted gray:\n%s", stripEscapes(text))
	}
	// A check the machine would not answer is no more a reading than
	// one there was nothing to check against, and is said the same.
	st = testStation
	st.volume = volume{} // DISK went unanswered
	text = texts(screen(compose(st, testNow), 120, 40, p))
	if !strings.Contains(text, p.waiting+"UNKNOWN") {
		t.Errorf("UNKNOWN is not painted waiting:\n%s", stripEscapes(text))
	}
}

// The verdict says every system is nominal only when every system was
// read and passed. A check with nothing to check against, or one the
// machine would not answer, is counted beside the ones that passed:
// conn took no reading there, and a word that called it nominal would
// be claiming one.
func TestTheVerdictCountsWhatWasNotRead(t *testing.T) {
	for _, c := range []struct {
		name string
		t    tally
		want string
	}{
		{"every one read and passed", tally{nominal: 10}, "ALL SYSTEMS NOMINAL"},
		{"one with nothing to check against", tally{nominal: 9, unchecked: 1}, "9 NOMINAL · 1 UNCHECKED"},
		{"one unanswered", tally{nominal: 9, unknown: 1}, "9 NOMINAL · 1 UNKNOWN"},
		{"one of each", tally{nominal: 8, unchecked: 1, unknown: 1}, "8 NOMINAL · 1 UNCHECKED · 1 UNKNOWN"},
		{"nothing read at all", tally{unknown: 10}, "10 UNKNOWN"},
	} {
		if got := c.t.verdict(); got != c.want {
			t.Errorf("%s: the verdict reads %q, want %q", c.name, got, c.want)
		}
	}
	// Read off a console rather than a tally by hand: the machine
	// answers everything but its core count, so LOAD alone is unchecked.
	st := testStation
	st.machine.cpus = 0
	text := stripEscapes(texts(screen(compose(st, testNow), 120, 40, plain)))
	if strings.Contains(text, "ALL SYSTEMS NOMINAL") {
		t.Errorf("the console called an unchecked system nominal:\n%s", text)
	}
	if !strings.Contains(text, "9 NOMINAL · 1 UNCHECKED") {
		t.Errorf("the console does not count what it did not read:\n%s", text)
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
		path bool
		want string
	}{
		{exe + " · 5.5 MB", 90, true, exe + " · 5.5 MB"},
		{exe + " · 5.5 MB", 60, true, "/var/…/T/go-build3688959642/b001/exe/conn · 5.5 MB"},
		{exe + " · 5.5 MB", 30, true, "/var/…/b001/exe/conn · 5.5 MB"},
		{exe + " · 5.5 MB", 22, true, "/var/…/conn · 5.5 MB"},
		{exe, 8, true, "…xe/conn"},
		{"~/projects/w0zro/conn/conn", 20, true, "~/…/w0zro/conn/conn"},
		{"~/projects/w0zro/conn", 40, true, "~/projects/w0zro/conn"},
		{"APPLE M3 PRO · 11 CORES (5P + 6E)", 12, false, "APPLE M3 PR…"},
		{"/NOT/A/PATH/BY/ITS/FLAG", 12, false, "/NOT/A/PATH…"},
		{"/a/b", 1, true, ""},
	} {
		if got := fit(c.in, c.w, c.path); got != c.want || utf8.RuneCountInString(got) > c.w {
			t.Errorf("fit(%q, %d, %v) = %q, want %q", c.in, c.w, c.path, got, c.want)
		}
	}
}

// The console's alarms are annunciators: the verdict's chip and every
// fault's own chip blink together, so what is wrong and how many are
// the one alarm. On the dark half those cells are the ground and
// nothing else moves; what is nominal never blinks, and neither does
// the word that all is well.
func TestTheAlarmsBlink(t *testing.T) {
	st := testStation
	st.volume.free = 6_800_000_000
	r := compose(st, testNow)
	if !r.lit {
		t.Error("a reading is dark before anyone asks it to blink")
	}
	lit := screen(r, 120, 40, plain)
	r.lit = false
	dark := screen(r, 120, 40, plain)
	for _, s := range []string{"1 SYSTEM NOT NOMINAL", " LOW"} {
		if !strings.Contains(texts(lit), s) {
			t.Errorf("%q is not up on the lit half:\n%s", s, texts(lit))
		}
		if strings.Contains(texts(dark), s) {
			t.Errorf("%q is still up on the dark half:\n%s", s, texts(dark))
		}
	}
	// The fault's own line keeps saying which system, and what it read;
	// only the chip goes.
	if !strings.Contains(texts(dark), "6.8 GB FREE") {
		t.Errorf("the fault's measurement went dark with its chip:\n%s", texts(dark))
	}
	// A check that is nominal does not blink.
	if !strings.Contains(texts(dark), "NOMINAL") {
		t.Errorf("what is nominal blinked:\n%s", texts(dark))
	}
	// Two rows differ, the fault's and the verdict's, and no other.
	if len(lit) != len(dark) {
		t.Fatalf("%d rows lit, %d dark", len(lit), len(dark))
	}
	moved := 0
	for i := range lit {
		if lit[i].text != dark[i].text {
			moved++
		}
	}
	if moved != 2 {
		t.Errorf("%d rows differ between lit and dark; the fault's and the verdict's should", moved)
	}
	// Two faults blink three rows: each chip, and the count.
	st.machine.power.percent, st.machine.power.state = 5, "discharging"
	two := compose(st, testNow)
	two.lit = false
	twoDark := screen(two, 120, 40, plain)
	twoLit := screen(compose(st, testNow), 120, 40, plain)
	moved = 0
	for i := range twoLit {
		if twoLit[i].text != twoDark[i].text {
			moved++
		}
	}
	if moved != 3 {
		t.Errorf("with two faults %d rows blink, not 3", moved)
	}
	// All being well is a word, not an annunciator.
	well := compose(testStation, testNow)
	well.lit = false
	if !strings.Contains(texts(screen(well, 120, 40, plain)), "ALL SYSTEMS NOMINAL") {
		t.Error("the word that all is well blinked")
	}
}
