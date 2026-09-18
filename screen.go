package main

import (
	"fmt"
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
// a chip, or what the checks came to. On a terminal the bottom row
// waits on a key. Uppercase throughout, by design; a path keeps its own
// case, since its case is part of it.

// The palette is the handoff's tokens. The plain palette has no
// sequences at all: the console is text, for a pipe and for the tests.
type palette struct {
	ground, border, ink, gray, faint, orange, parchment, bold, chip string
	surface                                                         string // one step off the ground: the panel's own
	edge, running, struck                                           string // ink in the surface, for a card's edges; the dot of a row at work; what is not running
	selection                                                       string // the ground a chosen row sits on
	well                                                            string // the ground under the surface, which a field is cut down to
	normal, end                                                     string // ink on the ground again; the row's end
	plain                                                           bool
}

var plain = palette{plain: true}

// ansiHex is a hex color, conn's own or a program's, as the escape a
// terminal wants: 38 for ink, 48 for a ground.
func ansiHex(code int, h string) string {
	h = strings.TrimPrefix(h, "#")
	v, _ := strconv.ParseUint(h, 16, 32)
	return fmt.Sprintf("\x1b[%d;2;%d;%d;%dm", code, v>>16&0xFF, v>>8&0xFF, v&0xFF)
}

// colored draws the boot console on whichever ground applyMode last
// picked: the console's own tokens are conn's scheme, so light or dark
// reaches this palette the same way it reaches everything else conn
// draws.
func colored() palette {
	p := palette{
		ground:    ansiHex(48, hex(groundColor)),
		border:    ansiHex(38, borderHex),
		ink:       ansiHex(38, hex(inkColor)),
		gray:      ansiHex(38, grayHex),
		faint:     ansiHex(38, faintHex),
		orange:    ansiHex(38, cursorHex),
		parchment: ansiHex(38, parchmentHex),
		bold:      "\x1b[1m",
		chip:      ansiHex(48, cursorHex) + ansiHex(38, hex(groundColor)) + "\x1b[1m",
		selection: ansiHex(48, borderHex),
		surface:   ansiHex(48, surfaceHex),
		well:      ansiHex(48, hex(groundColor)),
		edge:      ansiHex(38, surfaceHex),
		running:   ansiHex(38, runningHex),
		struck:    "\x1b[9m",
		end:       "\x1b[0m",
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

// rowsNeeded is how many rows the console takes for a report: the
// body, the verdict its last row. It is counted off the layout, not
// summed by hand. The key bar at the foot says how to go on, so the
// console has no row of its own to say it.
func rowsNeeded(r report) int {
	return len(body(r, minCols, check{}, plain))
}

// screen renders the console for a terminal of the given size, in the
// palette: rows the terminal's width, painted on the ground, to its
// height. Off a terminal, height is 0, and the rows are the body alone.
// A terminal too small for the body gets the small console instead.
func screen(r report, width, height int, p palette) []row {
	r = fitted(r, height)
	need := rowsNeeded(r)
	own := screenCheck(r.term, width, height, need)
	if own.fault {
		return small(own, width, height, need, p)
	}
	cols := max(width, minCols)
	if height == 0 {
		cols = wide(r, own)
	}
	rows := body(r, cols, own, p)
	if height > 0 {
		c := canvas{p: p, width: max(width, minCols), rows: rows}
		for len(c.rows) < height {
			c.blank(lastStage(r))
		}
		rows = c.rows
	}
	return rows
}

// fitted is the report as a terminal of this height can hold it. The
// roots are a line each, which is how a root that is not there says so
// on its own account; where the rows for that are not there they become
// one line carrying all of them, under the worst word any of them
// earned. The console already cuts a value to the width it was given,
// and this is the same bargain in the other direction — better a
// console that says less than one that refuses to draw because a root
// was added. Off a terminal, height is 0 and nothing is given up.
//
// It is idempotent: a report already fitted is returned as it is.
func fitted(r report, height int) report {
	if height <= 0 || rowsNeeded(r) <= height {
		return r
	}
	return joinRoots(r)
}

// joinRoots is the report with its ROOT lines made one ROOTS line. The
// paths are joined the way conn joins values, and the status is the
// worst of them: the line stands for all of them, so it cannot say
// NOMINAL while one of them is missing.
func joinRoots(r report) report {
	var paths []string
	var worst check
	var at, n int
	for i, k := range r.checks {
		if k.label != rootLabel {
			continue
		}
		if n == 0 {
			at, worst = i, k
		}
		n++
		paths = append(paths, k.value)
		if worse(k, worst) {
			worst = k
		}
	}
	if n < 2 {
		return r // one root is already one line, and none is no line
	}
	joined := check{label: rootsLabel, value: strings.Join(paths, " · "), path: true,
		status: worst.status, fault: worst.fault}
	checks := make([]check, 0, len(r.checks)-n+1)
	checks = append(checks, r.checks[:at]...)
	checks = append(checks, joined)
	checks = append(checks, r.checks[at+n:]...)
	r.checks = checks
	return r
}

// worse says whether a check stands worse than another: a fault beats
// anything, and anything that is not nominal beats nominal.
func worse(k, than check) bool {
	if k.fault != than.fault {
		return k.fault
	}
	return than.status == nominal && k.status != nominal
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

	// The readout: the machine in the left column, the session in the
	// right, each under its title. The left column was titled SYSTEM
	// until the host came off it, which left a row labelled SYSTEM
	// directly under a title of the same word. The column is the
	// machine and the row is the operating system on it, and now each
	// says which it is.
	factLine := func(l *line, col, width int, f fact) {
		l.to(col)
		l.leader(strings.ToUpper(f.label), col+factCol-1, p.faint)
		l.add(p.ink, fit(cased(f.value, f.path), width-factCol-2, f.path))
	}
	l := c.line()
	l.title(0, "MACHINE")
	l.title(rightCol, "SESSION")
	c.emit(l, stageReadout, false)
	for i := 0; i < max(len(r.system), len(r.login)); i++ {
		l := c.line()
		if i < len(r.system) {
			factLine(l, 0, rightCol, r.system[i])
		}
		if i < len(r.login) {
			factLine(l, rightCol, measure-rightCol, r.login[i])
		}
		c.emit(l, stageReadout, false)
	}

	// The checks: the title, then the screen's own line and the report's,
	// each to the status column. A fault's chip blinks with the verdict's;
	// what is nominal stays put.
	c.blank(stageChecks)
	l = c.line()
	l.title(0, "START-UP CHECKS")
	c.emit(l, stageChecks, false)
	var stood tally
	for i, k := range append([]check{own}, r.checks...) {
		l := c.line()
		l.leader(strings.ToUpper(k.label), checkCol-1, p.gray)
		l.add(p.ink, fit(cased(k.value, k.path), leaderEnd-checkCol-2, k.path))
		l.add("", " ")
		l.add(p.border, strings.Repeat(".", max(leaderEnd-l.cells, 1)))
		stood.count(k)
		// Nominal is said and left alone. Everything else takes the
		// chip and blinks with the verdict: what is not nominal and how
		// many there are are the same alarm. There is one rule for it
		// because there was nearly a second — UNCHECKED said quietly,
		// in a color, while LOW two rows above it was an annunciator —
		// and a console with two grades of wrong makes the reader work
		// out which grade this one is before they can read it. A system
		// conn could not check is not a system conn found nominal, and
		// the console has exactly one way of saying so.
		// The words stand centered on one another, in a field the
		// widest of them fills: LOW between two NOMINALs sits over the
		// MIN, chip and all, and the column reads as one column of
		// words rather than a ragged right edge of boxes.
		word := strings.ToUpper(k.status)
		if k.status == nominal {
			l.to(measure - statusW + (statusW-utf8.RuneCountInString(word))/2)
			l.add(p.gray, word)
		} else if r.lit {
			word = " " + word + " "
			l.to(measure - statusW + (statusW-utf8.RuneCountInString(word))/2)
			l.add(p.chip, word)
		}
		c.emit(l, stageChecks+i, false)
	}

	// The verdict: a rule, then the count of faults as a chip, or what
	// the checks came to. The chip is an annunciator and blinks, a
	// second lit against half of one dark, and the faults' own chips
	// blink with it; on the dark half those cells are the ground, and
	// nothing around them moves.
	last := lastStage(r)
	c.blank(last)
	c.rule(last, measure)
	l = c.line()
	switch {
	case stood.off == 0:
		l.add(p.gray, allNominal)
	case !r.lit:
		// dark this second
	case stood.off > 1:
		l.add(p.chip, " "+strconv.Itoa(stood.off)+" SYSTEMS NOT NOMINAL ")
	default:
		l.add(p.chip, " 1 SYSTEM NOT NOMINAL ")
	}
	c.emit(l, last, true)
	return c.rows
}

// allNominal is the one sentence a console with nothing to report says.
const allNominal = "ALL SYSTEMS NOMINAL"

// A tally is what the start-up checks came to: the ones that passed,
// and the ones that did not. A check conn could not make counts among
// the second, because the console's job is to say what it knows and a
// reading never taken is not a system found well.
type tally struct{ off, nominal int }

func (t *tally) count(k check) {
	if k.status == nominal {
		t.nominal++
		return
	}
	t.off++
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
	for height > 0 && len(c.rows) < height {
		c.blank(stageHeader)
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

// wide is the width at which nothing on the page is elided, for a page
// going somewhere that has no width of its own.
//
// A terminal is a fixed number of columns and the page is cut to them,
// which is the terminal's business and nobody loses anything: the
// operator is looking at the screen and can widen it. A pipe is not a
// screen. What comes out of it is filed, pasted into a report, read by
// somebody who was not here, and a page rendered at eighty columns
// because eighty was the floor arrives with the kernel, the uptime,
// the swap and the path of the binary each cut off mid-value. Those
// are the readings the page exists to carry.
//
// The arithmetic is the layout's, backwards. A fact sits in one of two
// columns of half the measure, its value past the label and a space; a
// check runs from the margin to the leaders, which stop short of the
// widest status.
func wide(r report, own check) int {
	measure := minCols - 2*margin
	for _, side := range [][]fact{r.system, r.login} {
		for _, f := range side {
			measure = max(measure, 2*(utf8.RuneCountInString(cased(f.value, f.path))+factCol+2))
		}
	}
	for _, k := range append([]check{own}, r.checks...) {
		measure = max(measure, utf8.RuneCountInString(cased(k.value, k.path))+checkCol+statusW+4)
	}
	return measure + 2*margin
}

// A canvas takes rows of one width in one palette.
type canvas struct {
	p     palette
	width int
	rows  []row
}

// A line is built from painted pieces; cells counts the columns. A mark
// is set in the margin, before the line, where there is no color to say
// which row is the cursor's — see palette.chosen.
type line struct {
	p     palette
	b     strings.Builder
	cells int
	mark  string
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
// that centers it — the pieces, and the ground to the edge. A marked
// row carries its mark at the very edge, the margin after it. In the
// plain palette the ground is nothing, and the row ends with its last
// piece.
func (c *canvas) emit(l *line, stage int, centered bool) {
	p := l.p
	left := margin
	if centered {
		left = max((c.width-l.cells)/2, 0)
	}
	lead := strings.Repeat(" ", left)
	if l.mark != "" && !centered {
		lead = p.orange + p.bold + l.mark + p.normal + strings.Repeat(" ", margin-1)
	}
	text := p.normal + lead + l.b.String() + strings.Repeat(" ", max(c.width-left-l.cells, 0)) + p.end
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

// stdoutIsTerminal says whether what conn prints is going to a person's
// screen, or to a pipe or file.
func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// stdinIsTerminal says whether there is somebody there to answer.
func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
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
