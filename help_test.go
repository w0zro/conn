package main

import (
	"os"
	"regexp"
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
	for _, want := range []string{"NAME", "KEYS", "ctrl-space"} {
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
	if got := m.keys(); !strings.Contains(got, wordmarkLine) {
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
	if got := m.keys(); !strings.Contains(got, "HELP") || strings.Contains(got, wordmarkLine) {
		t.Errorf("the panel says %q", got)
	}
	// The word is the view's again once the manual is out of the
	// workspace, which the reading is what says.
	next, _ = m.Update(processesMsg{gen: m.processesGen})
	if got := next.(model).keys(); !strings.Contains(got, wordmarkLine) {
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
	if got := m.keys(); !strings.Contains(got, wordmarkLine) {
		t.Errorf("the list says %q while the manual is up", got)
	}
	if got := m.station(); !strings.Contains(got, helpWord) {
		t.Errorf("the station says %q while the manual is up", got)
	}
	m.helping = false
	if got := m.station(); !strings.Contains(got, wordmarkLine) {
		t.Errorf("the station says %q with nothing up, not the wordmark", got)
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

// The panel key leaves the manual for the processes view, and is the
// way out that does not first ask what you were doing. The manual is
// conn's own furniture and the keys step over furniture, so nothing
// else can reach it: a manual that opened and would not close would be
// a trap rather than a help.
func TestThePanelKeyLeavesTheManual(t *testing.T) {
	m := model{view: viewProcesses, inside: true, srv: &server{}, helping: true,
		helpFrom: "%4", panes: map[string]pane{"ttys011": {id: "%4", tty: "ttys011"}}}
	next, cmd := m.key("alt+-")
	if got := next.(model); got.helping {
		t.Error("the manual is still up")
	}
	// The key says where to go, so it does not put the keys back in the
	// workspace the manual was asked from.
	if got := next.(model); got.helpFrom != "" {
		t.Errorf("the panel key kept the pane the manual was asked from: %q", got.helpFrom)
	}
	if cmd == nil {
		t.Error("nothing was done to put it away")
	}
	if got := next.(model).keys(); !strings.Contains(got, wordmarkLine) {
		t.Errorf("the panel still says %q", got)
	}
	// With no manual up, pressed on the panel, it is the other process,
	// and with nothing behind this one there is nowhere to go.
	m.helping = false
	if _, cmd := m.key("alt+-"); cmd != nil {
		t.Error("conn went somewhere with nothing to go back to")
	}
}

// Reading the manual is a detour, so leaving it puts the keys back
// where the chord took them from: into the workspace where that is
// where they were, and on the panel where the operator was working the
// view. An answer to a question is not a reason to move somebody.
func TestLeavingTheManualPutsTheKeysBackWhereTheyWere(t *testing.T) {
	work := pane{id: "%4", tty: "ttys011"}
	panes := map[string]pane{"ttys011": work}

	// Asked from the workspace: back into that pane.
	m := model{view: viewProcesses, inside: true, srv: &server{}, helping: true,
		helpFrom: "%4", panes: panes, lastIn: "ttys009"}
	next, cmd := m.leftHelp(false)
	got := next.(model)
	if got.helping || got.helpFrom != "" {
		t.Errorf("leaving left helping %v from %q", got.helping, got.helpFrom)
	}
	if cmd == nil {
		t.Error("nothing was done to put the keys back in the workspace")
	}

	// Asked from the panel: the keys stay on the panel, and the pane the
	// manual was standing in front of is not gone back into.
	m = model{view: viewProcesses, inside: true, srv: &server{}, helping: true,
		panes: panes, lastIn: "ttys011"}
	next, cmd = m.leftHelp(false)
	if got := next.(model); got.helping {
		t.Error("leaving from the panel left conn helping")
	}
	if cmd == nil {
		t.Error("the workspace was left holding the manual")
	}

	// The pane the chord came from can go while the manual is up; then
	// there is nothing to be put back into.
	m = model{view: viewProcesses, inside: true, srv: &server{}, helping: true,
		helpFrom: "%9", panes: panes}
	if _, cmd := m.leftHelp(false); cmd == nil {
		t.Error("a chord from a pane that has gone left the workspace as it was")
	}
}

// Leaving the manual goes back to the work it was standing in front of.
// The manual says so as it goes — a key, the way the chords speak to
// the panel — so the workspace is filled in the same breath instead of
// holding a dead pane until the next reading comes round.
func TestLeavingTheManualGoesBackToTheWork(t *testing.T) {
	work := pane{id: "%2", tty: "ttys009"}
	m := model{view: viewProcesses, inside: true, srv: &server{}, helping: true,
		lastIn: "ttys009", panes: map[string]pane{"ttys009": work}}
	next, cmd := m.key("alt+esc")
	got := next.(model)
	if got.helping {
		t.Error("conn still thinks the manual is up")
	}
	if cmd == nil {
		t.Fatal("nothing was done to put the workspace back")
	}
	// It is the same answer wherever the news comes from: a manual that
	// ended without saying is found dead by the reading, and handled the
	// same way rather than by a second rule that could drift from this.
	m.helping = true
	next, cmd = m.Update(processesMsg{gen: m.processesGen, bayDead: true, bayHelp: true,
		panes: map[string]pane{"ttys009": work}})
	if got := next.(model); got.helping {
		t.Error("a manual found dead left conn still helping")
	}
	if cmd == nil {
		t.Error("a manual found dead put nothing back")
	}
}

// With nothing to go back to the workspace takes a hold and the keys
// come to the panel: reading is over, and a placard is not somewhere to
// leave the operator standing.
func TestLeavingTheManualWithNothingToGoBackTo(t *testing.T) {
	m := model{view: viewProcesses, inside: true, srv: &server{}, helping: true}
	next, cmd := m.key("alt+esc")
	if got := next.(model); got.helping {
		t.Error("conn still thinks the manual is up")
	}
	if cmd == nil {
		t.Error("the workspace was left as it was")
	}
}

// The row under the cursor comes back with the operator. Asking the
// manual a question is not unchoosing what they were looking at, and
// coming back to the view with a different row picked out would be conn
// deciding they had.
func TestTheRowComesBackFromTheManual(t *testing.T) {
	projects := []project{{path: "/w", entries: []entry{
		{pid: 11, tty: "ttys001"}, {pid: 22, tty: "ttys002"}, {pid: 33, tty: "ttys003"},
	}}}
	m := model{view: viewProcesses, inside: true, srv: &server{}, projects: projects,
		cursor: 22, cursorAt: 1}
	next, _ := m.key("?")
	m = next.(model)
	if m.cursor != 0 {
		t.Errorf("a row is still under the cursor while the manual is up: %d", m.cursor)
	}
	if m.helpCursor != 22 {
		t.Errorf("the row was dropped rather than kept: %d", m.helpCursor)
	}
	// A reading while the manual is up does not hand a row back either.
	next, _ = m.Update(processesMsg{projects: projects, gen: m.processesGen, bayHelp: true})
	m = next.(model)
	if m.cursor != 0 {
		t.Errorf("the reading put a row under the cursor: %d", m.cursor)
	}
	// And leaving gives it back, the same row and not the same place in
	// the list: a process that ended while the manual was up would have
	// left another row standing where it was.
	next, _ = m.key("alt+esc")
	if got := next.(model); got.cursor != 22 || got.helpCursor != 0 {
		t.Errorf("leaving came back to row %d (kept %d), want 22", got.cursor, got.helpCursor)
	}
}

// Asked with no row under the cursor, the manual leaves with none.
func TestNoRowGoesInAndNoneComesBack(t *testing.T) {
	m := model{view: viewProcesses, inside: true, srv: &server{}, helping: true}
	next, _ := m.key("alt+esc")
	if got := next.(model); got.cursor != 0 || got.helpCursor != 0 {
		t.Errorf("leaving invented row %d", got.cursor)
	}
}

// The page is held to the binary: its SYNOPSIS names every command conn
// offers and every flag it takes, and no command conn does not answer
// to; its KEYS section names the one key tmux takes, as conn binds it.
func TestTheManPageIsHeldToTheBinary(t *testing.T) {
	page := string(manPage)
	i, j := strings.Index(page, ".SH SYNOPSIS"), strings.Index(page, ".SH DESCRIPTION")
	if i < 0 || j < i {
		t.Fatal("the page has no synopsis")
	}
	known := map[string]bool{}
	for _, c := range commands {
		if c.use != "" {
			known[c.name] = true
		}
	}
	for _, f := range flags {
		known[f.name] = true
	}
	syn := strings.ReplaceAll(page[i:j], `\-`, "-")
	for name := range known {
		if !strings.Contains(syn, name) {
			t.Errorf("the page's synopsis lacks %q", name)
		}
	}
	for _, line := range strings.Split(syn, "\n") {
		if f := strings.Fields(line); len(f) >= 3 && f[0] == ".B" && f[1] == "conn" && !known[f[2]] {
			t.Errorf("the page offers %q, which conn does not answer to", line)
		}
	}
	bound := regexp.MustCompile(`(?m)^bind (\S+) (\S+) `).FindAllStringSubmatch(tmuxConf(defaultKey), -1)
	if len(bound) != 1 || bound[0][1] != "-n" || bound[0][2] != defaultKey {
		t.Fatalf("conn binds %v, not the panel key alone in the root table", bound)
	}
	k := strings.Index(page, ".SH KEYS")
	if k < 0 {
		t.Fatal("the page has no keys section")
	}
	keys := strings.ReplaceAll(page[k:], `\-`, "-")
	say := strings.ToLower(strings.ReplaceAll(defaultKey, "C-", "ctrl-"))
	for _, want := range []string{say, "CONN_KEY"} {
		if !strings.Contains(keys, want) {
			t.Errorf("the page's keys section lacks %s", want)
		}
	}
}
