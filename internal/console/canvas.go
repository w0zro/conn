package console

import (
	"strings"

	"github.com/w0zro/conn/internal/theme"

	"github.com/charmbracelet/x/ansi"
)

// What the console draws with: a palette of the theme's colors, a
// canvas of rows at a width, and lines painted on it piece by piece;
// and the text held to the cells there are for it.

// The Palette is the handoff's tokens. The plain Palette has no
// sequences at all: the console is text, for a pipe and for the tests.
type Palette struct {
	Ground, Border, Ink, Gray, Faint, Orange, Parchment, Bold, Chip string
	Normal, End                                                     string // ink on the ground again; the row's end
	Plain                                                           bool
}

var Plain = Palette{Plain: true}

// inkIn is a hex of a ground's table as the sequence that draws text
// in it, and groundIn as the one that lays a ground of it: x/ansi
// writes the style, and conn only says which color and which side.
func inkIn(h string) string {
	return ansi.Style{}.ForegroundColor(theme.RGB(h)).String()
}

func groundIn(h string) string {
	return ansi.Style{}.BackgroundColor(theme.RGB(h)).String()
}

// Colored is the palette on a ground: the console's own tokens are
// the ground's table, so light or dark reaches this palette the same
// way it reaches everything else conn draws.
func Colored(g theme.Ground) Palette {
	p := Palette{
		Ground:    groundIn(theme.Hex(g.Ground)),
		Border:    inkIn(g.Border),
		Ink:       inkIn(theme.Hex(g.Ink)),
		Gray:      inkIn(g.Gray),
		Faint:     inkIn(g.Faint),
		Orange:    inkIn(g.Accent),
		Parchment: inkIn(g.Parchment),
		Bold:      ansi.Style{}.Bold().String(),
		Chip:      ansi.Style{}.BackgroundColor(theme.RGB(g.Block)).ForegroundColor(theme.RGB(g.OnBlock)).Bold().String(),
		End:       ansi.Style{}.Reset().String(),
	}
	p.Normal = p.End + p.Ground + p.Ink
	return p
}

// The console is set to the terminal's width less a Margin each side,
// and called small under MinCols.
const (
	Margin  = 3
	MinCols = 80
)

// MeasureOf is the measure at a width: the width less the margin each
// side.
func MeasureOf(width int) int {
	return width - 2*Margin
}

// A Fact is a line of the readout: what is reported and what was
// found. A path is shown as it is, where every other value is set in
// capitals.
type Fact struct {
	Label, Value string
	Path         bool
}

// FactCol is the column a fact's value stands at, from its label's.
const FactCol = 12

// A Row of the console and the stage of the sequence it comes on at.
type Row struct {
	Text  string
	Stage int
}

// A Canvas takes rows of one width in one palette.
type Canvas struct {
	P     Palette
	Width int
	Rows  []Row
}

// A Line is built from painted pieces; Cells counts the columns.
type Line struct {
	P     Palette
	b     strings.Builder
	Cells int
}

func (c *Canvas) Line() *Line {
	return &Line{P: c.P}
}

// Add paints a piece in a color, or none, and returns to ink on the
// ground after it, so no piece's color runs on.
func (l *Line) Add(color, s string) {
	if color != "" {
		l.b.WriteString(color)
	}
	l.b.WriteString(s)
	if color != "" {
		l.b.WriteString(l.P.Normal)
	}
	l.Cells += ansi.StringWidth(s)
}

// To pads the line out To a column.
func (l *Line) To(col int) {
	if col > l.Cells {
		l.Add("", strings.Repeat(" ", col-l.Cells))
	}
}

// Title is a block's Title, at a column.
func (l *Line) Title(col int, s string) {
	l.To(col)
	l.Add(l.P.Parchment+l.P.Bold, s)
}

// Leader is a label and short dots to a field's width.
func (l *Line) Leader(label string, field int, dots string) {
	l.Add(l.P.Gray, label)
	l.Add("", " ")
	l.Add(dots, strings.Repeat(".", max(field-l.Cells, 1)))
	l.Add("", " ")
}

// Emit frames a line as a row: the margin — or, centered, the column
// that centers it — the pieces, and the ground to the edge. In the
// plain palette the ground is nothing, and the row ends with its last
// piece.
func (c *Canvas) Emit(l *Line, stage int, centered bool) {
	p := l.P
	left := Margin
	if centered {
		left = max((c.Width-l.Cells)/2, 0)
	}
	text := p.Normal + strings.Repeat(" ", left) + l.b.String() + strings.Repeat(" ", max(c.Width-left-l.Cells, 0)) + p.End
	if p.Plain {
		text = strings.TrimRight(text, " ")
	}
	c.Rows = append(c.Rows, Row{Text: text, Stage: stage})
}

func (c *Canvas) Blank(stage int) {
	c.Emit(c.Line(), stage, false)
}

func (c *Canvas) Rule(stage, measure int) {
	l := c.Line()
	l.Add(c.P.Border, strings.Repeat("─", measure))
	c.Emit(l, stage, false)
}

// Cased is a value as the console sets it: in capitals, unless it is a
// path.
func Cased(value string, path bool) string {
	if path {
		return value
	}
	return strings.ToUpper(value)
}

// Fit holds a value to w columns. A path is shortened between its head
// and its end so the name it leads to is what survives; anything else is
// cut at the end. A note after the path, set off by " · ", keeps its
// place.
func Fit(s string, w int, path bool) string {
	if ansi.StringWidth(s) <= w {
		return s
	}
	if w <= 1 {
		return ""
	}
	if path {
		path, note, _ := strings.Cut(s, " · ")
		if note != "" {
			note = " · " + note
		}
		if room := w - ansi.StringWidth(note); room >= 6 {
			return shortenPath(path, room) + note
		}
	}
	return ansi.Truncate(s, w, "…")
}

// shortenPath elides directories from the middle of a path until it
// fits in w columns, keeping the head and as much of the end as will go.
// When the last name alone will not fit, its end is what shows.
func shortenPath(path string, w int) string {
	if ansi.StringWidth(path) <= w {
		return path
	}
	parts := strings.Split(path, "/")
	head := 1 // ~ or the first directory; under the root, the first directory
	if parts[0] == "" && len(parts) > 2 {
		head = 2
	}
	for keep := len(parts) - head - 1; keep >= 1; keep-- {
		s := strings.Join(parts[:head], "/") + "/…/" + strings.Join(parts[len(parts)-keep:], "/")
		if ansi.StringWidth(s) <= w {
			return s
		}
	}
	if w < 1 {
		return ""
	}
	// A wide character the cut falls through is kept whole, which can
	// leave a cell too many; the cut then moves past it.
	for n := ansi.StringWidth(path) - (w - 1); ; n++ {
		s := ansi.TruncateLeft(path, n, "…")
		if s == "" {
			return "…" // nothing of the path fits beside it
		}
		if ansi.StringWidth(s) <= w {
			return s
		}
	}
}

// Texts is the rows' text, a line to a row, as the console hands it to
// the terminal.
func Texts(rows []Row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Text
	}
	return out
}

// Join is the parts that are not empty, with the separator between.
func Join(sep string, parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, sep)
}
