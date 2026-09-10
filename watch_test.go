package main

import (
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

func testWatch() watchReport {
	return composeWatch(watch(testProcs, 67032, 501, testRoots), nil, "", "/Users/w0zro", watchNow, "w0zro@station", zulu(watchNow), "")
}

// The watch at 120 by 40 is a file of record, as are the empty watch and
// the one that could not be read.
func TestWatchMatchesTheGolden(t *testing.T) {
	golden(t, "watch-120x40.txt", texts(drawWatch(testWatch(), 67032, 120, 40, plain)))
	golden(t, "watch-cursor-100x9.txt", texts(drawWatch(testWatch(), 80002, 100, 9, plain)))
	empty := composeWatch(nil, nil, "", "/Users/w0zro", watchNow, "w0zro@station", zulu(watchNow), "")
	golden(t, "watch-empty-80x24.txt", texts(drawWatch(empty, 0, 80, 24, plain)))
	failed := composeWatch(nil, nil, "", "/Users/w0zro", watchNow, "w0zro@station", zulu(watchNow), "the process table could not be read: lsof: not found")
	golden(t, "watch-unread-80x24.txt", texts(drawWatch(failed, 0, 80, 24, plain)))
	rail := composeWatch(watch(testProcs, 67032, 501, testRoots), map[string]pane{"ttys004": {id: "%0"}, "ttys007": {id: "%3"}}, "ttys007", "/Users/w0zro", watchNow, "w0zro@station", zulu(watchNow), "")
	rail.inside = true
	golden(t, "watch-rail-48x30.txt", texts(drawWatch(rail, 70100, 48, 30, plain)))
}

// The watch's columns hold: the status flush right, the kind at the
// margin, the path from ~, the ages as of the clock, no legend on the
// bottom row, no row past the width.
func TestWatchLaysOut(t *testing.T) {
	rows := drawWatch(testWatch(), 70100, 120, 40, plain)
	text := texts(rows)
	measure, _, _ := columns(120)
	for _, s := range []string{
		"CONN ", "W0ZRO@STATION  ·  09-SEP-2026  03:00:00 Z",
		"KIND    COMMAND", "TTY", "AGE", "STATUS",
		"~/projects/w0zro/conn", "1 PROCESS", "CONN    conn", "TTYS004", "1M 30S", "HERE",
		"~/projects/w0zro/vim.pro/conjurer", "AGENT   claude --resume", "47M 00S", "ACTIVE",
		"~", "EDITOR  vim notes.md", "1D 01H", " STOPPED", " ▸ AGENT   claude --resume",
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
	rows := drawWatch(testWatch(), 67032, 100, 9, plain)
	text := texts(rows)
	if len(rows) != 9 || !strings.Contains(text, "… 6 BELOW") || strings.Contains(text, "ABOVE") || !strings.Contains(text, "▸ CONN") {
		t.Errorf("at 100x9 with the cursor on the first row:\n%s", text)
	}
	rows = drawWatch(testWatch(), 80002, 100, 9, plain)
	text = texts(rows)
	if len(rows) != 9 || !strings.Contains(text, "… 6 ABOVE") || strings.Contains(text, "BELOW") || !strings.Contains(text, "▸ EDITOR") {
		t.Errorf("at 100x9 with the cursor on the last row:\n%s", text)
	}
	if piped := drawWatch(testWatch(), 67032, 0, 0, plain); strings.Contains(texts(piped), "ABOVE") {
		t.Errorf("off a terminal:\n%s", texts(piped))
	}
}

// The cursor moves with j and k, stays within the rows, and follows its
// process across readings; when the process goes it holds its row.
func TestTheCursorFollowsItsProcess(t *testing.T) {
	m := model{p: plain, width: 120, height: 40, view: viewWatch, pid: 67032, uid: 501, roots: testRoots, now: watchNow}
	next, _ := m.Update(watchMsg{places: watch(testProcs, 67032, 501, testRoots)})
	m = next.(model)
	if m.cursor != 67032 {
		t.Errorf("the cursor should start on the first row, not %d", m.cursor)
	}
	press := func(k string) {
		next, _ := m.Update(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
		m = next.(model)
	}
	press("j")
	press("j")
	press("j")
	if m.cursor != 80002 || m.cursorAt != 2 {
		t.Errorf("after three j the cursor is on %d at %d", m.cursor, m.cursorAt)
	}
	press("k")
	if m.cursor != 70100 {
		t.Errorf("after k the cursor is on %d", m.cursor)
	}
	// claude gone: the go test, node and idle shell take its place; the
	// cursor keeps its row.
	var without []process
	for _, p := range testProcs {
		if p.pid != 70100 {
			without = append(without, p)
		}
	}
	next, _ = m.Update(watchMsg{places: watch(without, 67032, 501, testRoots)})
	m = next.(model)
	if m.cursor != 70212 || m.cursorAt != 1 {
		t.Errorf("with its process gone the cursor is on %d at %d", m.cursor, m.cursorAt)
	}
	next, _ = m.Update(watchMsg{places: watch(testProcs, 67032, 501, testRoots)})
	m = next.(model)
	if m.cursor != 70100 {
		t.Errorf("the cursor did not follow a pid that is back: %d", m.cursor)
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
	m := model{head: station{build: testStation.build, session: testStation.session}, now: watchNow, p: plain, width: 120, height: 40, pid: 67032, uid: 501, roots: testRoots}
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
	next, cmd = m.Update(watchMsg{places: watch(testProcs, 67032, 501, testRoots), gen: m.watchGen})
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
	w := composeWatch(watch(testProcs, 67032, 501, testRoots), map[string]pane{"ttys007": {id: "%3"}}, "ttys007", "/Users/w0zro", watchNow, "w0zro@station", zulu(watchNow), "")
	w.inside = true
	rows := drawWatch(w, 67032, 120, 40, colored())
	text := texts(rows)
	p := colored()
	if !strings.Contains(text, p.faint+"TTYS004") || !strings.Contains(text, p.gray+"TTYS007") {
		t.Errorf("the terminals are not colored by reach:\n%s", text)
	}
	if !strings.Contains(text, p.orange+p.bold+"AGENT") || !strings.Contains(text, p.orange+p.bold+"ACTIVE") {
		t.Errorf("the row in the slot is not in orange:\n%s", text)
	}
	// In the rail there is no terminal column, and the rows close up.
	railText := texts(drawWatch(w, 67032, 48, 30, plain))
	if strings.Contains(railText, "TTY") || !strings.Contains(railText, "AGENT  claude --resume") {
		t.Errorf("the rail:\n%s", railText)
	}
	for _, r := range drawWatch(w, 67032, 48, 30, plain) {
		if w := utf8.RuneCountInString(r.text); w > 48 {
			t.Errorf("rail row is %d wide: %q", w, r.text)
		}
	}
	w.note = "NOT IN A PANE OF CONN'S SERVER"
	rows = drawWatch(w, 67032, 120, 40, plain)
	if !strings.Contains(rows[39].text, w.note) {
		t.Errorf("the note is not on the bottom row:\n%s", texts(rows))
	}
}

// Enter reaches the cursor's process when its terminal is a pane of the
// server, n opens a shell at its place, and q detaches; each says why
// when it cannot. Outside the server q closes conn.
func TestKeysInsideTheServer(t *testing.T) {
	m := model{p: plain, width: 120, height: 40, view: viewWatch, pid: 67032, uid: 501, roots: testRoots, now: watchNow, srv: &server{tmux: "/nonexistent/tmux", socket: "/tmp/none"}, inside: true}
	next, _ := m.Update(watchMsg{places: watch(testProcs, 67032, 501, testRoots), panes: map[string]pane{"ttys007": {id: "%3", tty: "ttys007"}}})
	m = next.(model)
	press := func(k string, code rune) tea.Cmd {
		next, cmd := m.Update(tea.KeyPressMsg{Code: code, Text: k})
		m = next.(model)
		return cmd
	}
	if cmd := press("enter", tea.KeyEnter); cmd != nil || m.note != "NOT IN A PANE OF CONN'S SERVER" {
		t.Errorf("enter on a process outside the server: %q", m.note)
	}
	press("j", 'j')
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
// of every reading is a thing to read past forever. The bottom row is
// kept clear all the same, so a note has a place to land that does not
// move the rows.
func TestTheWatchSaysNoKeys(t *testing.T) {
	w := composeWatch(watch(testProcs, 67032, 501, testRoots), map[string]pane{"ttys007": {id: "%3"}}, "ttys007", "/Users/w0zro", watchNow, "w0zro@station", zulu(watchNow), "")
	for _, inside := range []bool{false, true} {
		w.inside = inside
		for _, size := range [][2]int{{120, 40}, {48, 30}, {100, 9}, {0, 0}} {
			text := stripEscapes(texts(drawWatch(w, 67032, size[0], size[1], plain)))
			for _, key := range []string{"MOVE", "REACHES", "OPENS", "DETACHES", "CLOSES", "CONSOLE"} {
				if strings.Contains(text, key) {
					t.Errorf("inside=%v at %dx%d the watch still says %q:\n%s", inside, size[0], size[1], key, text)
				}
			}
		}
	}
	rows := drawWatch(w, 67032, 120, 40, plain)
	if len(rows) != 40 || strings.TrimSpace(rows[39].text) != "" {
		t.Errorf("the bottom row is not kept clear: %q", rows[39].text)
	}
	w.note = "NOTHING UNDER THE CURSOR"
	rows = drawWatch(w, 67032, 120, 40, plain)
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
	rows := drawWatch(testWatch(), 67032, 120, 40, p)
	on := 0
	for _, r := range rows {
		if strings.Contains(r.text, p.selection) {
			on++
			if !strings.HasPrefix(stripEscapes(r.text), "   CONN    conn") {
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
	plainRows := texts(drawWatch(testWatch(), 67032, 120, 40, plain))
	if !strings.Contains(plainRows, "▸ CONN    conn") {
		t.Errorf("the plain watch lost its cursor:\n%s", plainRows)
	}
}

// What conn holds is written in the ink and what it can only report is
// dimmed a rank, so the rows it can act on read as its own. Outside the
// server conn holds nothing, and dims nothing: the distinction would be
// every row.
func TestWhatConnHoldsIsInTheInk(t *testing.T) {
	p := colored()
	held := composeWatch(watch(testProcs, 67032, 501, testRoots),
		map[string]pane{"ttys004": {id: "%0"}, "ttys007": {id: "%3"}}, "ttys007",
		"/Users/w0zro", watchNow, "w0zro@station", zulu(watchNow), "")
	held.inside = true
	text := texts(drawWatch(held, 67032, 120, 40, p))
	// conn's own, which is the cursor's row and so bold as well, and the
	// agent in a pane of the server: both in the ink.
	if !strings.Contains(text, p.ink+p.bold+"conn") {
		t.Errorf("conn's own row is not in the ink:\n%s", text)
	}
	if !strings.Contains(text, p.ink+"claude --resume") {
		t.Errorf("the agent conn holds is not in the ink:\n%s", text)
	}
	// The editor is in nobody's pane: a rank down.
	if !strings.Contains(text, p.gray+"vim notes.md") {
		t.Errorf("what conn cannot reach is not dimmed:\n%s", text)
	}
	if strings.Contains(text, p.ink+"vim notes.md") {
		t.Error("what conn cannot reach is written in the ink")
	}
	// Outside the server, every command is the ink: conn can reach none
	// of them, so dimming would say nothing.
	out := texts(drawWatch(testWatch(), 67032, 120, 40, p))
	for _, in := range []string{p.ink + p.bold + "conn", p.ink + "claude --resume", p.ink + "vim notes.md"} {
		if !strings.Contains(out, in) {
			t.Errorf("outside the server a command is not in the ink:\n%s", out)
		}
	}
}
