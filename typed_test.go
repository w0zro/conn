package main

import "testing"

// The keys every typed line has, answered in one place: a character
// goes in at the caret and puts the cursor back on the first row, the
// editing is readline's, and up and down and their ctrl pair walk the
// rows as a ring, off one end and on at the other. A key that is none
// of these is left to the view.
func TestATypedLineAnswersItsOwnKeys(t *testing.T) {
	var l typed
	took := func(k string, rows int) bool { return l.edit(k, rows) }
	for _, k := range []string{"c", "o", "n", "space", "n"} {
		if !took(k, 5) {
			t.Errorf("%q was not typed", k)
		}
	}
	if l.text != "con n" || l.at != 0 || l.cur != 5 {
		t.Fatalf("typed: %q at %d, caret %d", l.text, l.at, l.cur)
	}
	took("down", 5)
	took("ctrl+n", 5)
	if l.at != 2 {
		t.Errorf("down twice left the cursor at %d", l.at)
	}
	took("up", 5)
	took("ctrl+p", 5)
	if l.at != 0 {
		t.Errorf("up twice left the cursor at %d", l.at)
	}
	// Off either end and on at the other.
	took("ctrl+p", 5)
	if l.at != 4 {
		t.Errorf("up off the first row left the cursor at %d", l.at)
	}
	took("down", 5)
	if l.at != 0 {
		t.Errorf("down off the last row left the cursor at %d", l.at)
	}
	for range 9 {
		took("down", 5)
	}
	if l.at != 4 {
		t.Errorf("nine rows down a ring of five left the cursor at %d", l.at)
	}
	// Typing narrows the rows, and the cursor goes back to the first of
	// what is left.
	took("x", 2)
	if l.text != "con nx" || l.at != 0 {
		t.Errorf("after a character: %q at %d", l.text, l.at)
	}
	took("backspace", 5)
	if l.text != "con n" {
		t.Errorf("after backspace: %q", l.text)
	}
	took("ctrl+u", 5)
	if l.text != "" || l.at != 0 {
		t.Errorf("after ctrl+u: %q at %d", l.text, l.at)
	}
	// A key the line does not have is the view's own, and the line is
	// left as it was.
	took("é", 5) // a character, whatever the alphabet
	text, at, cur := l.text, l.at, l.cur
	for _, k := range []string{"enter", "esc", "tab", "ctrl+c", "alt+p", "f1"} {
		if took(k, 5) {
			t.Errorf("%q was taken as the line's own", k)
		}
	}
	if l.text != text || l.at != at || l.cur != cur {
		t.Errorf("a key that was not the line's changed it: %q at %d, caret %d", l.text, l.at, l.cur)
	}
	// With no rows the cursor is the first, which is no row.
	took("down", 0)
	if l.at != 0 {
		t.Errorf("with no rows the cursor went to %d", l.at)
	}
}

// The line is edited the way readline edits one: the caret moves by
// character and by word, to either end, and the kills take what is on
// the side they name. A motion narrows nothing, so the cursor stays on
// its row; a change narrows the rows and puts it back on the first.
func TestATypedLineIsEditedLikeReadline(t *testing.T) {
	var l typed
	for _, k := range []string{"w", "0", "z", "r", "o", "/", "c", "o", "n", "n"} {
		l.edit(k, 5)
	}
	l.edit("down", 5)
	l.edit("down", 5)
	step := func(k, text string, cur int) {
		t.Helper()
		if !l.edit(k, 5) {
			t.Fatalf("%q was not the line's", k)
		}
		if l.text != text || l.cur != cur {
			t.Errorf("after %s: %q caret %d, want %q caret %d", k, l.text, l.cur, text, cur)
		}
	}
	step("ctrl+a", "w0zro/conn", 0)
	step("ctrl+e", "w0zro/conn", 10)
	step("ctrl+b", "w0zro/conn", 9)
	step("left", "w0zro/conn", 8)
	step("ctrl+f", "w0zro/conn", 9)
	// A word is what stands between spaces, as the input has it, so the
	// path is one word.
	step("alt+b", "w0zro/conn", 0)
	step("alt+f", "w0zro/conn", 10)
	for range 5 {
		step("left", "w0zro/conn", l.cur-1)
	}
	if l.at != 2 {
		t.Errorf("a motion moved the cursor off its row, to %d", l.at)
	}
	step("ctrl+k", "w0zro", 5)
	if l.at != 0 {
		t.Errorf("a kill left the cursor on row %d", l.at)
	}
	step("ctrl+w", "", 0)
	for _, k := range []string{"a", "b", "space", "c", "d"} {
		l.edit(k, 5)
	}
	step("alt+b", "ab cd", 3)
	step("alt+d", "ab ", 3)
	step("ctrl+h", "ab", 2)
	step("home", "ab", 0)
	step("ctrl+d", "b", 0)
	step("x", "xb", 1)
	step("end", "xb", 2)
	step("alt+backspace", "", 0)
	// A line filled in from outside is edited from its end.
	l.set("~/pro")
	step("backspace", "~/pr", 4)
}

// The line as drawn keeps the caret on screen. What fits is drawn
// whole; a filter longer than the room shows its start until the caret
// goes past it, and a path shows its end until the caret goes before
// it; a cut end is marked.
func TestTheDrawnLineKeepsTheCaretOnScreen(t *testing.T) {
	for _, c := range []struct {
		text          string
		caret, room   int
		path          bool
		before, after string
	}{
		{"conn", 4, 10, false, "conn", ""},
		{"conn", 1, 10, false, "c", "onn"},
		{"abcdefghij", 10, 5, false, "…ghij", ""},
		{"abcdefghij", 0, 5, false, "", "abcd…"},
		{"abcdefghij", 6, 5, false, "…cde…", ""},
		{"/a/b/c/d/e", 10, 5, true, "…/d/e", ""},
		{"/a/b/c/d/e", 0, 5, true, "", "/a/b…"},
		{"ab", 1, 1, false, "", ""},
	} {
		before, after := typedRuns(c.text, c.caret, c.room, c.path)
		if before != c.before || after != c.after {
			t.Errorf("typedRuns(%q, %d, %d, %v) = %q, %q; want %q, %q", c.text, c.caret, c.room, c.path, before, after, c.before, c.after)
		}
	}
}
