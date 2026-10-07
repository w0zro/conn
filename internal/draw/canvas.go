package draw

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/w0zro/conn/internal/theme"

	"github.com/charmbracelet/x/ansi"
)

// What every view of conn's draws with: a palette of the theme's
// colors, a canvas of rows at a width, and lines painted on it piece
// by piece; and the text held to the cells there are for it.

// The Palette is the handoff's tokens. The plain Palette has no
// sequences at all: the console is text, for a pipe and for the tests.
type Palette struct {
	Ground, Border, Ink, Gray, Faint, Orange, Parchment, Bold, Chip string
	Dim                                                             string // the terminal's own dimming, laid over an ink: a step under the faint
	Surface                                                         string // one step off the ground: the panel's own
	Edge, Running, Struck                                           string // ink in the surface, for a card's edges; the dot of a row at work; what is not running
	Selection                                                       string // the ground a chosen row sits on
	Well                                                            string // the ground under the surface, which a field is cut down to
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
		Dim:       ansi.Style{}.Faint().String(),
		Chip:      ansi.Style{}.BackgroundColor(theme.RGB(g.Accent)).ForegroundColor(g.Ground).Bold().String(),
		Selection: groundIn(g.Border),
		Surface:   groundIn(g.Surface),
		Well:      groundIn(theme.Hex(g.Ground)),
		Edge:      inkIn(g.Surface),
		Running:   inkIn(g.Running),
		Struck:    ansi.Style{}.Strikethrough(true).String(),
		End:       ansi.Style{}.Reset().String(),
	}
	p.Normal = p.End + p.Ground + p.Ink
	return p
}

// Lifted is the palette with the ground raised one step, to the
// surface: a block drawn in it sits on that ground edge to edge without
// taking the one a chosen row sits on.
func (p Palette) Lifted() Palette {
	return p.OnSurface()
}

// OnSurface is the palette with the surface for its ground: what the
// panel draws in, its pane being a step off the ground the bay is on,
// so the two halves of the window read as two things. The plain
// palette has no ground either way.
func (p Palette) OnSurface() Palette {
	if p.Plain {
		return p
	}
	p.Ground = p.Surface
	p.Normal = p.End + p.Ground + p.Ink
	return p
}

// Chosen is the palette with the ground raised to the selection color:
// a row drawn in it sits on that ground instead, from edge to edge, and
// every piece on it returns to it rather than to the ground. It is how
// a row is shown to be the one under the cursor. The plain palette has
// no ground to raise, and marks the row instead.
func (p Palette) Chosen() Palette {
	if p.Plain {
		return p
	}
	p.Ground = p.Selection
	p.Normal = p.End + p.Selection + p.Ink
	return p
}

// A view is set to the terminal's width less a Margin each side, and
// called small under MinCols, the console's own width.
const (
	Margin  = 3
	MinCols = 80
)

// MeasureOf is the measure at a width: the width less the margin each
// side.
func MeasureOf(width int) int {
	return width - 2*Margin
}

// MeasureAt is the measure a view draws to at a width: the console's,
// or, under minCols, the panel's, which is a column wider. The panel
// is a pane, and the bay's border stands past its right edge: with
// the border, three columns of air on the right came to four, one more
// than the margin on the left. The panel gives up two, and with the
// border the sides match. At the console's own width there is no
// border, and the margins are the margin each side.
func MeasureAt(width int) int {
	measure := MeasureOf(width)
	if width < MinCols {
		measure++
	}
	return measure
}

// A Row of the console and the stage of the sequence it comes on at.
type Row struct {
	Text  string
	Stage int
	PID   int // the entry the row draws, where it draws one, for a click to find
}

// A Canvas takes rows of one width in one palette.
type Canvas struct {
	P     Palette
	Width int
	Rows  []Row
}

