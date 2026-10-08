package main

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/w0zro/conn/internal/draw"
	"github.com/w0zro/conn/internal/room"
	"github.com/w0zro/conn/internal/work"

	"github.com/w0zro/conn/internal/tmux"

	"github.com/w0zro/conn/internal/theme"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
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
	m := model{processesView: processesView{cursor: 4321}, view: viewProcesses, inside: true}
	if got := m.keys(); !strings.Contains(got, wordmarkLine) {
		t.Fatalf("a panel with no manual up says %q", got)
	}
	next, _ := m.Update(detourMsg{toManual})
	m = next.(model)
	if m.detour.to != toManual {
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
	m := model{view: viewProjects, detour: detour{to: toManual}, inside: true}
	if got := m.keys(); !strings.Contains(got, wordmarkLine) {
		t.Errorf("the list says %q while the manual is up", got)
	}
	if got := m.station(); !strings.Contains(got, helpWord) {
		t.Errorf("the station says %q while the manual is up", got)
	}
	m.detour.to = noDetour
	if got := m.station(); !strings.Contains(got, wordmarkLine) {
		t.Errorf("the station says %q with nothing up, not the wordmark", got)
	}
}

// While the manual is up the reading does not hand a row back. follow
// keeps hold of the row the operator was on as the rows change under
// it, and with the manual up there is no such row — without this the
// cursor came back on the next beat, a couple of seconds later.
func TestTheReadingLeavesTheCursorAloneWhileHelping(t *testing.T) {
	projects := []work.Project{{Path: "/w", Entries: []work.Entry{{PID: 11, TTY: "ttys001"}, {PID: 22, TTY: "ttys002"}}}}
	m := model{processesView: processesView{cursor: 0}, reading: reading{projects: projects}, view: viewProcesses, inside: true}
	// A reading that finds the manual in the workspace: conn is helping,
	// and the cursor it was told to let go of stays let go.
	up := processesMsg{reading: reading{projects: projects}, gen: m.processesGen, bayDetour: toManual}
	next, _ := m.Update(up)
	if got := next.(model); got.cursor != 0 || got.detour.to != toManual {
		t.Errorf("the reading put the cursor back on %d (helping %v)", got.cursor, got.detour.to == toManual)
	}
	// And with the manual gone it follows as it always did.
	next, _ = m.Update(processesMsg{reading: reading{projects: projects}, gen: m.processesGen})
	if got := next.(model); got.cursor == 0 || got.detour.to == toManual {
		t.Errorf("with no manual up the reading left the cursor at %d (helping %v)", got.cursor, got.detour.to == toManual)
	}
}

