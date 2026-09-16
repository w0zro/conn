package main

import (
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// A typed line: what has been typed into it, where in it the caret is,
// and which of the rows that answer it the cursor is on. The list, the
// sessions view and the asking view are each one of these over rows of
// their own — a line to type into, the rows it leaves, and a cursor
// among them — and the keys that work the line are the same in all
// three. They were three copies of the same handler, and a fourth view
// of the shape would have been a fourth.
//
// The editing is readline's, in emacs mode, because that is what a
// hand typing into a shell already knows, and it is not conn's own: the
// line is a text input from bubbles, the one editor of that shape made
// for the program conn is, used as the model only. conn feeds it the
// keys and reads the text and the caret back, and draws the line
// itself, the way it draws everything. What it has and readline has
// not — a yank, a transpose, a word that stops at a slash — is not
// modelled here to make up the difference; the line is what the input
// is.
//
// The cursor is an index into the rows the line leaves, not into the
// rows there are: what is typed narrows the rows, and the cursor goes
// back to the first of them, since the row it was on may not be among
// them any more and the first row that answers is the one the typing
// was reaching for.
type typed struct {
	text string // what the line says, as the views read it
	cur  int    // the caret, in runes from the start
	at   int    // the row the cursor is on
	in   textinput.Model
	live bool // the input has been made; the zero line has none yet
}

// input is the editor, made on first use and kept in step with the
// text: a line filled in from outside — cleared, or completed — is put
// into the editor before the next key reaches it.
func (l *typed) input() *textinput.Model {
	if !l.live {
		l.in = textinput.New()
		l.in.Focus()
		l.live = true
	}
	if l.in.Value() != l.text {
		l.in.SetValue(l.text)
		l.in.SetCursor(utf8.RuneCountInString(l.text))
	}
	return &l.in
}

// edit answers a key every typed line has, and says whether the key
// was one of these: up and down and their ctrl pair move the cursor
// among the rows there are, the way readline walks its history; the
// editor's own keys work the text; and a character goes in at the
// caret. A key that is none of these is the view's own — enter and esc
// and tab mean something different on each line, and the view answers
// those before asking here.
//
// A character is any key that stands for one, which is how a letter the
// other views are worked by is itself here: a plain s or a or p is
// typed into the line rather than run.
func (l *typed) edit(k string, rows int) bool {
	switch k {
	case "up", "ctrl+p":
		l.at = clamp(l.at-1, rows)
		return true
	case "down", "ctrl+n":
		l.at = clamp(l.at+1, rows)
		return true
	}
	in := l.input()
	msg := tea.KeyPressMsg{Text: k}
	switch {
	case k == "space":
		msg = tea.KeyPressMsg{Code: ' ', Text: " "}
	case utf8.RuneCountInString(k) == 1:
		r, _ := utf8.DecodeRuneInString(k)
		msg = tea.KeyPressMsg{Code: r, Text: k}
	case !key.Matches(msg, editing(in.KeyMap)...):
		return false
	}
	was := in.Value()
	l.in, _ = in.Update(msg)
	l.text, l.cur = l.in.Value(), l.in.Position()
	// The rows are narrowed by what the line says now, and the cursor
	// goes back to the first of them; a motion changes nothing and
	// leaves it where it was.
	if l.text != was {
		l.at = 0
	}
	return true
}

// editing is the input's keys that edit the text: the motions and the
// kills, and not the ones for its suggestions, which the line does not
// have, nor its paste, which arrives as the terminal's own paste.
func editing(k textinput.KeyMap) []key.Binding {
	return []key.Binding{
		k.CharacterForward, k.CharacterBackward, k.WordForward, k.WordBackward,
		k.DeleteWordBackward, k.DeleteWordForward, k.DeleteAfterCursor, k.DeleteBeforeCursor,
		k.DeleteCharacterBackward, k.DeleteCharacterForward, k.LineStart, k.LineEnd,
	}
}

// set fills the line in, with the caret at its end, which is how a
// line comes to say something it was not typed: a completion.
func (l *typed) set(text string) {
	l.text, l.cur, l.at = text, utf8.RuneCountInString(text), 0
}

// clear empties the line and puts the cursor back on the first row,
// which is how a view of this shape comes on.
func (l *typed) clear() {
	l.set("")
}

// typedRuns is the line as drawn: the text either side of the caret,
// fitted to the room. A line longer than the room shows the part
// around the caret, and where there is room for more, the start of a
// filter, which is read from its start, or the end of a path, which
// is read from its end; a cut end is marked. The caret is always on
// screen.
func typedRuns(text string, caret, room int, path bool) (before, after string) {
	r := []rune(text)
	caret = min(max(caret, 0), len(r))
	if len(r) <= room {
		return string(r[:caret]), string(r[caret:])
	}
	if room <= 1 {
		return "", ""
	}
	start := 0
	if path {
		start = len(r) - room
	}
	if caret < start {
		start = caret
	}
	if caret > start+room {
		start = caret - room
	}
	end := min(start+room, len(r))
	w := append([]rune{}, r[start:end]...)
	if start > 0 {
		w[0] = '…'
	}
	if end < len(r) {
		w[len(w)-1] = '…'
	}
	return string(w[:caret-start]), string(w[caret-start:])
}