// A Line is built from painted pieces; cells counts the columns. A mark
// is set in the margin, before the Line, where there is no color to say
// which row is the cursor's — see Palette.Chosen. A turn is set in the
// margin too, in the column after the mark: a frame of something going
// round beside the row it belongs to.
type Line struct {
	P     Palette
	b     strings.Builder
	Cells int
	Mark  string
	Turn  string
	PID   int // the entry the line is, where it is one; see Row
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
// that centers it — the pieces, and the ground to the edge. A marked
// row carries its mark at the very edge and a turning row its frame in
// the column after it, the rest of the margin after them. In the plain
// palette the ground is nothing, and the row ends with its last piece.
func (c *Canvas) Emit(l *Line, stage int, centered bool) {
	p := l.P
	left := Margin
	if centered {
		left = max((c.Width-l.Cells)/2, 0)
	}
	lead := strings.Repeat(" ", left)
	if !centered && (l.Mark != "" || l.Turn != "") {
		mark, turn := " ", " "
		if l.Mark != "" {
			mark = p.Orange + p.Bold + l.Mark + p.Normal
		}
		if l.Turn != "" {
			turn = p.Running + l.Turn + p.Normal
		}
		lead = mark + turn + strings.Repeat(" ", Margin-2)
	}
	text := p.Normal + lead + l.b.String() + strings.Repeat(" ", max(c.Width-left-l.Cells, 0)) + p.End
	if p.Plain {
		text = strings.TrimRight(text, " ")
	}
	c.Rows = append(c.Rows, Row{Text: text, Stage: stage, PID: l.PID})
}

func (c *Canvas) Blank(stage int) {
	c.Emit(c.Line(), stage, false)
}

func (c *Canvas) Rule(stage, measure int) {
	l := c.Line()
	l.Add(c.P.Border, strings.Repeat("─", measure))
	c.Emit(l, stage, false)
}

// A view on the panel opens and closes the same way: its title in the
// accent against a figure in the gray, a rule under them, and at the
// foot its body kept to the room there is with the cursor's row in
// view and the ground filling the rest. What stands between is the
// view's own. Eight views opened and closed this way in eight copies,
// and a ninth would have been a ninth.

// Head opens a view: the canvas at the panel's width with the title
// row and the rule on it, and the measure the view draws to.
func Head(title, right string, width int, p Palette) (Canvas, int) {
	width = max(width, PanelMinCols)
	measure := MeasureAt(width)
	c := Canvas{P: p, Width: width}
	c.Blank(0)
	l := c.Line()
	l.Add(p.Orange+p.Bold, title)
	l.To(measure - ansi.StringWidth(right))
	l.Add(p.Gray, right)
	c.Emit(l, 0, false)
	c.Rule(0, measure)
	return c, measure
}

// Foot closes a view: the body under what the canvas holds, kept to
// the room with the cursor's row in view, -1 for none, and the ground
// to the height where one is given; a height of 0 is every row. The
// rows are left on the canvas as well as answered, for a view with a
// row of its own to add under them.
func (c *Canvas) Foot(body []Row, cursorRow, height int) []Row {
	room := height
	if height == 0 {
		room = 1 << 30
	}
	c.Rows = append(c.Rows, Scrolled(body, cursorRow, room-len(c.Rows), c.Width, c.P)...)
	for height > 0 && len(c.Rows) < height {
		c.Blank(0)
	}
	return c.Rows
}

// Scrolled is a body of rows kept to the room there is for it, with the
// cursor's row in view: the rows out of view are counted on a row of
// their own at the foot, which is part of the room.
func Scrolled(body []Row, cursorRow, room, width int, p Palette) []Row {
	if room <= 0 || len(body) <= room {
		return body
	}
	visible := max(room-1, 0)
	top := 0
	if cursorRow >= visible {
		top = cursorRow - visible + 1
	}
	below := len(body) - top - visible
	body = body[top:min(top+visible, len(body))]
	d := Canvas{P: p, Width: width}
	l := d.Line()
	note := []string{}
	if top > 0 {
		note = append(note, strconv.Itoa(top)+" ABOVE")
	}
	if below > 0 {
		note = append(note, strconv.Itoa(below)+" BELOW")
	}
	l.Add(p.Gray, "… "+strings.Join(note, " · "))
	d.Emit(l, 0, false)
	return append(body, d.Rows...)
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

// PadTo fills the page out to the pane's height, so a short reading
// does not leave older rows showing under it.
func PadTo(c Canvas, height int) []Row {
	if height > 0 {
		for len(c.Rows) < height {
			c.Blank(0)
		}
		c.Rows = c.Rows[:height]
	}
	return c.Rows
}

// WrapValue breaks a value to a width, on spaces where there are any
// and hard where there are none — a command line is mostly spaces and a
// path is none, and both have to arrive whole. It is conn's own rather
// than x/ansi's, which breaks at every hyphen too: a command's flags and
// a tmux verb are words that have hyphens in them, and a line that ends
// in --res is a command nobody can read back.
func WrapValue(s string, width int) []string {
	width = max(width, 1)
	var out []string
	for ansi.StringWidth(s) > width {
		head := ansi.Truncate(s, width, "")
		if head == "" {
			// A character wider than the line goes on one of its own.
			_, n := utf8.DecodeRuneInString(s)
			head = s[:n]
		}
		cut := strings.LastIndexByte(head, ' ')
		if cut <= 0 {
			out = append(out, head)
			s = s[len(head):]
			continue
		}
		out = append(out, s[:cut])
		s = strings.TrimLeft(s[cut:], " ")
	}
	if s == "" && len(out) > 0 {
		return out
	}
	return append(out, s)
}

// BlankRow is a row of nothing at a width, in a palette, as Emit frames
// one.
func BlankRow(p Palette, width int) string {
	cv := Canvas{P: p, Width: width}
	cv.Blank(0)
	return cv.Rows[0].Text
}

// TypedRuns is the line as drawn: the text either side of the caret,
// fitted to the room. A line longer than the room shows the part
// around the caret, and where there is room for more, the start of a
// filter, which is read from its start, or the end of a path, which
// is read from its end; a cut end is marked. The caret is always on
// screen.
func TypedRuns(text string, caret, room int, path bool) (before, after string) {
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

// Texts is the rows' text, a line to a row, as a view hands it to
// the terminal.
func Texts(rows []Row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Text
	}
	return out
}