// The manual runs as a conn of its own, which is why it takes no row in
// the processes view: conn's own processes are not among the processes
// conn is holding for the operator.
func TestTheManualIsNotOneOfTheProcesses(t *testing.T) {
	p := work.Process{Command: "/usr/local/bin/conn", Args: []string{"/usr/local/bin/conn", "manual"}}
	if got := work.KindOf(p); got != work.KindConn {
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
	m := newManual(nil, "", draw.Plain)
	m.page.SetWidth(80)
	m.page.SetHeight(10)
	m.page.SetContentLines(m.rows(lines, 80, 10))
	step := func(k string) manualModel {
		next, _ := m.Update(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
		return next.(manualModel)
	}
	if m = step("k"); m.page.YOffset() != 0 {
		t.Errorf("scrolled up from the top to %d", m.page.YOffset())
	}
	if m = step("j"); m.page.YOffset() != 1 {
		t.Errorf("j went to %d", m.page.YOffset())
	}
	if m = step("G"); m.page.YOffset() != 40 {
		t.Errorf("G went to %d, and the last line should sit at the foot", m.page.YOffset())
	}
	if m = step("j"); m.page.YOffset() != 40 {
		t.Errorf("j past the end went to %d", m.page.YOffset())
	}
	if m = step("g"); m.page.YOffset() != 0 {
		t.Errorf("g went to %d", m.page.YOffset())
	}
	// A page shorter than the pane is drawn down to its foot all the
	// same, on the ground.
	if got := m.rows([]string{"line"}, 80, 10); len(got) != 10 {
		t.Errorf("a one-line page is %d rows in a pane of 10", len(got))
	}
}

// The panel key leaves the manual for the processes view, and is the
// way out that does not first ask what you were doing. The manual is
// conn's own furniture and the keys step over furniture, so nothing
// else can reach it: a manual that opened and would not close would be
// a trap rather than a help.
func TestThePanelKeyLeavesTheManual(t *testing.T) {
	m := model{reading: reading{panes: map[string]room.Pane{"ttys011": {ID: "%4", TTY: "ttys011"}}}, view: viewProcesses, inside: true, srv: room.New(&tmux.Server{}), detour: detour{to: toManual, from: "%4"}}
	next, cmd := m.key(room.Arrived.Heard())
	if got := next; got.detour.to == toManual {
		t.Error("the manual is still up")
	}
	// The key says where to go, so it does not put the keys back in the
	// workspace the manual was asked from.
	if got := next; got.detour.from != "" {
		t.Errorf("the panel key kept the pane the manual was asked from: %q", got.detour.from)
	}
	if cmd == nil {
		t.Error("nothing was done to put it away")
	}
	if got := next.keys(); !strings.Contains(got, wordmarkLine) {
		t.Errorf("the panel still says %q", got)
	}
	// With no manual up, pressed on the panel, it is the other process,
	// and with nothing behind this one there is nowhere to go.
	m.detour.to = noDetour
	if _, cmd := m.key(room.Arrived.Heard()); cmd != nil {
		t.Error("conn went somewhere with nothing to go back to")
	}
}

// Reading the manual is a detour, so leaving it puts the keys back
// where the chord took them from: into the workspace where that is
// where they were, and on the panel where the operator was working the
// view. An answer to a question is not a reason to move somebody.
func TestLeavingTheManualPutsTheKeysBackWhereTheyWere(t *testing.T) {
	work := room.Pane{ID: "%4", TTY: "ttys011"}
	panes := map[string]room.Pane{"ttys011": work}

	// Asked from the workspace: back into that pane.
	m := model{reading: reading{panes: panes}, view: viewProcesses, inside: true, srv: room.New(&tmux.Server{}), detour: detour{to: toManual, from: "%4"}, bay: bay{work: "ttys009"}}
	next, cmd := m.leftDetour(false)
	got := next
	if got.detour.to == toManual || got.detour.from != "" {
		t.Errorf("leaving left helping %v from %q", got.detour.to == toManual, got.detour.from)
	}
	if cmd == nil {
		t.Error("nothing was done to put the keys back in the workspace")
	}

	// Asked from the panel: the keys stay on the panel, and the pane the
	// manual was standing in front of is not gone back into.
	m = model{reading: reading{panes: panes}, view: viewProcesses, inside: true, srv: room.New(&tmux.Server{}), detour: detour{to: toManual}, bay: bay{work: "ttys011"}}
	next, cmd = m.leftDetour(false)
	if got := next; got.detour.to == toManual {
		t.Error("leaving from the panel left conn helping")
	}
	if cmd == nil {
		t.Error("the workspace was left holding the manual")
	}

	// The pane the chord came from can go while the manual is up; then
	// there is nothing to be put back into.
	m = model{reading: reading{panes: panes}, view: viewProcesses, inside: true, srv: room.New(&tmux.Server{}), detour: detour{to: toManual, from: "%9"}}
	if _, cmd := m.leftDetour(false); cmd == nil {
		t.Error("a chord from a pane that has gone left the workspace as it was")
	}
}

// Leaving the manual goes back to the work it was standing in front of.
// The manual says so as it goes — a key, the way the chords speak to
// the panel — so the workspace is filled in the same breath instead of
// holding a dead pane until the next reading comes round.
func TestLeavingTheManualGoesBackToTheWork(t *testing.T) {
	work := room.Pane{ID: "%2", TTY: "ttys009"}
	m := model{reading: reading{panes: map[string]room.Pane{"ttys009": work}}, view: viewProcesses, inside: true, srv: room.New(&tmux.Server{}), detour: detour{to: toManual},
		bay: bay{work: "ttys009"}}
	next, cmd := m.key(room.LeftHelp.Heard())
	got := next
	if got.detour.to == toManual {
		t.Error("conn still thinks the manual is up")
	}
	if cmd == nil {
		t.Fatal("nothing was done to put the workspace back")
	}
	// It is the same answer wherever the news comes from: a manual that
	// ended without saying is found dead by the reading, and handled the
	// same way rather than by a second rule that could drift from this.
	m.detour.to = toManual
	found, cmd := m.Update(processesMsg{reading: reading{panes: map[string]room.Pane{"ttys009": work}}, gen: m.processesGen, bayDead: true, bayDetour: toManual})
	if got := found.(model); got.detour.to == toManual {
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
	m := model{view: viewProcesses, inside: true, srv: room.New(&tmux.Server{}), detour: detour{to: toManual}}
	next, cmd := m.key(room.LeftHelp.Heard())
	if got := next; got.detour.to == toManual {
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
	projects := []work.Project{{Path: "/w", Entries: []work.Entry{
		{PID: 11, TTY: "ttys001"}, {PID: 22, TTY: "ttys002"}, {PID: 33, TTY: "ttys003"},
	}}}
	m := model{processesView: processesView{cursor: 22, cursorAt: 1}, reading: reading{projects: projects}, view: viewProcesses, inside: true, srv: room.New(&tmux.Server{})}
	next, _ := m.key("?")
	m = next
	if m.cursor != 0 {
		t.Errorf("a row is still under the cursor while the manual is up: %d", m.cursor)
	}
	if m.detour.cursor != 22 {
		t.Errorf("the row was dropped rather than kept: %d", m.detour.cursor)
	}
	// A reading while the manual is up does not hand a row back either.
	read, _ := m.Update(processesMsg{reading: reading{projects: projects}, gen: m.processesGen, bayDetour: toManual})
	m = read.(model)
	if m.cursor != 0 {
		t.Errorf("the reading put a row under the cursor: %d", m.cursor)
	}
	// And leaving gives it back, the same row and not the same place in
	// the list: a process that ended while the manual was up would have
	// left another row standing where it was.
	next, _ = m.key(room.LeftHelp.Heard())
	if got := next; got.cursor != 22 || got.detour.cursor != 0 {
		t.Errorf("leaving came back to row %d (kept %d), want 22", got.cursor, got.detour.cursor)
	}
}

// Asked with no row under the cursor, the manual leaves with none.
func TestNoRowGoesInAndNoneComesBack(t *testing.T) {
	m := model{view: viewProcesses, inside: true, srv: room.New(&tmux.Server{}), detour: detour{to: toManual}}
	next, _ := m.key(room.LeftHelp.Heard())
	if got := next; got.cursor != 0 || got.detour.cursor != 0 {
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
	// The root table is what reaches through a process; copy mode's own
	// table binds the keys the key bar says there, and takes nothing.
	bound := regexp.MustCompile(`(?m)^bind -n (\S+) `).FindAllStringSubmatch(serverConfOn(room.DefaultKey, theme.Conn.Dark), -1)
	if len(bound) != 1 || bound[0][1] != room.DefaultKey {
		t.Fatalf("conn binds %v in the root table, not the panel key alone", bound)
	}
	k := strings.Index(page, ".SH KEYS")
	if k < 0 {
		t.Fatal("the page has no keys section")
	}
	keys := strings.ReplaceAll(page[k:], `\-`, "-")
	say := strings.ToLower(strings.ReplaceAll(room.DefaultKey, "C-", "ctrl-"))
	for _, want := range []string{say, "CONN_KEY"} {
		if !strings.Contains(keys, want) {
			t.Errorf("the page's keys section lacks %s", want)
		}
	}
}

// The page is wrapped inside the margin: a line man fills to its width
// stands three columns in and ends inside the pane, where a line
// filled to the pane's own width ran past its right edge.
func TestTheManualIsWrappedInsideTheMargin(t *testing.T) {
	if got := manWidth(100); got != 100-draw.Margin {
		t.Errorf("a pane of 100 asks man for %d, want %d", got, 100-draw.Margin)
	}
	if got := manWidth(40); got != draw.MinCols {
		t.Errorf("a narrow pane asks man for %d, want the floor %d", got, draw.MinCols)
	}
	m := manualModel{p: draw.Plain}
	line := strings.Repeat("x", manWidth(100))
	for _, r := range m.rows([]string{line}, 100, 1) {
		if w := ansi.StringWidth(r); w > 100 {
			t.Errorf("a full line is %d cells in a pane of 100", w)
		}
	}
}
