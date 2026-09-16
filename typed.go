package main

import "unicode/utf8"

// A typed line: what has been typed into it, and which of the rows
// that answer it the cursor is on. The list, the sessions view and the
// asking view are each one of these over rows of their own — a line to
// type into, the rows it leaves, and a cursor among them — and the
// keys that work the line are the same in all three. They were three
// copies of the same handler, and a fourth view of the shape would
// have been a fourth.
//
// The cursor is an index into the rows the line leaves, not into the
// rows there are: what is typed narrows the rows, and the cursor goes
// back to the first of them, since the row it was on may not be among
// them any more and the first row that answers is the one the typing
// was reaching for.
type typed struct {
	text string
	at   int
}

// edit answers a key every typed line has: up and down and their ctrl
// pair move the cursor among the rows there are, backspace and ctrl+u
// take the text back, and a character goes on the end. It says whether
// the key was one of these. A key that is not is the view's own — enter
// and esc and tab mean something different on each line, and the view
// answers those before asking here.
//
// A character is any key that stands for one, which is how a letter the
// other views are worked by is itself here: a plain s or a or p is
// typed into the line rather than run. ctrl+n and ctrl+p stand in for
// down and up because a hand on the line cannot reach j and k.
func (l *typed) edit(k string, rows int) bool {
	switch {
	case k == "up" || k == "ctrl+p":
		l.at = clamp(l.at-1, rows)
	case k == "down" || k == "ctrl+n":
		l.at = clamp(l.at+1, rows)
	case k == "backspace":
		if r := []rune(l.text); len(r) > 0 {
			l.text = string(r[:len(r)-1])
		}
		l.at = 0
	case k == "ctrl+u":
		l.text, l.at = "", 0
	case k == "space":
		l.text, l.at = l.text+" ", 0
	case utf8.RuneCountInString(k) == 1:
		l.text, l.at = l.text+k, 0
	default:
		return false
	}
	return true
}

// clear empties the line and puts the cursor back on the first row,
// which is how a view of this shape comes on.
func (l *typed) clear() {
	l.text, l.at = "", 0
}
