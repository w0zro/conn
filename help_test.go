package main

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// The manual conn shows is the one it was built with. A conn run out of
// a build directory has no installed page, and an installed conn may
// have an older page beside a newer binary; the one in the binary is
// the only one certain to describe the conn showing it.
func TestConnCarriesItsOwnManual(t *testing.T) {
	onDisk, err := os.ReadFile("man/conn.1")
	if err != nil {
		t.Skip(err)
	}
	if string(manPage) != string(onDisk) {
		t.Error("the manual in the binary is not the page in the tree")
	}
	if !strings.Contains(string(manPage), ".SH KEYS") {
		t.Error("the manual carries no keys section")
	}
}

// conn writes the page beside its own state and points man at it.
func TestTheManualIsWrittenWhereManCanReadIt(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	home := t.TempDir()
	path, err := writeManPage(home)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != string(manPage) {
		t.Fatalf("the page conn wrote: %v", err)
	}
	// And man reads it back as a page, which is what conn pages.
	lines := manText(path, 80)
	if len(lines) < 10 {
		t.Fatalf("the page read back as %d lines", len(lines))
	}
	var text strings.Builder
	for _, l := range lines {
		for _, r := range manLine(l) {
			text.WriteString(r.text)
		}
		text.WriteString("\n")
	}
	for _, want := range []string{"NAME", "KEYS", "prefix ?"} {
		if !strings.Contains(text.String(), want) {
			t.Errorf("the page conn pages lacks %q", want)
		}
	}
}

// While the manual is in the workspace the panel says HELP, and no row
// is under the cursor: the manual is not a process, so there is no row
// these keys are about.
func TestTheManualPutsThePanelInHelp(t *testing.T) {
	m := model{view: viewProcesses, cursor: 4321, inside: true}
	if got := m.keys(); !strings.Contains(got, "PROCS") {
		t.Fatalf("a panel with no manual up says %q", got)
	}
	next, _ := m.Update(helpMsg{on: true})
	m = next.(model)
	if !m.helping {
		t.Error("conn does not know the manual is up")
	}
	if m.cursor != 0 {
		t.Errorf("a row is still under the cursor: %d", m.cursor)
	}
	if got := m.keys(); !strings.Contains(got, "HELP") || strings.Contains(got, "PROCS") {
		t.Errorf("the panel says %q", got)
	}
	// The word is the view's again once the manual is out of the
	// workspace, which the reading is what says.
	next, _ = m.Update(processesMsg{gen: m.processesGen})
	if got := next.(model).keys(); !strings.Contains(got, "PROCS") {
		t.Errorf("with the manual gone the panel says %q", got)
	}
}

// Off the processes view the panel's word is the view's own: the manual
// may be standing in the workspace while the operator works the list,
// and there the keys are the list's. The station says HELP all the
// same, because that is the word for a line whose keys are in the
// manual rather than on the panel at all.
func TestHelpIsThePanelsWordOnlyInTheProcessesView(t *testing.T) {
	m := model{view: viewProjects, helping: true, inside: true}
	if got := m.keys(); !strings.Contains(got, "PROJECTS") {
		t.Errorf("the list says %q while the manual is up", got)
	}
	if got := m.station(); !strings.Contains(got, helpWord) {
		t.Errorf("the station says %q while the manual is up", got)
	}
	m.helping = false
	if got := m.station(); got != "" {
		t.Errorf("the station says %q with nothing up", got)
	}
}

// The chord that opens the manual puts it away again. The manual is
// conn's own furniture and the keys step over furniture, so nothing
// else can reach it: a manual that opened and would not close would be
// a trap rather than a help.
func TestTheChordPutsTheManualAwayAgain(t *testing.T) {
	m := model{view: viewProcesses, inside: true, srv: &server{}, cursor: 77, helping: true}
	next, cmd := m.key("alt+?")
	m = next.(model)
	if m.helping {
		t.Error("the manual is still up after the chord that closes it")
	}
	if cmd == nil {
		t.Error("nothing was done to put it away")
	}
	if got := m.keys(); !strings.Contains(got, "PROCS") {
		t.Errorf("the panel still says %q", got)
	}
}

