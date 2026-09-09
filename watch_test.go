package main

import (
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

func testWatch() watchReport {
	return composeWatch(watch(testProcs, 67032, 501, testRoots), "/Users/w0zro", watchNow, "w0zro@station", zulu(watchNow), "")
}

// The watch at 120 by 40 is a file of record, as are the empty watch and
// the one that could not be read.
func TestWatchMatchesTheGolden(t *testing.T) {
	golden(t, "watch-120x40.txt", texts(drawWatch(testWatch(), 120, 40, plain)))
	empty := composeWatch(nil, "/Users/w0zro", watchNow, "w0zro@station", zulu(watchNow), "")
	golden(t, "watch-empty-80x24.txt", texts(drawWatch(empty, 80, 24, plain)))
	failed := composeWatch(nil, "/Users/w0zro", watchNow, "w0zro@station", zulu(watchNow), "the process table could not be read: lsof: not found")
	golden(t, "watch-unread-80x24.txt", texts(drawWatch(failed, 80, 24, plain)))
}

// The watch's columns hold: the status flush right, the kind at the
// margin, the path from ~, the ages as of the clock, the keys on the
// bottom row, no row past the width.
func TestWatchLaysOut(t *testing.T) {
	rows := drawWatch(testWatch(), 120, 40, plain)
	text := texts(rows)
	measure, _, _ := columns(120)
	for _, s := range []string{
		"CONN  WATCH", "W0ZRO@STATION  ·  09-SEP-2026  03:00:00 Z",
		"KIND    COMMAND", "TTY", "AGE", "STATUS",
		"~/projects/w0zro/conn", "1 PROCESS", "CONN    conn", "TTYS004", "1M 30S", "HERE",
		"~/projects/w0zro/vim.pro/conjurer", "AGENT   claude --resume", "47M 00S", "ACTIVE",
		"~", "EDITOR  vim notes.md", "1D 01H", " STOPPED", watchKey,
	} {
		if !strings.Contains(text, s) {
			t.Errorf("watch lacks %q:\n%s", s, text)
		}
	}
	if len(rows) != 40 || !strings.Contains(rows[39].text, watchKey) {
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
	for i, r := range drawWatch(testWatch(), 120, 40, colored()) {
		if w := utf8.RuneCountInString(stripEscapes(r.text)); w != 120 {
			t.Errorf("colored row %d paints %d columns", i, w)
		}
	}
}

// A watch taller than the terminal says how many rows are out of view,
// and still ends on the keys.
func TestAWatchThatWillNotFitSaysSo(t *testing.T) {
	rows := drawWatch(testWatch(), 100, 9, plain)
	text := texts(rows)
	if len(rows) != 9 || !strings.Contains(text, "MORE ROWS") || !strings.Contains(rows[8].text, watchKey) {
		t.Errorf("at 100x9:\n%s", text)
	}
	if piped := drawWatch(testWatch(), 0, 0, plain); strings.Contains(texts(piped), "MORE ROWS") || strings.Contains(texts(piped), watchKey) {
		t.Errorf("off a terminal:\n%s", texts(piped))
	}
}

// A key at the end of the console goes to the watch, which reads the
// table and reads it again on its tick; c brings the console back, and a
// stale tick is dropped.
func TestTheKeyContinuesToTheWatch(t *testing.T) {
	m := model{head: station{build: testStation.build, session: testStation.session}, now: watchNow, p: plain, width: 120, height: 40, self: 67032, uid: 501, roots: testRoots}
	st := testStation
	m.st = &st
	m.stage = lastStage(m.report())
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = next.(model)
	if m.view != viewWatch || cmd == nil {
		t.Fatalf("a key at the end should go to the watch and read it")
	}
	if !strings.Contains(m.View().Content, "WATCH") || !strings.Contains(m.View().Content, "NOTHING ON WATCH") {
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
