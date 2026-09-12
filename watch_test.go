package main

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

func testWatch() watchReport {
	return composeWatch(watch(testProcs, 501, testRoots, testIsProject, nil), nil, "", testProjRoots, "/Users/w0zro", watchNow, "")
}

// The watch at 120 by 40 is a file of record, as are the empty watch and
// the one that could not be read.
func TestWatchMatchesTheGolden(t *testing.T) {
	golden(t, "watch-120x40.txt", texts(drawWatch(testWatch(), 67040, 120, 40, plain)))
	golden(t, "watch-cursor-100x9.txt", texts(drawWatch(testWatch(), 80002, 100, 9, plain)))
	empty := composeWatch(nil, nil, "", testProjRoots, "/Users/w0zro", watchNow, "")
	golden(t, "watch-empty-80x24.txt", texts(drawWatch(empty, 0, 80, 24, plain)))
	failed := composeWatch(nil, nil, "", testProjRoots, "/Users/w0zro", watchNow, "the process table could not be read: lsof: not found")
	golden(t, "watch-unread-80x24.txt", texts(drawWatch(failed, 0, 80, 24, plain)))
	rail := composeWatch(watch(testProcs, 501, testRoots, testIsProject, nil), map[string]pane{"ttys005": {id: "%0"}, "ttys007": {id: "%3"}}, "ttys007", testProjRoots, "/Users/w0zro", watchNow, "")
	rail.inside = true
	golden(t, "watch-rail-48x30.txt", texts(drawWatch(rail, 70100, 48, 30, plain)))
}

// The watch's columns hold: the status flush right, a root's kind at
// the margin and what runs under it a level in per level, the path
// from ~, the ages as of the clock, no legend on the bottom row, no
// row past the width.
func TestWatchLaysOut(t *testing.T) {
	rows := drawWatch(testWatch(), 70100, 120, 40, plain)
	text := texts(rows)
	measure, _, _ := columns(120)
	for _, s := range []string{
		// The name alone: the station and the clock stood against the
		// right of this row and are the bar's now.
		"CONN", "KIND    COMMAND", "TTY", "AGE", "STATUS",
		// A place is named by what is left of its path once the root the
		// checkouts are kept under is taken off it; one outside every
		// root is written from ~, whole.
		"w0zro/conn", "2 PROCESSES", "SHELL   zsh", "TTYS005", "1M 30S", "IDLE",
		"w0zro/vim.pro/conjurer", "47M 00S", "ACTIVE",
		"~", "1D 01H", " STOPPED",
		// A root at the margin, and the tree under it stepping in: the
		// agent its shell runs, the shell the agent runs, the go that
		// one runs. The cursor's mark sits in the margin regardless.
		"\n   SHELL   zsh",
		"\n       SHELL   bash -c go test ./...",
		"\n         RUN     go test ./...",
		"\n     EDITOR  vim notes.md",
		" ▸   AGENT   claude --resume",
	} {
		if !strings.Contains(text, s) {
			t.Errorf("watch lacks %q:\n%s", s, text)
		}
	}
	if len(rows) != 40 || strings.TrimSpace(rows[39].text) != "" {
		t.Errorf("%d rows; the last is %q", len(rows), rows[len(rows)-1].text)
	}
	for _, r := range rows {
		if w := utf8.RuneCountInString(r.text); w > 120 {
			t.Errorf("row is %d wide: %q", w, r.text)
		}
		if strings.HasSuffix(r.text, "ACTIVE") && utf8.RuneCountInString(r.text) != margin+measure {
			t.Errorf("status is not flush with %d: %q", margin+measure, r.text)
		}
	}
	for i, r := range drawWatch(testWatch(), 70100, 120, 40, colored()) {
		if w := utf8.RuneCountInString(stripEscapes(r.text)); w != 120 {
			t.Errorf("colored row %d paints %d columns", i, w)
		}
	}
	if strings.Count(text, "▸") != 1 {
		t.Errorf("the cursor marks %d rows", strings.Count(text, "▸"))
	}
}

