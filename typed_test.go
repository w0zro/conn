package main

import "testing"

// The keys every typed line has, answered in one place: characters go
// on the end and put the cursor back on the first row, backspace and
// ctrl+u take the text back, up and down and their ctrl pair walk the
// rows and stop at either end. A key that is none of these is left to
// the view.
func TestATypedLineAnswersItsOwnKeys(t *testing.T) {
	var l typed
	took := func(k string, rows int) bool { return l.edit(k, rows) }
	for _, k := range []string{"c", "o", "n", "space", "n"} {
		if !took(k, 5) {
			t.Errorf("%q was not typed", k)
		}
	}
	if l.text != "con n" || l.at != 0 {
		t.Fatalf("typed: %q at %d", l.text, l.at)
	}
	took("down", 5)
	took("ctrl+n", 5)
	if l.at != 2 {
		t.Errorf("down twice left the cursor at %d", l.at)
	}
	took("up", 5)
	took("ctrl+p", 5)
	took("ctrl+p", 5)
	if l.at != 0 {
		t.Errorf("up past the first row left the cursor at %d", l.at)
	}
	for range 9 {
		took("down", 5)
	}
	if l.at != 4 {
		t.Errorf("down past the last row left the cursor at %d", l.at)
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
	before := l
	for _, k := range []string{"enter", "esc", "tab", "ctrl+c", "alt+p", "f1"} {
		if took(k, 5) {
			t.Errorf("%q was taken as the line's own", k)
		}
	}
	if l != before {
		t.Errorf("a key that was not the line's changed it: %+v", l)
	}
	// With no rows the cursor is the first, which is no row.
	took("down", 0)
	if l.at != 0 {
		t.Errorf("with no rows the cursor went to %d", l.at)
	}
}
