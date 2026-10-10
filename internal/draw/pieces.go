package draw

import (
	"strings"

	"github.com/w0zro/conn/internal/work"

	"github.com/charmbracelet/x/ansi"
)

// The pieces conn draws a row and a page out of: the mark at the head of
// a row, the stamp on the one that wants you, the key you can press, the
// label with a rule running off it to a count, the line typed into, and
// a card's two edges. canvas.go is what they are painted on.
//
// A piece is what it looks like and nothing about what it means: which
// mark a row takes is its view's to decide, and a piece asked for is
// drawn. In the plain palette, which is what a pipe and the tests read,
// a piece keeps whatever it says in glyphs and drops whatever it said
// in color: the marks stay, since they are five different words, and the
// stamp's half-cells go, since a block of color trimmed to nothing is
// two stray characters.

// The marks. A row says what it is before it says what it is doing: the
// eye tells a contact from a container from a shell down the one column
// it reads the panel by, without reading a word of any row it is on.
//
// The mark said the state before, and the state is said four other ways
// already — a row at work turns a spinner, a row waiting blinks its
// stamp, a row not running is struck through, a row serving says its
// port — so the column at the head of a row was the fifth telling of
// what the row had said, and said nothing of what the row was. The
// command cannot say it either: zsh and vim name themselves, but a
// contact's row says what it is doing, and a declared name, a container
// and a build are one word each with nothing between them. So the mark
// is the kind, and the color on it is how the kind stands.
//
// A kind with a mark of its own takes it: the shell is the prompt it
// shows you, the editor a page, the contact Mercury for the mind at the
// other end, the service a lamp of the kind a console watches rather
// than types at. What is left over is the run, which is most rows, and
// it takes the lightest mark there is, so that a panel of ordinary work
// is quiet and the kinds worth finding stand out of it.
const (
	MarkContact = "☿" // Mercury, the messenger and the first crewed capsule: a mind at the other end
	MarkShell   = "❯" // the prompt it shows you
	MarkEditor  = "▯" // a page, open: it has the terminal and asks nothing
	MarkService = "◉" // a lamp on a console: a thing held up, and watched
	MarkRun     = "○" // anything else, and the commonest row: the lightest mark
)

// MarkOf is the mark a kind wears. A kind conn does not tell apart is a
// run, which is what kindOf makes of it.
func MarkOf(kind string) string {
	switch kind {
	case work.KindContact:
		return MarkContact
	case work.KindShell:
		return MarkShell
	case work.KindEditor:
		return MarkEditor
	case work.KindService:
		return MarkService
	}
	return MarkRun
}

// The half-cells a stamp is capped with, so it begins and ends at half a
// cell rather than on the edge of one.
const (
	stampLeft  = "▐"
	stampRight = "▌"
)

// A card is a block of the surface with half a cell of it above and
// below, which is how a fill gets an edge in a grid of whole cells.
const (
	CardAbove = "▄"
	CardBelow = "▀"
)

// The bar down the left of a row the cursor is on, for the views that
// mark it rather than raising the ground under it.
const CursorBar = "▌"

// Dot is the mark at the head of a row — its kind — in a color the
// caller picks for how it stands: the accent for what wants you, the
// green for what is working, the faint for what is quiet or over.
func (l *Line) Dot(color, glyph string) {
	l.Add(color, glyph)
	l.Add("", "  ")
}

// Activity is what a row is doing, in a width: the command, and after
// it the ports it has, in the gray, as web · :8438. The command gives
// up what the ports take; where the width has no room for the ports
// past a few cells of command, the ports go and the command has it.
func (l *Line) Activity(color, portsColor, command string, ports []string, width int) {
	// The ports are the row's own fact, where you would go, and the
	// last thing a narrow row gives up: the command is elided to what
	// is left beside them, down to a letter, and past that the ports
	// stand alone. Only a width the ports themselves do not fit gives
	// the whole of it to the command.
	word := PortsWord(ports)
	w := ansi.StringWidth(word)
	switch {
	case w == 0:
	case width-w >= 2:
		l.Add(color, Fit(command, width-w, false))
		l.Add(portsColor, word)
		return
	case width >= w-3:
		l.Add(portsColor, strings.TrimPrefix(word, " · "))
		return
	}
	l.Add(color, Fit(command, width, false))
}

// Stamp is a word knocked out of the accent's block — WAITING, and how
// long it has been. It is the one piece that is read before it is read:
// a block of color in a row of text is seen first and understood after.
func (l *Line) Stamp(s string) {
	if l.P.Plain {
		l.Add(l.P.Chip, " "+s+" ")
		return
	}
	l.Add(l.P.blockEnd, stampLeft)
	l.Add(l.P.Chip, " "+s+" ")
	l.Add(l.P.blockEnd, stampRight)
}

