package main

import (
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The boot console, as the design hands it off and the brief has grown
// it: the wordmark with the station's identification beside it; under a
// rule, the readout — the system on the left, the session on the right;
// the start-up checks, one status column against the right edge; and
// under a second rule the verdict, pulled tight — the count of faults as
// a chip, or the word that all is well. On a terminal the bottom row
// waits on a key. Uppercase throughout, by design; a path keeps its own
// case, since its case is part of it.

// The palette is the handoff's tokens. The plain palette has no
// sequences at all: the console is text, for a pipe and for the tests.
type palette struct {
	ground, border, ink, gray, faint, orange, owed, parchment, bold, chip string
	normal, end                                                           string // ink on the ground again; the row's end
	plain                                                                 bool
}

var plain = palette{plain: true}

func colored() palette {
	p := palette{
		ground:    "\x1b[48;2;21;19;15m",
		border:    "\x1b[38;2;42;38;32m",
		ink:       "\x1b[38;2;230;223;208m",
		gray:      "\x1b[38;2;139;130;114m",
		faint:     "\x1b[38;2;92;86;74m",
		orange:    "\x1b[38;2;232;93;47m",
		owed:      "\x1b[38;2;255;120;71m",
		parchment: "\x1b[38;2;191;179;154m",
		bold:      "\x1b[1m",
		chip:      "\x1b[48;2;232;93;47m\x1b[38;2;21;19;15m\x1b[1m",
		end:       "\x1b[0m",
	}
	p.normal = p.end + p.ground + p.ink
	return p
}

// The console is set to the terminal's width less a margin each side:
// the readout in two columns of half the measure, the checks' statuses
// flush with its right edge. These are the fixed columns, and the width
// under which the terminal is called small; the rows it needs depend on
// the report.
const (
	margin     = 3
	factCol    = 12 // a fact's value, from its column
	checkCol   = 12 // a check's value, from the margin
	statusW    = 9  // the widest status: UNCHECKED, NOT A DIR
	minCols    = 80
	stationGap = 5 // between the wordmark and the station block
	prompt     = "PRESS ANY KEY TO CONTINUE"
)

// columns are the measure's, for a terminal width columns wide: the
// second column of the readout, and where a check's leaders stop, short
// of the widest status and a space.
func columns(width int) (measure, rightCol, leaderEnd int) {
	measure = width - 2*margin
	return measure, measure / 2, measure - statusW - 2
}

// A row of the console and the stage of the sequence it comes on at.
type row struct {
	text  string
	stage int
}

// The stages: the header at once, the readout, each check in turn, and
// the verdict last.
const (
	stageHeader = iota
	stageReadout
	stageChecks // the first check; each after is one more
)

// lastStage is the verdict's, after the screen's check and the report's.
func lastStage(r report) int {
	return stageChecks + 1 + len(r.checks)
}

// rowsNeeded is how many rows the console takes for a report: the body,
// then air and the prompt. It is counted off the layout, not summed by
// hand.
func rowsNeeded(r report) int {
	return len(body(r, minCols, check{}, plain)) + 2
}

// screen renders the console for a terminal of the given size, in the
// palette: rows the terminal's width, painted on the ground, to its
// height. Off a terminal, height is 0, and the rows are the body alone.
// A terminal too small for the body gets the small console instead.
func screen(r report, width, height int, p palette) []row {
	need := rowsNeeded(r)
	own := screenCheck(r.term, width, height, need)
	if own.fault {
		return small(own, width, height, need, p)
	}
	rows := body(r, max(width, minCols), own, p)
	if height > 0 {
		c := canvas{p: p, width: max(width, minCols), rows: rows}
		for len(c.rows) < height-1 {
			c.blank(lastStage(r))
		}
		l := c.line()
		l.add(p.gray, prompt)
		c.emit(l, lastStage(r), true)
		rows = c.rows
	}
	return rows
}

// body is the console proper, at a width no less than minCols, with the
// screen's own check first among the checks.
func body(r report, width int, own check, p palette) []row {
	measure, rightCol, leaderEnd := columns(width)
	c := canvas{p: p, width: width}

	// The header: the wordmark, the station block beside it on its first
	// rows, and a rule under it.
	c.blank(stageHeader)
	markW := utf8.RuneCountInString(wordmark[0])
	station := [][2]string{
		{p.bold, strings.TrimSpace("CONN " + r.version)},
		{p.gray, "STATION  " + strings.ToUpper(r.station)},
		{p.gray, strings.ToUpper(r.clock)},
		{p.gray, r.build},
	}
	for i, m := range wordmark {
		l := c.line()
		l.add(p.orange+p.bold, m)
		if i < len(station) && station[i][1] != "" {
			l.to(markW + stationGap)
			l.add(station[i][0], station[i][1])
			if i == 0 && r.note != "" {
				l.add(p.gray, " "+r.note)
			}
		}
		c.emit(l, stageHeader, false)
	}
	c.rule(stageHeader, measure)

	// The readout: the system in the left column, the session in the
	// right, each under its title.
	factLine := func(l *line, col, width int, f fact) {
		l.to(col)
		l.leader(strings.ToUpper(f.label), col+factCol-1, p.faint)
		l.add(p.ink, fit(cased(f.value, f.path), width-factCol-2, f.path))
	}
	l := c.line()
	l.title(0, "SYSTEM")
	l.title(rightCol, "SESSION")
	c.emit(l, stageReadout, false)
	for i := 0; i < max(len(r.system), len(r.session)); i++ {
		l := c.line()
		if i < len(r.system) {
			factLine(l, 0, rightCol, r.system[i])
		}
		if i < len(r.session) {
			factLine(l, rightCol, measure-rightCol, r.session[i])
		}
		c.emit(l, stageReadout, false)
	}

	// The checks: the title, then the screen's own line and the report's,
	// each to the status column.
	c.blank(stageChecks)
	l = c.line()
	l.title(0, "START-UP CHECKS")
	c.emit(l, stageChecks, false)
	faults := 0
	for i, k := range append([]check{own}, r.checks...) {
		l := c.line()
		l.leader(strings.ToUpper(k.label), checkCol-1, p.gray)
		l.add(p.ink, fit(cased(k.value, k.path), leaderEnd-checkCol-2, k.path))
		l.add("", " ")
		l.add(p.border, strings.Repeat(".", max(leaderEnd-l.cells, 1)))
		if k.fault {
			faults++
			l.to(measure - utf8.RuneCountInString(k.status) - 2)
			l.add(p.chip, " "+strings.ToUpper(k.status)+" ")
		} else {
			l.to(measure - utf8.RuneCountInString(k.status))
			l.add(p.gray, strings.ToUpper(k.status))
		}
		c.emit(l, stageChecks+i, false)
	}

	// The verdict: a rule, then the count of faults as a chip, or the
	// word that all is well.
	last := lastStage(r)
	c.blank(last)
	c.rule(last, measure)
	l = c.line()
	switch {
	case faults > 1:
		l.add(p.chip, " "+strconv.Itoa(faults)+" SYSTEMS NOT NOMINAL ")
	case faults == 1:
		l.add(p.chip, " 1 SYSTEM NOT NOMINAL ")
	default:
		l.add(p.gray, "ALL SYSTEMS NOMINAL")
	}
	c.emit(l, last, true)
	return c.rows
}

// small is the console for a terminal the body will not fit: the mark
// when there is room for it, the name otherwise, and the screen's check
// with the size it needs, all at once. Nothing is clipped, so the reason
// is always in view.
func small(own check, width, height, need int, p palette) []row {
	c := canvas{p: p, width: width}
	measure, _, _ := columns(width)
	c.blank(stageHeader)
	if markW := utf8.RuneCountInString(wordmark[0]); measure >= markW && height >= len(wordmark)+5 {
		for _, m := range wordmark {
			l := c.line()
			l.add(p.orange+p.bold, m)
			c.emit(l, stageHeader, false)
		}
	} else {
		l := c.line()
		l.add(p.orange+p.bold, "CONN")
		c.emit(l, stageHeader, false)
	}
	c.blank(stageHeader)
	l := c.line()
	l.add(p.gray, "SCREEN ")
	l.add(p.ink, fit(strings.ToUpper(own.value), measure-len("SCREEN ")-len(" SMALL ")-1, false))
	l.to(measure - len(" SMALL "))
	l.add(p.chip, " SMALL ")
	c.emit(l, stageHeader, false)
	l = c.line()
	l.add(p.gray, "NEEDS ")
	l.add(p.ink, strconv.Itoa(minCols)+"×"+strconv.Itoa(need))
	c.emit(l, stageHeader, false)
	if height > 0 {
		for len(c.rows) < height-1 {
			c.blank(stageHeader)
		}
		if len(c.rows) == height-1 {
			l := c.line()
			l.add(p.gray, prompt)
			c.emit(l, stageHeader, true)
		}
	}
	return c.rows
}

// screenCheck is the screen itself: its size, what the terminal calls
// itself, and whether the console fits. Off a terminal there is no
// screen to check.
func screenCheck(term string, width, height, need int) check {
	if height == 0 {
		return check{label: "SCREEN", value: join(" · ", "NO TERMINAL", term), status: unchecked}
	}
	value := join(" · ", strconv.Itoa(width)+"×"+strconv.Itoa(height), term)
	if width < minCols || height < need {
		return check{label: "SCREEN", value: value, status: "SMALL", fault: true}
	}
	return check{label: "SCREEN", value: value, status: nominal}
}

// A canvas takes rows of one width in one palette.
type canvas struct {
	p     palette
	width int
	rows  []row
}

// A line is built from painted pieces; cells counts the columns.
type line struct {
	p     palette
	b     strings.Builder
	cells int
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
	l.cells += utf8.RuneCountInString(s)
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
// that centers it — the pieces, and the ground to the edge. In the plain
// palette the ground is nothing, and the row ends with its last piece.
func (c *canvas) emit(l *line, stage int, centered bool) {
	p := c.p
	left := margin
	if centered {
		left = max((c.width-l.cells)/2, 0)
	}
	text := p.normal + strings.Repeat(" ", left) + l.b.String() + strings.Repeat(" ", max(c.width-left-l.cells, 0)) + p.end
	if p.plain {
		text = strings.TrimRight(text, " ")
	}
	c.rows = append(c.rows, row{text: text, stage: stage})
}

func (c *canvas) blank(stage int) {
	c.emit(c.line(), stage, false)
}

func (c *canvas) rule(stage, measure int) {
	l := c.line()
	l.add(c.p.border, strings.Repeat("─", measure))
	c.emit(l, stage, false)
}

// cased is a value as the console sets it: in capitals, unless it is a
// path.
func cased(value string, path bool) string {
	if path {
		return value
	}
	return strings.ToUpper(value)
}

// stdoutIsTerminal says whether what conn prints is going to a person's
// screen, or to a pipe or file.
func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// fit holds a value to w columns. A path is shortened between its head
// and its end so the name it leads to is what survives; anything else is
// cut at the end. A note after the path, set off by " · ", keeps its
// place.
func fit(s string, w int, path bool) string {
	if utf8.RuneCountInString(s) <= w {
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
		if room := w - utf8.RuneCountInString(note); room >= 6 {
			return shortenPath(path, room) + note
		}
	}
	return string([]rune(s)[:w-1]) + "…"
}

// shortenPath elides directories from the middle of a path until it
// fits in w columns, keeping the head and as much of the end as will go.
// When the last name alone will not fit, its end is what shows.
func shortenPath(path string, w int) string {
	if utf8.RuneCountInString(path) <= w {
		return path
	}
	parts := strings.Split(path, "/")
	head := 1 // ~ or the first directory; under the root, the first directory
	if parts[0] == "" && len(parts) > 2 {
		head = 2
	}
	for keep := len(parts) - head - 1; keep >= 1; keep-- {
		s := strings.Join(parts[:head], "/") + "/…/" + strings.Join(parts[len(parts)-keep:], "/")
		if utf8.RuneCountInString(s) <= w {
			return s
		}
	}
	r := []rune(path)
	return "…" + string(r[len(r)-(w-1):])
}