// A watch taller than the terminal scrolls to keep the cursor in view
// and says how many rows are above and below. Nine rows are nine rows of
// it: the foot it used to keep against a note is the bar's now.
func TestAWatchThatWillNotFitScrolls(t *testing.T) {
	rows := drawWatch(testWatch(), 80001, 100, 9, plain)
	text := texts(rows)
	if len(rows) != 9 || !strings.Contains(text, "… 11 BELOW") || strings.Contains(text, "ABOVE") || !strings.Contains(text, "▸ SHELL") {
		t.Errorf("at 100x9 with the cursor on the first row:\n%s", text)
	}
	rows = drawWatch(testWatch(), 70301, 100, 9, plain)
	text = texts(rows)
	// The cursor's mark keeps the margin; the row it marks still steps
	// in for the level it is at.
	if len(rows) != 9 || !strings.Contains(text, "… 11 ABOVE") || strings.Contains(text, "BELOW") || !strings.Contains(text, "▸       RUN") {
		t.Errorf("at 100x9 with the cursor on the last row:\n%s", text)
	}
	if piped := drawWatch(testWatch(), 80001, 0, 0, plain); strings.Contains(texts(piped), "ABOVE") {
		t.Errorf("off a terminal:\n%s", texts(piped))
	}
}

// The cursor moves with j and k, stays within the rows, and follows its
// process across readings; when the process goes it holds its row.
func TestTheCursorFollowsItsProcess(t *testing.T) {
	m := model{p: plain, width: 120, height: 40, view: viewWatch, uid: 501, roots: testRoots, now: watchNow}
	next, _ := m.Update(watchMsg{places: watch(testProcs, 501, testRoots, testIsProject, nil)})
	m = next.(model)
	// The rows read oldest first: home's shell and the vim it holds
	// stopped, then conn's two shells, then the conjurer's tree — its
	// shell, the claude it runs, the node that one started, the bash it
	// started after, and the bash's own go.
	if m.cursor != 80001 {
		t.Errorf("the cursor should start on the first row, not %d", m.cursor)
	}
	press := func(k string) {
		next, _ := m.Update(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
		m = next.(model)
	}
	press("j")
	press("j")
	press("j")
	if m.cursor != 67040 || m.cursorAt != 3 {
		t.Errorf("after three j the cursor is on %d at %d", m.cursor, m.cursorAt)
	}
	press("k")
	press("k")
	if m.cursor != 80002 {
		t.Errorf("after two k the cursor is on %d", m.cursor)
	}
	// A reading that still has the pid keeps the cursor on it, wherever
	// in the rows it has moved to.
	next, _ = m.Update(watchMsg{places: watch(testProcs, 501, testRoots, testIsProject, nil)})
	m = next.(model)
	if m.cursor != 80002 || m.cursorAt != 1 {
		t.Errorf("the cursor left the pid it was on: %d at %d", m.cursor, m.cursorAt)
	}
	// Down to claude itself, and then claude gone: what it ran stands on
	// its own, and the cursor, with no pid of its own left to follow,
	// holds the row it was at — which the node it started now has.
	for range 4 {
		press("j")
	}
	if m.cursor != 70100 || m.cursorAt != 5 {
		t.Errorf("the cursor is on %d at %d, not on claude", m.cursor, m.cursorAt)
	}
	var without []process
	for _, p := range testProcs {
		if p.pid != 70100 {
			without = append(without, p)
		}
	}
	next, _ = m.Update(watchMsg{places: watch(without, 501, testRoots, testIsProject, nil)})
	m = next.(model)
	if m.cursorAt != 5 || m.cursor != 70212 {
		t.Errorf("with its process gone the cursor is on %d at %d", m.cursor, m.cursorAt)
	}
	next, _ = m.Update(watchMsg{})
	m = next.(model)
	if m.cursor != 0 || strings.Contains(m.View().Content, "▸") {
		t.Errorf("an empty watch has a cursor: %d", m.cursor)
	}
}

// A key at the end of the console goes to the watch, which reads the
// table and reads it again on its tick; c brings the console back, and a
// stale tick is dropped.
func TestTheKeyContinuesToTheWatch(t *testing.T) {
	m := model{head: station{build: testStation.build, session: testStation.session}, now: watchNow, p: plain, width: 120, height: 40, uid: 501, roots: testRoots}
	st := testStation
	m.st = &st
	m.stage = lastStage(m.report())
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = next.(model)
	if m.view != viewConsole || !m.entering || cmd == nil {
		t.Fatalf("a key at the end should read the watch and hold the console for the answer")
	}
	// The console holds rather than putting an empty watch up: the watch
	// arrives with its rows in it, in one change of the screen.
	if !strings.Contains(m.View().Content, "START-UP CHECKS") {
		t.Errorf("the console should still be up while the reading is on its way:\n%s", m.View().Content)
	}
	next, cmd = m.Update(watchMsg{places: watch(testProcs, 501, testRoots, testIsProject, nil), gen: m.watchGen})
	m = next.(model)
	if m.view != viewWatch || m.entering {
		t.Fatalf("the reading the console was waiting on did not put the watch up")
	}
	if cmd == nil || !strings.Contains(m.View().Content, "claude --resume") {
		t.Errorf("the watch should show what was read and set the tick going:\n%s", m.View().Content)
	}
	if _, cmd := m.Update(watchTickMsg{gen: m.watchGen - 1}); cmd != nil {
		t.Error("a stale tick should be dropped")
	}
	if _, cmd := m.Update(watchTickMsg{gen: m.watchGen}); cmd == nil {
		t.Error("the tick should read the watch again")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = next.(model)
	if m.view != viewConsole || !strings.Contains(m.View().Content, "START-UP CHECKS") {
		t.Errorf("c should bring the console back:\n%s", m.View().Content)
	}
	if _, cmd := m.Update(watchTickMsg{gen: m.watchGen}); cmd != nil {
		t.Error("a tick off the watch should be dropped")
	}
	// Coming back, the rows of the last stay are still in hand, so the
	// watch goes up with them at once rather than holding for a reading.
	next, cmd = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = next.(model)
	if m.view != viewWatch || m.entering || cmd == nil || m.watchGen != 2 {
		t.Errorf("a key on the finished console should return to the watch and read it afresh: gen %d", m.watchGen)
	}
	if !strings.Contains(m.View().Content, "claude --resume") {
		t.Errorf("the watch came back empty rather than with the rows it had:\n%s", m.View().Content)
	}
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"}); cmd == nil {
		t.Error("q should close conn from the watch")
	}
}

// In the server, the keys say what can be done, a terminal the server
// does not hold is faint, and a note takes the bottom row until a key.
func TestTheWatchInsideTheServer(t *testing.T) {
	w := composeWatch(watch(testProcs, 501, testRoots, testIsProject, nil), map[string]pane{"ttys007": {id: "%3"}}, "ttys007", testProjRoots, "/Users/w0zro", watchNow, "")
	w.inside = true
	rows := drawWatch(w, 67040, 120, 40, colored())
	text := texts(rows)
	p := colored()
	// ttys005 is in no pane the server holds here, and is the cursor's
	// row besides, so it reads at gray rather than faint; ttys007 is in
	// a pane, so it reads at the plain gray of a row conn can reach.
	if !strings.Contains(text, p.gray+"TTYS005") || !strings.Contains(text, p.gray+"TTYS007") {
		t.Errorf("the terminals are not colored by reach:\n%s", text)
	}
	// The slot's mark is on the kind of its head, which is the shell,
	// not the agent under it.
	if !strings.Contains(text, p.orange+p.bold+"SHELL") {
		t.Errorf("the slot's head is not marked:\n%s", text)
	}
	// In the rail there is no terminal column, and the rows close up.
	railText := texts(drawWatch(w, 67040, 48, 30, plain))
	if strings.Contains(railText, "TTY") || !strings.Contains(railText, "AGENT  claude --resume") {
		t.Errorf("the rail:\n%s", railText)
	}
	for _, r := range drawWatch(w, 67040, 48, 30, plain) {
		if w := utf8.RuneCountInString(r.text); w > 48 {
			t.Errorf("rail row is %d wide: %q", w, r.text)
		}
	}
}

// Enter reaches the cursor's process when its terminal is a pane of the
// server, n opens a shell at its place, and q detaches; each says why
// when it cannot. Outside the server q closes conn.
func TestKeysInsideTheServer(t *testing.T) {
	m := model{p: plain, width: 120, height: 40, view: viewWatch, uid: 501, roots: testRoots, now: watchNow, srv: &server{tmux: "/nonexistent/tmux", socket: "/tmp/none"}, inside: true}
	next, _ := m.Update(watchMsg{places: watch(testProcs, 501, testRoots, testIsProject, nil), panes: map[string]pane{"ttys007": {id: "%3", tty: "ttys007"}}})
	m = next.(model)
	press := func(k string, code rune) tea.Cmd {
		next, cmd := m.Update(tea.KeyPressMsg{Code: code, Text: k})
		m = next.(model)
		return cmd
	}
	// The cursor starts on home's shell, whose terminal the server does
	// not hold. The note is the whole of the answer — conn asks the
	// server for nothing but the putting of it on the bar.
	if press("enter", tea.KeyEnter); m.note != "NOT IN A PANE OF CONN'S SERVER" {
		t.Errorf("enter on a process outside the server: %q", m.note)
	}
	// Down to the conjurer's tree, whose terminal is a pane of the
	// server: enter reaches it.
	for range 4 {
		press("j", 'j')
	}
	if cmd := press("enter", tea.KeyEnter); cmd == nil || m.note != "" {
		t.Errorf("enter on a process in the server should reach it: %q", m.note)
	} else if n, ok := cmd().(noteMsg); !ok || !strings.Contains(n.note, "TMUX") {
		t.Errorf("a server that is not there should be said on the bottom row: %+v", n)
	}
	if cmd := press("s", 's'); cmd == nil {
		t.Error("s should open a shell at the place")
	}
	if cmd := press("q", 'q'); cmd == nil {
		t.Error("q should detach")
	} else if _, quit := cmd().(tea.QuitMsg); quit {
		t.Error("q inside the server should not close conn")
	}
	m.inside = false
	if cmd := press("enter", tea.KeyEnter); cmd != nil || !strings.Contains(m.note, "OUTSIDE") {
		t.Errorf("enter outside the server: %q", m.note)
	}
	if cmd := press("q", 'q'); cmd == nil {
		t.Error("q outside the server should close conn")
	} else if _, quit := cmd().(tea.QuitMsg); !quit {
		t.Error("q outside the server should close conn")
	}
}

// The watch says no keys. They are learned once; a legend on every row
// of every reading is a thing to read past forever. Nor does it keep a
// row back for a note: what conn has to say is on the bar, and the foot
// is the list's like every other row.
func TestTheWatchSaysNoKeys(t *testing.T) {
	w := composeWatch(watch(testProcs, 501, testRoots, testIsProject, nil), map[string]pane{"ttys007": {id: "%3"}}, "ttys007", testProjRoots, "/Users/w0zro", watchNow, "")
	for _, inside := range []bool{false, true} {
		w.inside = inside
		for _, size := range [][2]int{{120, 40}, {48, 30}, {100, 9}, {0, 0}} {
			text := stripEscapes(texts(drawWatch(w, 67040, size[0], size[1], plain)))
			for _, key := range []string{"MOVE", "REACHES", "OPENS", "DETACHES", "CLOSES", "CONSOLE"} {
				if strings.Contains(text, key) {
					t.Errorf("inside=%v at %dx%d the watch still says %q:\n%s", inside, size[0], size[1], key, text)
				}
			}
		}
	}
	// Nine rows of a watch that will not fit are nine rows of it: the
	// foot carries the count of what is out of view, not a blank kept
	// against a note that is no longer drawn here.
	rows := drawWatch(w, 70301, 100, 9, plain)
	if len(rows) != 9 || !strings.Contains(rows[8].text, "ABOVE") {
		t.Errorf("the foot is not the list's: %q", rows[8].text)
	}
}

// The cursor is a ground, not a mark: its row is drawn on the selection
// color from edge to edge, and no other row is. Where there is no color
// to raise — a pipe, a golden file — the row takes a mark instead, so
// the record still says which one it is.
func TestTheCursorIsAGround(t *testing.T) {
	p := colored()
	rows := drawWatch(testWatch(), 67040, 120, 40, p)
	on := 0
	for _, r := range rows {
		if strings.Contains(r.text, p.selection) {
			on++
			// The slot's shell is a level in, under the rail's own.
			if !strings.HasPrefix(stripEscapes(r.text), "     SHELL   zsh") {
				t.Errorf("the raised row is not the cursor's: %q", stripEscapes(r.text))
			}
			// Raised from edge to edge: the row never falls back to the
			// ground partway along.
			if strings.Contains(r.text, p.ground) {
				t.Errorf("the raised row falls back to the ground: %q", r.text)
			}
		}
	}
	if on != 1 {
		t.Errorf("%d rows are raised; one should be", on)
	}
	if strings.Contains(texts(rows), "▸") {
		t.Error("the cursor is still a mark where it has a ground")
	}
	// In plain text there is no ground to raise, so the mark stays.
	plainRows := texts(drawWatch(testWatch(), 67040, 120, 40, plain))
	if !strings.Contains(plainRows, "▸   SHELL   zsh") {
		t.Errorf("the plain watch lost its cursor:\n%s", plainRows)
	}
}

// Three tiers, by what conn can do with a row: what is in the slot is
// the orange, what conn holds a pane for is the ink, and what it can
// only report is a rank down — the whole row of it, not the command
// alone. Outside the server conn holds nothing, and dims nothing: the
// distinction would be every row.
func TestTheRowsReadByWhatConnCanDoWithThem(t *testing.T) {
	p := colored()
	held := composeWatch(watch(testProcs, 501, testRoots, testIsProject, nil),
		map[string]pane{"ttys005": {id: "%0"}, "ttys007": {id: "%3"}}, "ttys007",
		testProjRoots, "/Users/w0zro", watchNow, "")
	held.inside = true
	// The cursor is on a row conn holds a pane for, away from the rows
	// under test, so none of them is giving up a rank of dimming to be
	// read.
	text := texts(drawWatch(held, 67040, 120, 40, p))

	// The slot is a mark: the kind of the head of what is in it, and
	// nothing else. Not its command, not its terminal, not its status —
	// a row is a lot of orange, and the status column is a color of its
	// own already.
	if !strings.Contains(text, p.orange+p.bold+"SHELL") {
		t.Errorf("the slot's head is not marked:\n%s", text)
	}
	for _, notIn := range []string{p.orange + "zsh", p.orange + "TTYS007", p.orange + "ACTIVE", p.orange + "2H 00M"} {
		if strings.Contains(text, notIn) {
			t.Errorf("the orange ran past the kind: %q\n%s", notIn, text)
		}
	}
	// What hangs under the head is in the same pane and just as much in
	// the slot; it reads as the other true thing about it, which is that
	// conn holds a pane for it.
	if !strings.Contains(text, p.ink+"claude --resume") {
		t.Errorf("what hangs under the slot's head is not in the ink:\n%s", text)
	}
	// In nobody's pane: a rank down, and every column of it.
	for _, in := range []string{p.faint + "vim notes.md", p.faint + "TTYS009", p.faint + "1D 01H"} {
		if !strings.Contains(text, in) {
			t.Errorf("what conn cannot reach is not dimmed: %q missing\n%s", in, text)
		}
	}
	if strings.Contains(text, p.ink+"vim notes.md") {
		t.Error("what conn cannot reach is written in the ink")
	}
	// The row under the cursor gives a rank of the dimming back rather
	// than the reading: faint on the raised ground is barely there.
	onIt := texts(drawWatch(held, 80002, 120, 40, p))
	if !strings.Contains(onIt, p.gray+p.bold+"vim notes.md") {
		t.Errorf("the dimmed row under the cursor is not read back up:\n%s", onIt)
	}
	// Outside the server, every command is the ink: conn can reach none
	// of them, so dimming would say nothing.
	out := texts(drawWatch(testWatch(), 67040, 120, 40, p))
	for _, in := range []string{p.ink + p.bold + "zsh", p.ink + "claude --resume", p.ink + "vim notes.md"} {
		if !strings.Contains(out, in) {
			t.Errorf("outside the server a command is not in the ink:\n%s", out)
		}
	}
}

// The rail is conn's width, not the terminal's. Inside the server, off
// the console, conn draws to railWidth rather than to whatever the pane
// happens to be at the moment — it holds tmux to that width anyway, and
// drawing to it means the frame conn paints is already the shape the
// pane is about to be, so the split that opens the slot has nothing to
// reflow.
func TestTheRailDrawsToItsOwnWidth(t *testing.T) {
	m := model{p: plain, width: 140, height: 40, inside: true, view: viewWatch}
	if got := m.cols(); got != railWidth {
		t.Errorf("the rail drew to %d columns, not the rail's %d", got, railWidth)
	}
	// The console is the whole window, and takes the width it is given.
	m.view = viewConsole
	if got := m.cols(); got != 140 {
		t.Errorf("the console drew to %d columns, not the window's 140", got)
	}
	// Outside the server there is no slot to leave room for.
	m.view, m.inside = viewWatch, false
	if got := m.cols(); got != 140 {
		t.Errorf("outside the server the watch drew to %d columns", got)
	}
	// A window narrower than the rail is still the whole of what there
	// is to draw in.
	m.inside, m.width = true, 30
	if got := m.cols(); got != 30 {
		t.Errorf("a 30-column window drew to %d", got)
	}
}

// A place is named by what is left of its path once the root the
// checkouts are kept under is taken off it. The root is the same for
// every project and says nothing that tells one from another, and it
// was said at the head of every block on a rail forty-four columns
// wide.
func TestAPlaceIsNamedByWhatTellsItApart(t *testing.T) {
	roots := []string{"/Users/w0zro/projects", "/srv/work"}
	for _, c := range []struct{ path, want string }{
		{"/Users/w0zro/projects/w0zro/conn", "w0zro/conn"},
		{"/Users/w0zro/projects/w0zro/vim.pro/conjurer", "w0zro/vim.pro/conjurer"},
		{"/srv/work/api", "api"},
		// A root itself has nothing left of it after itself, and is
		// written from ~ like anywhere else with nothing to take off.
		{"/Users/w0zro/projects", "~/projects"},
		// Outside every root, where it is is the whole of what the line
		// has to say.
		{"/Users/w0zro", "~"},
		{"/private/tmp/scratch", "/private/tmp/scratch"},
	} {
		if got := placeName(c.path, roots, "/Users/w0zro"); got != c.want {
			t.Errorf("%s is called %q, want %q", c.path, got, c.want)
		}
	}
	// With no roots at all nothing is taken off anything.
	if got := placeName("/Users/w0zro/projects/w0zro/conn", nil, "/Users/w0zro"); got != "~/projects/w0zro/conn" {
		t.Errorf("with no roots the place is called %q", got)
	}
}

// The one word on the watch that asks something of you blinks, which is
// the one thing on a screen that reaches the corner of an eye: reading
// down a list of rows that all say something, the row that wants you is
// the row that moves. On the dark half its cells are the ground and
// nothing around them moves — a word that jumped its neighbours about
// would be worse than one that never blinked.
func TestTheWaitingWordBlinks(t *testing.T) {
	held := []place{{path: "/w", entries: []entry{
		{pid: 11, kind: kindShell, command: "zsh", status: statusActive},
		{pid: 12, kind: kindAgent, command: "claude", status: statusWaiting, depth: 1, since: watchNow.Add(-time.Minute)},
	}}}
	b := composeWatch(held, nil, "", testProjRoots, "/Users/w0zro", watchNow, "")

	b.lit = true
	on := texts(drawWatch(b, 0, 60, 12, plain))
	if !strings.Contains(on, statusWaiting) {
		t.Errorf("the lit half has no word:\n%s", on)
	}
	b.lit = false
	off := texts(drawWatch(b, 0, 60, 12, plain))
	if strings.Contains(off, statusWaiting) {
		t.Errorf("the dark half still says it:\n%s", off)
	}
	// Only the word goes. Every row is the same shape on both halves, so
	// nothing around it moves.
	b.lit = true
	onRows := drawWatch(b, 0, 60, 12, plain)
	b.lit = false
	offRows := drawWatch(b, 0, 60, 12, plain)
	if len(onRows) != len(offRows) {
		t.Fatalf("the halves are %d rows and %d", len(onRows), len(offRows))
	}
	for i := range onRows {
		if lit, dark := onRows[i].text, offRows[i].text; lit != dark && !strings.Contains(lit, statusWaiting) {
			t.Errorf("row %d moved between the halves:\n%q\n%q", i, lit, dark)
		}
	}
	// What is merely active does not blink, and neither does a fault: a
	// process you suspended yourself is not asking anything of you.
	steady := composeWatch([]place{{path: "/w", entries: []entry{
		{pid: 21, kind: kindEditor, command: "vim", status: statusStopped, fault: true},
	}}}, nil, "", testProjRoots, "/Users/w0zro", watchNow, "")
	steady.lit = false
	if !strings.Contains(texts(drawWatch(steady, 0, 60, 12, plain)), statusStopped) {
		t.Error("a fault went dark with the blink")
	}
}

// The blink runs while something annunciates and stops when nothing
// does, so a watch with nothing held up on it is not redrawn a second
// and a half at a time for nothing.
func TestTheBlinkRunsOnlyForWhatAnnunciates(t *testing.T) {
	m := newModel(plain)
	if !m.annunciating() {
		t.Error("the console does not annunciate")
	}
	m.view = viewWatch
	m.places = []place{{path: "/w", entries: []entry{{pid: 11, status: statusActive}}}}
	if m.annunciating() {
		t.Error("a watch with nothing waiting annunciates")
	}
	m.places[0].entries = append(m.places[0].entries, entry{pid: 12, status: statusWaiting, since: watchNow})
	if !m.annunciating() {
		t.Error("a row waiting on you does not annunciate")
	}
	// Coming to it starts the tick; going off it stops the tick and
	// leaves the word lit, which is where anything not blinking rests.
	m.ticking, m.lit = false, false
	next, cmd := m.blinked()
	if !next.ticking || !next.lit || cmd == nil {
		t.Errorf("the blink did not start: ticking %v lit %v cmd %v", next.ticking, next.lit, cmd != nil)
	}
	if _, again := next.blinked(); again != nil {
		t.Error("the blink was started twice over")
	}
	next.places = nil
	stopped, cmd := next.blinked()
	if stopped.ticking || !stopped.lit || cmd != nil {
		t.Errorf("the blink did not stop: ticking %v lit %v", stopped.ticking, stopped.lit)
	}
}

// The prefix twice over goes to the process that was in the slot before
// the one in it now, and takes the one in it now as the one to come back
// to — so pressed twice it is where it started. conn's own furniture is
// not somewhere you were working: a hold standing in an empty slot and
// the look are not remembered, and going back never lands on one.
func TestTheOtherProcessIsTheOneYouWereLastIn(t *testing.T) {
	m := newModel(plain)
	m.view, m.inside, m.now = viewWatch, true, watchNow
	m.srv = &server{tmux: "/nonexistent/tmux", socket: "/tmp/none"}
	m.panes = map[string]pane{
		"ttys001": {id: "%1", tty: "ttys001"},
		"ttys002": {id: "%2", tty: "ttys002"},
		"ttys009": {id: "%9", tty: "ttys009", hold: true},
	}
	other := func(m model) (model, tea.Cmd) {
		next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Mod: tea.ModAlt, Code: 'o'}))
		return next.(model), cmd
	}

	// Nothing has been in the slot yet, so there is nowhere to go back
	// to. The note is the whole of the answer — the only command it is
	// worth is the one that puts it on the bar.
	m, _ = other(m)
	if m.note != "NO OTHER PROCESS TO GO BACK TO" {
		t.Errorf("with nothing behind it: %q", m.note)
	}

	// A hold in the slot, then a process: the hold is not remembered.
	m.note = ""
	m.slot = "ttys009"
	next, _ := m.Update(reachedMsg{"ttys001"})
	m = next.(model)
	if m.lastSlot != "" {
		t.Errorf("the hold was remembered as somewhere to go back to: %q", m.lastSlot)
	}

	// A second process: the first is where going back leads.
	next, _ = m.Update(reachedMsg{"ttys002"})
	m = next.(model)
	if m.slot != "ttys002" || m.lastSlot != "ttys001" {
		t.Errorf("slot %q, other %q", m.slot, m.lastSlot)
	}
	m, cmd := other(m)
	if cmd == nil {
		t.Fatal("going back to the other process asked the server for nothing")
	}
	// Reaching answers with the terminal it put in the slot, and that
	// swaps which is which: pressed again it is back where it started.
	next, _ = m.Update(reachedMsg{"ttys001"})
	m = next.(model)
	if m.slot != "ttys001" || m.lastSlot != "ttys002" {
		t.Errorf("after going back: slot %q, other %q", m.slot, m.lastSlot)
	}

	// A process that has gone is not somewhere to go back to.
	gone := m
	gone.panes = map[string]pane{"ttys001": {id: "%1", tty: "ttys001"}}
	gone.note = ""
	gone, _ = other(gone)
	if gone.note != "NO OTHER PROCESS TO GO BACK TO" {
		t.Errorf("a pane that has gone: %q", gone.note)
	}

	// Outside the server nothing can be reached at all.
	out := m
	out.inside = false
	out.note = ""
	out, _ = other(out)
	if out.note != "NOTHING CAN BE REACHED OUTSIDE CONN'S TMUX SERVER" {
		t.Errorf("outside the server: %q", out.note)
	}
}