// While the manual is up the reading does not hand a row back. follow
// keeps hold of the row the operator was on as the rows change under
// it, and with the manual up there is no such row — without this the
// cursor came back on the next beat, a couple of seconds later.
func TestTheReadingLeavesTheCursorAloneWhileHelping(t *testing.T) {
	projects := []project{{path: "/w", entries: []entry{{pid: 11, tty: "ttys001"}, {pid: 22, tty: "ttys002"}}}}
	m := model{view: viewProcesses, inside: true, cursor: 0, projects: projects}
	// A reading that finds the manual in the workspace: conn is helping,
	// and the cursor it was told to let go of stays let go.
	up := processesMsg{projects: projects, gen: m.processesGen, bayHelp: true}
	next, _ := m.Update(up)
	if got := next.(model); got.cursor != 0 || !got.helping {
		t.Errorf("the reading put the cursor back on %d (helping %v)", got.cursor, got.helping)
	}
	// And with the manual gone it follows as it always did.
	next, _ = m.Update(processesMsg{projects: projects, gen: m.processesGen})
	if got := next.(model); got.cursor == 0 || got.helping {
		t.Errorf("with no manual up the reading left the cursor at %d (helping %v)", got.cursor, got.helping)
	}
}

// The manual runs as a conn of its own, which is why it takes no row in
// the processes view: conn's own processes are not among the processes
// conn is holding for the operator.
func TestTheManualIsNotOneOfTheProcesses(t *testing.T) {
	p := process{command: "/usr/local/bin/conn", args: []string{"/usr/local/bin/conn", "manual"}}
	if got := kindOf(p); got != kindConn {
		t.Errorf("conn manual reads as %s, and would take a row", got)
	}
}

// man says bold and underline by overstriking. The manual is drawn from
// conn's palette, so the overstrike comes off and what it stood for is
// kept.
func TestTheOverstrikeBecomesEmphasis(t *testing.T) {
	for _, c := range []struct {
		in    string
		text  string
		bolds int
	}{
		{"N\bNA\bAM\bME\bE", "NAME", 1},
		{"       conn", "       conn", 0},
		{"a _\bb c", "a b c", 1},
		{"", "", 0},
	} {
		runs := manLine(c.in)
		var text string
		bolds := 0
		for _, r := range runs {
			text += r.text
			if r.bold {
				bolds++
			}
		}
		if text != c.text || bolds != c.bolds {
			t.Errorf("manLine(%q) = %q with %d bold runs, want %q with %d", c.in, text, bolds, c.text, c.bolds)
		}
	}
}

// The manual scrolls, and stops at both ends: a page scrolled past its
// last line is a screen of nothing with the text gone off the top.
func TestTheManualScrollsAndStops(t *testing.T) {
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = "line"
	}
	m := manualModel{lines: lines, height: 10, width: 80, p: plain}
	step := func(k string) manualModel {
		next, _ := m.Update(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
		return next.(manualModel)
	}
	if m = step("k"); m.top != 0 {
		t.Errorf("scrolled up from the top to %d", m.top)
	}
	if m = step("j"); m.top != 1 {
		t.Errorf("j went to %d", m.top)
	}
	if m = step("G"); m.top != 40 {
		t.Errorf("G went to %d, and the last line should sit at the foot", m.top)
	}
	if m = step("j"); m.top != 40 {
		t.Errorf("j past the end went to %d", m.top)
	}
	if m = step("g"); m.top != 0 {
		t.Errorf("g went to %d", m.top)
	}
}

// prefix - leaves the manual for the processes view, and is the way out
// that does not first ask what you were doing.
func TestPrefixMinusLeavesTheManual(t *testing.T) {
	m := model{view: viewProcesses, inside: true, srv: &server{}, helping: true}
	next, cmd := m.key("alt+-")
	if got := next.(model); got.helping {
		t.Error("the manual is still up")
	}
	if cmd == nil {
		t.Error("nothing was done to put it away")
	}
	// With no manual up it is tmux's own business and conn does nothing.
	m.helping = false
	if _, cmd := m.key("alt+-"); cmd != nil {
		t.Error("conn acted on prefix - with no manual up")
	}
}

// Leaving the manual from inside it goes back to the work it was
// standing in front of. The pane ends, the workspace is left holding a
// dead manual, and that is conn's cue.
func TestLeavingTheManualGoesBackToTheWork(t *testing.T) {
	m := model{view: viewProcesses, inside: true, srv: &server{}, helping: true,
		lastIn: "ttys009", panes: map[string]pane{"ttys009": {id: "%2", tty: "ttys009"}}}
	next, _ := m.Update(processesMsg{gen: m.processesGen, bayDead: true, bayHelp: true})
	if got := next.(model); got.helping {
		t.Error("conn still thinks the manual is up")
	}
}
