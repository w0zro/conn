package main

import (
	"strings"
	"unicode/utf8"
)

// The pieces conn draws a row and a page out of: the dot at the head of
// a row, the stamp on the one that wants you, the key you can press, the
// label with a rule running off it to a count, the line typed into, and
// a card's two edges. They are here rather than in screen.go because
// screen.go is the console and these are drawn on every view.
//
// A piece is what it looks like and nothing about what it means: which
// dot a row takes is its view's to decide, and a piece asked for is
// drawn. In the plain palette, which is what a pipe and the tests read,
// a piece keeps whatever it says in glyphs and drops whatever it said
// in color: the dots stay, since ● and ○ are not the same word, and the
// stamp's half-cells go, since a block of color trimmed to nothing is
// two stray characters.

// The dots. A row says what it is doing before it says what it is: the
// eye finds a filled dot in a column of hollow ones without reading a
// word of the row it is on.
const (
	dotWants = "●" // it has stopped to ask you something
	dotWorks = "●" // it is doing something now
	dotRests = "○" // it is up, and quiet
	dotOver  = "◌" // it has ended, or never came up
)

// The half-cells a stamp is capped with, so it begins and ends at half a
// cell rather than on the edge of one.
const (
	stampLeft  = "▐"
	stampRight = "▌"
)

// A card is a block of the surface with half a cell of it above and
// below, which is how a fill gets an edge in a grid of whole cells.
const (
	cardAbove = "▄"
	cardBelow = "▀"
)

// The bar down the left of a row the cursor is on, for the views that
// mark it rather than raising the ground under it.
const cursorBar = "▌"

// dot is the state of a row, at the head of it, in a color the caller
// picks: the accent for what wants you, the green for what is working,
// the faint for what is quiet or over.
func (l *line) dot(color, glyph string) {
	l.add(color, glyph)
	l.add("", "  ")
}

// activity is what a row is doing, in a width: the command, and after
// it the ports it has, in the gray, as web · :8438. The command gives
// up what the ports take; where the width has no room for the ports
// past a few cells of command, the ports go and the command has it.
func (l *line) activity(color, portsColor, command string, ports []string, width int) {
	word := portsWord(ports)
	if w := utf8.RuneCountInString(word); w > 0 && width-w >= 4 {
		l.add(color, fit(command, width-w, false))
		l.add(portsColor, word)
		return
	}
	l.add(color, fit(command, width, false))
}

// stamp is a word knocked out of the accent — WAITING, and how long it
// has been. It is the one piece that is read before it is read: a block
// of color in a row of text is seen first and understood after.
func (l *line) stamp(s string) {
	if l.p.plain {
		l.add(l.p.chip, " "+s+" ")
		return
	}
	l.add(l.p.orange, stampLeft)
	l.add(l.p.chip, " "+s+" ")
	l.add(l.p.orange, stampRight)
}

// stampWidth is the cells a stamp takes, for a caller placing one
// against the measure.
func stampWidth(s string, p palette) int {
	w := utf8.RuneCountInString(s) + 2
	if p.plain {
		return w
	}
	return w + 2
}

// key is a key you can press, on the raised ground: a shape that says
// press me without a word saying so. The manual is the one text, and
// this is the manual's rows put where the decision is made.
func (l *line) key(k string) {
	l.add(l.p.selection+l.p.ink+l.p.bold, " "+k+" ")
}

// keyWidth is the cells a key takes.
func keyWidth(k string) int { return utf8.RuneCountInString(k) + 2 }

// eyebrow is a block's name: a small label, a rule running off it to the
// right edge, and what it counts at the end. It is what a box was for —
// saying where a block begins and what is in it — without a box, which
// costs two rows and two columns and encloses what does not need
// enclosing.
//
// The label is given in the case it is to be read in. A block conn
// names itself shouts, and is the one place conn still does; a project
// is named by its path, and a path keeps its own case, since its case
// is part of it.
func (l *line) eyebrow(col int, label string, right int, count string) {
	l.eyebrowIn(l.p.gray+l.p.bold, col, label, right, count)
}

// eyebrowIn is an eyebrow with its label in a color of the caller's:
// the accent, for a block that is an alarm.
func (l *line) eyebrowIn(color string, col int, label string, right int, count string) {
	l.to(col)
	l.add(color, label)
	l.add("", " ")
	tail := 0
	if count != "" {
		tail = utf8.RuneCountInString(count) + 1
	}
	if n := right - l.cells - tail; n > 0 {
		l.add(l.p.border, strings.Repeat("─", n))
	}
	if count != "" {
		l.add("", " ")
		l.add(l.p.ink+l.p.bold, count)
	}
}

// field is a line typed into, drawn as a field rather than as a word and
// a caret loose on the ground: the raised ground says where the typing
// goes, and holds its width whether anything has been typed or not. The
// caret stands where the typing left it, so what is before it and what
// is after it are given apart.
func (l *line) field(col, width int, label, before, after string) {
	l.to(col)
	l.add(l.p.gray, label)
	l.add("", "  ")
	start := l.cells
	l.add(l.p.selection+l.p.ink, " "+before)
	l.add(l.p.selection+l.p.orange+l.p.bold, caret)
	l.add(l.p.selection+l.p.ink, after)
	if n := width - (l.cells - start); n > 0 {
		l.add(l.p.selection, strings.Repeat(" ", n))
	}
}

// card draws the edge above or below a block of the surface, from a
// column, for a width. Between them the rows are drawn in the lifted
// palette, which puts them on the same surface edge to edge.
func (c *canvas) card(edge string, col, width, stage int) {
	l := c.line()
	l.to(col)
	l.add(c.p.edge, strings.Repeat(edge, width))
	c.emit(l, stage, false)
}
