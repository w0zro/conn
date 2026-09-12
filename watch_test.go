package main

import (
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

func testWatch() watchReport {
	return composeWatch(watch(testProcs, 501, testRoots, nil), nil, "", "/Users/w0zro", watchNow, "w0zro@station", zulu(watchNow), "")
}

// The watch at 120 by 40 is a file of record, as are the empty watch and
// the one that could not be read.
func TestWatchMatchesTheGolden(t *testing.T) {
	golden(t, "watch-120x40.txt", texts(drawWatch(testWatch(), 67040, 120, 40, plain)))
	golden(t, "watch-cursor-100x9.txt", texts(drawWatch(testWatch(), 80002, 100, 9, plain)))
	empty := composeWatch(nil, nil, "", "/Users/w0zro", watchNow, "w0zro@station", zulu(watchNow), "")
	golden(t, "watch-empty-80x24.txt", texts(drawWatch(empty, 0, 80, 24, plain)))
	failed := composeWatch(nil, nil, "", "/Users/w0zro", watchNow, "w0zro@station", zulu(watchNow), "the process table could not be read: lsof: not found")
	golden(t, "watch-unread-80x24.txt", texts(drawWatch(failed, 0, 80, 24, plain)))
	rail := composeWatch(watch(testProcs, 501, testRoots, nil), map[string]pane{"ttys005": {id: "%0"}, "ttys007": {id: "%3"}}, "ttys007", "/Users/w0zro", watchNow, "w0zro@station", zulu(watchNow), "")
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
		"CONN ", "W0ZRO@STATION  ·  09-SEP-2026  03:00:00 Z",
		"KIND    COMMAND", "TTY", "AGE", "STATUS",
		"~/projects/w0zro/conn", "2 PROCESSES", "SHELL   zsh", "TTYS005", "1M 30S", "IDLE",
		"~/projects/w0zro/vim.pro/conjurer", "47M 00S", "ACTIVE",
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
// and says how many rows are above and below.
func TestAWatchThatWillNotFitScrolls(t *testing.T) {
	rows := drawWatch(testWatch(), 70001, 100, 9, plain)
	text := texts(rows)
	if len(rows) != 9 || !strings.Contains(text, "… 12 BELOW") || strings.Contains(text, "ABOVE") || !strings.Contains(text, "▸ SHELL") {
		t.Errorf("at 100x9 with the cursor on the first row:\n%s", text)
	}
	rows = drawWatch(testWatch(), 80002, 100, 9, plain)
	text = texts(rows)
	// The cursor's mark keeps the margin; the row it marks still steps
	// in for the level it is at.
	if len(rows) != 9 || !strings.Contains(text, "… 12 ABOVE") || strings.Contains(text, "BELOW") || !strings.Contains(text, "▸   EDITOR") {
		t.Errorf("at 100x9 with the cursor on the last row:\n%s", text)
	}
	if piped := drawWatch(testWatch(), 70001, 0, 0, plain); strings.Contains(texts(piped), "ABOVE") {
		t.Errorf("off a terminal:\n%s", texts(piped))
	}
}

// The cursor moves with j and k, stays within the rows, and follows its
// process across readings; when the process goes it holds its row.
func TestTheCursorFollowsItsProcess(t *testing.T) {
	m := model{p: plain, width: 120, height: 40, view: viewWatch, uid: 501, roots: testRoots, now: watchNow}
	next, _ := m.Update(watchMsg{places: watch(testProcs, 501, testRoots, nil)})
	m = next.(model)
	// The rows read down the tree: the conjurer's shell, the claude it
	// runs, the bash that one runs, its go, then the node beside the
	// bash.
	if m.cursor != 70001 {
		t.Errorf("the cursor should start on the first row, not %d", m.cursor)
	}
	press := func(k string) {
		next, _ := m.Update(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
		m = next.(model)
	}
	press("j")
	press("j")
	press("j")
	if m.cursor != 70301 || m.cursorAt != 3 {
		t.Errorf("after three j the cursor is on %d at %d", m.cursor, m.cursorAt)
	}
	press("k")
	press("k")
	if m.cursor != 70100 {
		t.Errorf("after two k the cursor is on %d", m.cursor)
	}
	// A reading that still has the pid keeps the cursor on it, wherever
	// in the rows it has moved to.
	next, _ = m.Update(watchMsg{places: watch(testProcs, 501, testRoots, nil)})
	m = next.(model)
	if m.cursor != 70100 || m.cursorAt != 1 {
		t.Errorf("the cursor left the pid it was on: %d at %d", m.cursor, m.cursorAt)
	}
	// claude gone: what it ran stands on its own, and the cursor, with
	// no pid of its own left to follow, holds the row it was at.
	var without []process
	for _, p := range testProcs {
		if p.pid != 70100 {
			without = append(without, p)
		}
	}
	next, _ = m.Update(watchMsg{places: watch(without, 501, testRoots, nil)})
	m = next.(model)
	if m.cursorAt != 1 || m.cursor != 70301 {
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
	if m.view != viewWatch || cmd == nil {
		t.Fatalf("a key at the end should go to the watch and read it")
	}
	if strings.Contains(m.View().Content, "CONN  WATCH") || !strings.Contains(m.View().Content, "NOTHING ON WATCH") {
		t.Errorf("the watch should be up, empty until read:\n%s", m.View().Content)
	}
	next, cmd = m.Update(watchMsg{places: watch(testProcs, 501, testRoots, nil), gen: m.watchGen})
	m = next.(model)
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
	next, cmd = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = next.(model)
	if m.view != viewWatch || cmd == nil || m.watchGen != 2 {
		t.Errorf("a key on the finished console should return to the watch and read it afresh: gen %d", m.watchGen)
	}
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"}); cmd == nil {
		t.Error("q should close conn from the watch")
	}
}

// In the server, the keys say what can be done, a terminal the server
// does not hold is faint, and a note takes the bottom row until a key.
func TestTheWatchInsideTheServer(t *testing.T) {
	w := composeWatch(watch(testProcs, 501, testRoots, nil), map[string]pane{"ttys007": {id: "%3"}}, "ttys007", "/Users/w0zro", watchNow, "w0zro@station", zulu(watchNow), "")
	w.inside = true
	rows := drawWatch(w, 67040, 120, 40, colored())
	text := texts(rows)
	p := colored()
	// ttys005 is in no pane the server holds here, and is the cursor's
	// row besides, so it reads at gray rather than faint; the slot's
	// ttys007 is in the orange with the rest of its row.
	if !strings.Contains(text, p.gray+"TTYS005") || !strings.Contains(text, p.orange+"TTYS007") {
		t.Errorf("the terminals are not colored by reach:\n%s", text)
	}
	if !strings.Contains(text, p.orange+p.bold+"AGENT") || !strings.Contains(text, p.orange+p.bold+"ACTIVE") {
		t.Errorf("the row in the slot is not in orange:\n%s", text)
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
	w.note = "NOT IN A PANE OF CONN'S SERVER"
	rows = drawWatch(w, 67040, 120, 40, plain)
	if !strings.Contains(rows[39].text, w.note) {
		t.Errorf("the note is not on the bottom row:\n%s", texts(rows))
	}
}

// Enter reaches the cursor's process when its terminal is a pane of the
// server, n opens a shell at its place, and q detaches; each says why
// when it cannot. Outside the server q closes conn.
func TestKeysInsideTheServer(t *testing.T) {
	m := model{p: plain, width: 120, height: 40, view: viewWatch, uid: 501, roots: testRoots, now: watchNow, srv: &server{tmux: "/nonexistent/tmux", socket: "/tmp/none"}, inside: true}
	next, _ := m.Update(watchMsg{places: watch(testProcs, 501, testRoots, nil), panes: map[string]pane{"ttys007": {id: "%3", tty: "ttys007"}}})
	m = next.(model)
	press := func(k string, code rune) tea.Cmd {
		next, cmd := m.Update(tea.KeyPressMsg{Code: code, Text: k})
		m = next.(model)
		return cmd
	}
	// The cursor starts on the conjurer's shell, whose terminal is a
	// pane of the server: enter reaches it.
	if cmd := press("enter", tea.KeyEnter); cmd == nil || m.note != "" {
		t.Errorf("enter on a process in the server should reach it: %q", m.note)
	} else if n, ok := cmd().(noteMsg); !ok || !strings.Contains(n.note, "TMUX") {
		t.Errorf("a server that is not there should be said on the bottom row: %+v", n)
	}
	// Down past that tree to conn's own place, whose terminals the
	// server does not hold.
	for range 5 {
		press("j", 'j')
	}
	if cmd := press("enter", tea.KeyEnter); cmd != nil || m.note != "NOT IN A PANE OF CONN'S SERVER" {
		t.Errorf("enter on a process outside the server: %q", m.note)
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
// of every reading is a thing to read past forever. The bottom row is
// kept clear all the same, so a note has a place to land that does not
// move the rows.
func TestTheWatchSaysNoKeys(t *testing.T) {
	w := composeWatch(watch(testProcs, 501, testRoots, nil), map[string]pane{"ttys007": {id: "%3"}}, "ttys007", "/Users/w0zro", watchNow, "w0zro@station", zulu(watchNow), "")
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
	rows := drawWatch(w, 67040, 120, 40, plain)
	if len(rows) != 40 || strings.TrimSpace(rows[39].text) != "" {
		t.Errorf("the bottom row is not kept clear: %q", rows[39].text)
	}
	w.note = "NOTHING UNDER THE CURSOR"
	rows = drawWatch(w, 67040, 120, 40, plain)
	if len(rows) != 40 || !strings.Contains(rows[39].text, w.note) {
		t.Errorf("a note has no place to land: %q", rows[39].text)
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
	held := composeWatch(watch(testProcs, 501, testRoots, nil),
		map[string]pane{"ttys005": {id: "%0"}, "ttys007": {id: "%3"}}, "ttys007",
		"/Users/w0zro", watchNow, "w0zro@station", zulu(watchNow), "")
	held.inside = true
	// The cursor is on the slot's shell, away from the rows under test,
	// so none of them is giving up a rank of dimming to be read.
	text := texts(drawWatch(held, 70001, 120, 40, p))

	// In the slot: the whole row in the orange, terminal and age with
	// the rest of it, since what is in the slot is a pane and not one
	// process of it.
	for _, in := range []string{p.orange + "claude --resume", p.orange + "TTYS007", p.orange + p.bold + "AGENT"} {
		if !strings.Contains(text, in) {
			t.Errorf("the slot's row is not in the orange: %q missing\n%s", in, text)
		}
	}
	// In a pane conn holds, but not the slot: the ink.
	if !strings.Contains(text, p.ink+"zsh") {
		t.Errorf("the shell conn holds is not in the ink:\n%s", text)
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
