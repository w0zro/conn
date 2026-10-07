package main

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

// The palette is the handoff's tokens. The plain palette has no
// sequences at all: the console is text, for a pipe and for the tests.
type palette struct {
	ground, border, ink, gray, faint, orange, parchment, bold, chip string
	dim                                                             string // the terminal's own dimming, laid over an ink: a step under the faint
	surface                                                         string // one step off the ground: the panel's own
	edge, running, struck                                           string // ink in the surface, for a card's edges; the dot of a row at work; what is not running
	selection                                                       string // the ground a chosen row sits on
	well                                                            string // the ground under the surface, which a field is cut down to
	normal, end                                                     string // ink on the ground again; the row's end
	plain                                                           bool
}

var plain = palette{plain: true}

// inkIn is a hex of a ground's table as the sequence that draws text
// in it, and groundIn as the one that lays a ground of it: x/ansi
// writes the style, and conn only says which color and which side.
func inkIn(h string) string {
	return ansi.Style{}.ForegroundColor(theme.RGB(h)).String()
}

func groundIn(h string) string {
	return ansi.Style{}.BackgroundColor(theme.RGB(h)).String()
}

// colored is the palette on a ground: the console's own tokens are
// the ground's table, so light or dark reaches this palette the same
// way it reaches everything else conn draws.
func colored(g theme.Ground) palette {
	p := palette{
		ground:    groundIn(theme.Hex(g.Ground)),
		border:    inkIn(g.Border),
		ink:       inkIn(theme.Hex(g.Ink)),
		gray:      inkIn(g.Gray),
		faint:     inkIn(g.Faint),
		orange:    inkIn(g.Accent),
		parchment: inkIn(g.Parchment),
		bold:      ansi.Style{}.Bold().String(),
		dim:       ansi.Style{}.Faint().String(),
		chip:      ansi.Style{}.BackgroundColor(theme.RGB(g.Accent)).ForegroundColor(g.Ground).Bold().String(),
		selection: groundIn(g.Border),
		surface:   groundIn(g.Surface),
		well:      groundIn(theme.Hex(g.Ground)),
		edge:      inkIn(g.Surface),
		running:   inkIn(g.Running),
		struck:    ansi.Style{}.Strikethrough(true).String(),
		end:       ansi.Style{}.Reset().String(),
	}
	p.normal = p.end + p.ground + p.ink
	return p
}

// lifted is the palette with the ground raised one step, to the
// surface: a block drawn in it sits on that ground edge to edge without
// taking the one a chosen row sits on.
func (p palette) lifted() palette {
	return p.onSurface()
}

// onSurface is the palette with the surface for its ground: what the
// panel draws in, its pane being a step off the ground the bay is on,
// so the two halves of the window read as two things. The plain
// palette has no ground either way.
func (p palette) onSurface() palette {
	if p.plain {
		return p
	}
	p.ground = p.surface
	p.normal = p.end + p.ground + p.ink
	return p
}

// chosen is the palette with the ground raised to the selection color:
// a row drawn in it sits on that ground instead, from edge to edge, and
// every piece on it returns to it rather than to the ground. It is how
// a row is shown to be the one under the cursor. The plain palette has
// no ground to raise, and marks the row instead.
func (p palette) chosen() palette {
	if p.plain {
		return p
	}
	p.ground = p.selection
	p.normal = p.end + p.selection + p.ink
	return p
}

// A view is set to the terminal's width less a margin each side, and
// called small under minCols, the console's own width.
const (
	margin  = 3
	minCols = 80
)

// measureOf is the measure at a width: the width less the margin each
// side.
func measureOf(width int) int {
	return width - 2*margin
}

// measureAt is the measure a view draws to at a width: the console's,
// or, under minCols, the panel's, which is a column wider. The panel
// is a pane, and the bay's border stands past its right edge: with
// the border, three columns of air on the right came to four, one more
// than the margin on the left. The panel gives up two, and with the
// border the sides match. At the console's own width there is no
// border, and the margins are the margin each side.
func measureAt(width int) int {
	measure := measureOf(width)
	if width < minCols {
		measure++
	}
	return measure
}

// A row of the console and the stage of the sequence it comes on at.
type row struct {
	text  string
	stage int
	pid   int // the entry the row draws, where it draws one, for a click to find
}

// A canvas takes rows of one width in one palette.
type canvas struct {
	p     palette
	width int
	rows  []row
}

// A line is built from painted pieces; cells counts the columns. A mark
// is set in the margin, before the line, where there is no color to say
// which row is the cursor's — see palette.chosen. A turn is set in the
// margin too, in the column after the mark: a frame of something going
// round beside the row it belongs to.
type line struct {
	p     palette
	b     strings.Builder
	cells int
	mark  string
	turn  string
	pid   int // the entry the line is, where it is one; see row
}

func (c *canvas) line() *line {
	return &line{p: c.p}
}

// add paints a piece in a color, or none, and returns to ink on the
// ground after it, so no piece's color runs on.
func (l *line) add(color, s string) {
	if color != "" {
		l.b.WriteString(color)
	}
	l.b.WriteString(s)
	if color != "" {
		l.b.WriteString(l.p.normal)
	}
	l.cells += ansi.StringWidth(s)
}

// to pads the line out to a column.
func (l *line) to(col int) {
	if col > l.cells {
		l.add("", strings.Repeat(" ", col-l.cells))
	}
}

