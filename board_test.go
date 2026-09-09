package main

import (
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

func testBoard() boardReport {
	return composeBoard(board(testProcs, 67032, 501, testRoots), "/Users/w0zro", boardNow, "w0zro@station", zulu(boardNow), "")
}

// The board at 120 by 40 is a file of record, as are the empty board and
// the one that could not be read.
func TestBoardMatchesTheGolden(t *testing.T) {
	golden(t, "board-120x40.txt", texts(drawBoard(testBoard(), 120, 40, plain)))
	empty := composeBoard(nil, "/Users/w0zro", boardNow, "w0zro@station", zulu(boardNow), "")
	golden(t, "board-empty-80x24.txt", texts(drawBoard(empty, 80, 24, plain)))
	failed := composeBoard(nil, "/Users/w0zro", boardNow, "w0zro@station", zulu(boardNow), "the process table could not be read: lsof: not found")
	golden(t, "board-unread-80x24.txt", texts(drawBoard(failed, 80, 24, plain)))
}

// The board's columns hold: the status flush right, the kind at the
// margin, the path from ~, the ages as of the clock, the keys on the
// bottom row, no row past the width.
func TestBoardLaysOut(t *testing.T) {
	rows := drawBoard(testBoard(), 120, 40, plain)
	text := texts(rows)
	measure, _, _ := columns(120)
	for _, s := range []string{
		"CONN  THE BOARD", "W0ZRO@STATION  ·  09-SEP-2026  03:00:00 Z",
		"KIND    COMMAND", "TTY", "AGE", "STATUS",
		"~/projects/w0zro/conn", "1 PROCESS", "CONN    conn", "TTYS004", "1M 30S", "HERE",
		"~/projects/w0zro/vim.pro/conjurer", "AGENT   claude --resume", "47M 00S", "ACTIVE",
		"~", "EDITOR  vim notes.md", "1D 01H", " STOPPED", boardKey,
	} {
		if !strings.Contains(text, s) {
			t.Errorf("board lacks %q:\n%s", s, text)
		}
	}
	if len(rows) != 40 || !strings.Contains(rows[39].text, boardKey) {
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
	for i, r := range drawBoard(testBoard(), 120, 40, colored()) {
		if w := utf8.RuneCountInString(stripEscapes(r.text)); w != 120 {
			t.Errorf("colored row %d paints %d columns", i, w)
		}
	}
}

// A board taller than the terminal says how many rows are out of view,
// and still ends on the keys.
func TestABoardThatWillNotFitSaysSo(t *testing.T) {
	rows := drawBoard(testBoard(), 100, 9, plain)
	text := texts(rows)
	if len(rows) != 9 || !strings.Contains(text, "MORE ROWS") || !strings.Contains(rows[8].text, boardKey) {
		t.Errorf("at 100x9:\n%s", text)
	}
	if piped := drawBoard(testBoard(), 0, 0, plain); strings.Contains(texts(piped), "MORE ROWS") || strings.Contains(texts(piped), boardKey) {
		t.Errorf("off a terminal:\n%s", texts(piped))
	}
}

// A key at the end of the console goes to the board, which reads the
// table and reads it again on its tick; c brings the console back, and a
// stale tick is dropped.
func TestTheKeyContinuesToTheBoard(t *testing.T) {
	m := model{head: station{build: testStation.build, session: testStation.session}, now: boardNow, p: plain, width: 120, height: 40, self: 67032, uid: 501, roots: testRoots}
	st := testStation
	m.st = &st
	m.stage = lastStage(m.report())
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = next.(model)
	if m.view != viewBoard || cmd == nil {
		t.Fatalf("a key at the end should go to the board and read it")
	}
	if !strings.Contains(m.View().Content, "THE BOARD") || !strings.Contains(m.View().Content, "NOTHING ON THE BOARD") {
		t.Errorf("the board should be up, empty until read:\n%s", m.View().Content)
	}
	next, cmd = m.Update(boardMsg{places: board(testProcs, 67032, 501, testRoots), gen: m.boardGen})
	m = next.(model)
	if cmd == nil || !strings.Contains(m.View().Content, "claude --resume") {
		t.Errorf("the board should show what was read and set the tick going:\n%s", m.View().Content)
	}
	if _, cmd := m.Update(boardTickMsg{gen: m.boardGen - 1}); cmd != nil {
		t.Error("a stale tick should be dropped")
	}
	if _, cmd := m.Update(boardTickMsg{gen: m.boardGen}); cmd == nil {
		t.Error("the tick should read the board again")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = next.(model)
	if m.view != viewConsole || !strings.Contains(m.View().Content, "START-UP CHECKS") {
		t.Errorf("c should bring the console back:\n%s", m.View().Content)
	}
	if _, cmd := m.Update(boardTickMsg{gen: m.boardGen}); cmd != nil {
		t.Error("a tick off the board should be dropped")
	}
	next, cmd = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = next.(model)
	if m.view != viewBoard || cmd == nil || m.boardGen != 2 {
		t.Errorf("a key on the finished console should return to the board and read it afresh: gen %d", m.boardGen)
	}
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"}); cmd == nil {
		t.Error("q should close conn from the board")
	}
}