// StampWidth is the cells a stamp takes, for a caller placing one
// against the measure.
func StampWidth(s string, p Palette) int {
	w := ansi.StringWidth(s) + 2
	if p.Plain {
		return w
	}
	return w + 2
}

// Key is a Key you can press, on the raised ground: a shape that says
// press me without a word saying so. The manual is the one text, and
// this is the manual's rows put where the decision is made.
func (l *Line) Key(k string) {
	l.Add(l.P.Selection+l.P.Ink+l.P.Bold, " "+k+" ")
}

// KeyWidth is the cells a key takes.
func KeyWidth(k string) int { return ansi.StringWidth(k) + 2 }

// Eyebrow is a block's name: a small label, a rule running off it to the
// right edge, and what it counts at the end. It is what a box was for —
// saying where a block begins and what is in it — without a box, which
// costs two rows and two columns and encloses what does not need
// enclosing.
//
// The label is given in the case it is to be read in. A block conn
// names itself shouts, and is the one place conn still does; a project
// is named by its path, and a path keeps its own case, since its case
// is part of it.
func (l *Line) Eyebrow(col int, label string, right int, count string) {
	l.EyebrowIn(l.P.Gray+l.P.Bold, col, label, right, count)
}

// EyebrowIn is an eyebrow with its label in a color of the caller's:
// the accent, for a block that is an alarm.
func (l *Line) EyebrowIn(color string, col int, label string, right int, count string) {
	l.EyebrowTail(color, col, label, right, ansi.StringWidth(count))
	if count != "" {
		l.Add(l.P.Ink+l.P.Bold, count)
	}
}

// EyebrowTail is an eyebrow whose figure at the end of the rule the
// caller draws itself, in the width it says: a block that says how it
// stands rather than how many rows it has puts a stamp there, and a
// stamp is not a word in a color. The line is left at the column the
// figure begins at.
func (l *Line) EyebrowTail(color string, col int, label string, right, tailW int) {
	l.To(col)
	l.Add(color, label)
	l.Add("", " ")
	tail := 0
	if tailW > 0 {
		tail = tailW + 1
	}
	if n := right - l.Cells - tail; n > 0 {
		l.Add(l.P.Border, strings.Repeat("─", n))
	}
	if tailW > 0 {
		l.Add("", " ")
	}
}

// Field is a line typed into, drawn as a Field rather than as a word and
// a caret loose on the ground: a well cut through the surface down to
// the ground says where the typing goes, and holds its width whether
// anything has been typed or not. The caret stands where the typing
// left it, so what is before it and what is after it are given apart.
func (l *Line) Field(col, width int, label, before, after string) {
	l.To(col)
	l.Add(l.P.Gray, label)
	l.Add("", "  ")
	start := l.Cells
	l.Add(l.P.well+l.P.Ink+l.P.Bold, " "+before)
	l.Add(l.P.well+l.P.Orange+l.P.Bold, caret)
	l.Add(l.P.well+l.P.Ink+l.P.Bold, after)
	if n := width - (l.Cells - start); n > 0 {
		l.Add(l.P.well, strings.Repeat(" ", n))
	}
}

// Card draws the edge above or below a block of the surface, from a
// column, for a width. Between them the rows are drawn in the lifted
// palette, which puts them on the same surface edge to edge.
func (c *Canvas) Card(edge string, col, width, stage int) {
	l := c.Line()
	l.To(col)
	l.Add(c.P.edge, strings.Repeat(edge, width))
	c.Emit(l, stage, false)
}

// PanelMinCols is the narrowest a view is drawn: a panel narrower is
// drawn at this width and cut by the pane.
const PanelMinCols = 40

// caret is where typing goes on a line typed into.
const caret = "▏"

// The Spinner's frames: the cell full but for one dot, the gap going
// round, a full turn in eight, and a turn a second (spinEvery, in
// tui.go). A single dot going round was a trace too faint to be seen
// turning beside a row of text; the full cell has the weight of the
// dot beside it, and the gap is what moves. It turns in the margin,
// where the rows at work make a column of their own and the text they
// are about keeps its line.
var Spinner = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

// PortsWord is how a row of the tree says its ports after its command:
// web · :8438, or every port it has, lowest first. Nothing for a row
// with none. The panel stands them at its right instead, as a column,
// and asks for portsColumn.
func PortsWord(ports []string) string {
	if len(ports) == 0 {
		return ""
	}
	return " · " + PortsColumn(ports)
}

// PortsColumn is the ports written together: :8438, or every port
// lowest first. Nothing for none.
func PortsColumn(ports []string) string {
	if len(ports) == 0 {
		return ""
	}
	return ":" + strings.Join(ports, " :")
}