// title is a block's title, at a column.
func (l *line) title(col int, s string) {
	l.to(col)
	l.add(l.p.parchment+l.p.bold, s)
}

// leader is a label and short dots to a field's width.
func (l *line) leader(label string, field int, dots string) {
	l.add(l.p.gray, label)
	l.add("", " ")
	l.add(dots, strings.Repeat(".", max(field-l.cells, 1)))
	l.add("", " ")
}

// emit frames a line as a row: the margin — or, centered, the column
// that centers it — the pieces, and the ground to the edge. A marked
// row carries its mark at the very edge and a turning row its frame in
// the column after it, the rest of the margin after them. In the plain
// palette the ground is nothing, and the row ends with its last piece.
func (c *canvas) emit(l *line, stage int, centered bool) {
	p := l.p
	left := margin
	if centered {
		left = max((c.width-l.cells)/2, 0)
	}
	lead := strings.Repeat(" ", left)
	if !centered && (l.mark != "" || l.turn != "") {
		mark, turn := " ", " "
		if l.mark != "" {
			mark = p.orange + p.bold + l.mark + p.normal
		}
		if l.turn != "" {
			turn = p.running + l.turn + p.normal
		}
		lead = mark + turn + strings.Repeat(" ", margin-2)
	}
	text := p.normal + lead + l.b.String() + strings.Repeat(" ", max(c.width-left-l.cells, 0)) + p.end
	if p.plain {
		text = strings.TrimRight(text, " ")
	}
	c.rows = append(c.rows, row{text: text, stage: stage, pid: l.pid})
}

func (c *canvas) blank(stage int) {
	c.emit(c.line(), stage, false)
}

func (c *canvas) rule(stage, measure int) {
	l := c.line()
	l.add(c.p.border, strings.Repeat("─", measure))
	c.emit(l, stage, false)
}

// A view on the panel opens and closes the same way: its title in the
// accent against a figure in the gray, a rule under them, and at the
// foot its body kept to the room there is with the cursor's row in
// view and the ground filling the rest. What stands between is the
// view's own. Eight views opened and closed this way in eight copies,
// and a ninth would have been a ninth.

// head opens a view: the canvas at the panel's width with the title
// row and the rule on it, and the measure the view draws to.
func head(title, right string, width int, p palette) (canvas, int) {
	width = max(width, panelMinCols)
	measure := measureAt(width)
	c := canvas{p: p, width: width}
	c.blank(0)
	l := c.line()
	l.add(p.orange+p.bold, title)
	l.to(measure - ansi.StringWidth(right))
	l.add(p.gray, right)
	c.emit(l, 0, false)
	c.rule(0, measure)
	return c, measure
}

// foot closes a view: the body under what the canvas holds, kept to
// the room with the cursor's row in view, -1 for none, and the ground
// to the height where one is given; a height of 0 is every row. The
// rows are left on the canvas as well as answered, for a view with a
// row of its own to add under them.
func (c *canvas) foot(body []row, cursorRow, height int) []row {
	room := height
	if height == 0 {
		room = 1 << 30
	}
	c.rows = append(c.rows, scrolled(body, cursorRow, room-len(c.rows), c.width, c.p)...)
	for height > 0 && len(c.rows) < height {
		c.blank(0)
	}
	return c.rows
}

// scrolled is a body of rows kept to the room there is for it, with the
// cursor's row in view: the rows out of view are counted on a row of
// their own at the foot, which is part of the room.
func scrolled(body []row, cursorRow, room, width int, p palette) []row {
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
	d := canvas{p: p, width: width}
	l := d.line()
	note := []string{}
	if top > 0 {
		note = append(note, strconv.Itoa(top)+" ABOVE")
	}
	if below > 0 {
		note = append(note, strconv.Itoa(below)+" BELOW")
	}
	l.add(p.gray, "… "+strings.Join(note, " · "))
	d.emit(l, 0, false)
	return append(body, d.rows...)
}

// cased is a value as the console sets it: in capitals, unless it is a
// path.
func cased(value string, path bool) string {
	if path {
		return value
	}
	return strings.ToUpper(value)
}

// fit holds a value to w columns. A path is shortened between its head
// and its end so the name it leads to is what survives; anything else is
// cut at the end. A note after the path, set off by " · ", keeps its
// place.
func fit(s string, w int, path bool) string {
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

// padTo fills the page out to the pane's height, so a short reading
// does not leave older rows showing under it.
func padTo(c canvas, height int) []row {
	if height > 0 {
		for len(c.rows) < height {
			c.blank(0)
		}
		c.rows = c.rows[:height]
	}
	return c.rows
}

// wrapValue breaks a value to a width, on spaces where there are any
// and hard where there are none — a command line is mostly spaces and a
// path is none, and both have to arrive whole. It is conn's own rather
// than x/ansi's, which breaks at every hyphen too: a command's flags and
// a tmux verb are words that have hyphens in them, and a line that ends
// in --res is a command nobody can read back.
func wrapValue(s string, width int) []string {
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

// blankRow is a row of nothing at a width, in a palette, as emit frames
// one.
func blankRow(p palette, width int) string {
	cv := canvas{p: p, width: width}
	cv.blank(0)
	return cv.rows[0].text
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

// textsOf is the rows' text, a line to a row, as a view hands it to
// the terminal.
func textsOf(rows []row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.text
	}
	return out
}
