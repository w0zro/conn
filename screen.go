package main

import (
	"strconv"
	"strings"

	"github.com/w0zro/conn/internal/draw"

	"github.com/charmbracelet/x/ansi"
)

// The boot console, as the design hands it off and the brief has grown
// it: the wordmark with the station's identification beside it; under a
// rule, the readout — the system on the left, the session on the right;
// the start-up checks, one status column against the right edge; and
// under a second rule the verdict, pulled tight — the count of faults as
// a chip, or what the checks came to. On a terminal the bottom row
// waits on a key. Uppercase throughout, by design; a path keeps its own
// case, since its case is part of it.

// The console is set to the measure: the readout in two columns of
// half of it, the checks' statuses flush with its right edge. These are
// the fixed columns; the rows it needs depend on the report.
const (
	checkCol   = 12 // a check's value, from the margin
	StatusW    = 9  // the widest status: UNCHECKED, NOT A DIR
	stationGap = 5  // between the wordmark and the station block
)

// columns are the measure's, for a terminal width columns wide: the
// second column of the readout, and where a check's leaders stop, short
// of the widest status and a space.
func columns(width int) (measure, rightCol, leaderEnd int) {
	measure = draw.MeasureOf(width)
	return measure, measure / 2, measure - StatusW - 2
}

// The stages: the header at once, the readout, each check in turn, and
// the verdict last.
const (
	StageHeader = iota
	StageReadout
	StageChecks // the first check; each after is one more
)

// LastStage is the verdict's, after the screen's check and the report's.
func LastStage(r Report) int {
	return StageChecks + 1 + len(r.checks)
}

// rowsNeeded is how many rows the console takes for a report: the
// body, the verdict its last row. It is counted off the layout, not
// summed by hand. The key bar at the foot says how to go on, so the
// console has no row of its own to say it.
func rowsNeeded(r Report) int {
	return len(body(r, draw.MinCols, check{}, draw.Plain))
}

// Screen renders the console for a terminal of the given size, in the
// palette: rows the terminal's width, painted on the ground, to its
// height. Off a terminal, height is 0, and the rows are the body alone.
// A terminal too small for the body gets the small console instead.
func Screen(r Report, width, height int, p draw.Palette) []draw.Row {
	r = Fitted(r, height)
	need := rowsNeeded(r)
	own := screenCheck(r.term, width, height, need)
	if own.fault {
		return small(own, width, height, need, p)
	}
	cols := max(width, draw.MinCols)
	if height == 0 {
		cols = wide(r, own)
	}
	rows := body(r, cols, own, p)
	if height > 0 {
		c := draw.Canvas{P: p, Width: max(width, draw.MinCols), Rows: rows}
		for len(c.Rows) < height {
			c.Blank(LastStage(r))
		}
		rows = c.Rows
	}
	return rows
}

// Fitted is the report as a terminal of this height can hold it. The
// roots are a line each, which is how a root that is not there says so
// on its own account; where the rows for that are not there they become
// one line carrying all of them, under the worst word any of them
// earned. The console already cuts a value to the width it was given,
// and this is the same bargain in the other direction — better a
// console that says less than one that refuses to draw because a root
// was added. Off a terminal, height is 0 and nothing is given up.
//
// It is idempotent: a report already Fitted is returned as it is.
func Fitted(r Report, height int) Report {
	if height <= 0 || rowsNeeded(r) <= height {
		return r
	}
	return joinRoots(r)
}

// joinRoots is the report with its ROOT lines made one ROOTS line. The
// paths are joined the way conn joins values, and the status is the
// worst of them: the line stands for all of them, so it cannot say
// NOMINAL while one of them is missing.
func joinRoots(r Report) Report {
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
func body(r Report, width int, own check, p draw.Palette) []draw.Row {
	measure, rightCol, leaderEnd := columns(width)
	c := draw.Canvas{P: p, Width: width}

	// The header: the wordmark, the station block beside it on its first
	// rows, and a rule under it.
	c.Blank(StageHeader)
	markW := ansi.StringWidth(wordmark[0])
	station := [][2]string{
		{p.Bold, strings.TrimSpace("CONN " + r.version)},
		{p.Gray, "STATION  " + strings.ToUpper(r.station)},
		{p.Gray, strings.ToUpper(r.clock)},
		{p.Gray, r.build},
	}
	for i, m := range wordmark {
		l := c.Line()
		l.Add(p.Orange+p.Bold, m)
		if i < len(station) && station[i][1] != "" {
			l.To(markW + stationGap)
			l.Add(station[i][0], station[i][1])
			if i == 0 && r.note != "" {
				l.Add(p.Gray, " "+r.note)
			}
		}
		c.Emit(l, StageHeader, false)
	}
	c.Rule(StageHeader, measure)

	// The readout: the machine in the left column, the session in the
	// right, each under its title. The left column was titled SYSTEM
	// until the host came off it, which left a row labelled SYSTEM
	// directly under a title of the same word. The column is the
	// machine and the row is the operating system on it, and now each
	// says which it is.
	factLine := func(l *draw.Line, col, width int, f draw.Fact) {
		l.To(col)
		l.Leader(strings.ToUpper(f.Label), col+draw.FactCol-1, p.Faint)
		l.Add(p.Ink, draw.Fit(draw.Cased(f.Value, f.Path), width-draw.FactCol-2, f.Path))
	}
	l := c.Line()
	l.Title(0, "MACHINE")
	l.Title(rightCol, "SESSION")
	c.Emit(l, StageReadout, false)
	for i := 0; i < max(len(r.system), len(r.login)); i++ {
		l := c.Line()
		if i < len(r.system) {
			factLine(l, 0, rightCol, r.system[i])
		}
		if i < len(r.login) {
			factLine(l, rightCol, measure-rightCol, r.login[i])
		}
		c.Emit(l, StageReadout, false)
	}

	// The checks: the title, then the screen's own line and the report's,
	// each to the status column. A fault's chip blinks with the verdict's;
	// what is nominal stays put.
	c.Blank(StageChecks)
	l = c.Line()
	l.Title(0, "START-UP CHECKS")
	c.Emit(l, StageChecks, false)
	var stood tally
	for i, k := range append([]check{own}, r.checks...) {
		l := c.Line()
		l.Leader(strings.ToUpper(k.label), checkCol-1, p.Gray)
		l.Add(p.Ink, draw.Fit(draw.Cased(k.value, k.path), leaderEnd-checkCol-2, k.path))
		l.Add("", " ")
		l.Add(p.Border, strings.Repeat(".", max(leaderEnd-l.Cells, 1)))
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
			l.To(measure - StatusW + (StatusW-ansi.StringWidth(word))/2)
			l.Add(p.Gray, word)
		} else if r.Lit {
			word = " " + word + " "
			l.To(measure - StatusW + (StatusW-ansi.StringWidth(word))/2)
			l.Add(p.Chip, word)
		}
		c.Emit(l, StageChecks+i, false)
	}

	// The verdict: a rule, then the count of faults as a chip, or what
	// the checks came to. The chip is an annunciator and blinks, a
	// second lit against half of one dark, and the faults' own chips
	// blink with it; on the dark half those cells are the ground, and
	// nothing around them moves.
	last := LastStage(r)
	c.Blank(last)
	c.Rule(last, measure)
	l = c.Line()
	switch {
	case stood.off == 0:
		l.Add(p.Gray, AllNominal)
	case !r.Lit:
		// dark this second
	case stood.off > 1:
		l.Add(p.Chip, " "+strconv.Itoa(stood.off)+" SYSTEMS NOT NOMINAL ")
	default:
		l.Add(p.Chip, " 1 SYSTEM NOT NOMINAL ")
	}
	c.Emit(l, last, true)
	return c.Rows
}

// AllNominal is the one sentence a console with nothing to report says.
const AllNominal = "ALL SYSTEMS NOMINAL"

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
func small(own check, width, height, need int, p draw.Palette) []draw.Row {
	c := draw.Canvas{P: p, Width: width}
	measure, _, _ := columns(width)
	c.Blank(StageHeader)
	if markW := ansi.StringWidth(wordmark[0]); measure >= markW && height >= len(wordmark)+5 {
		for _, m := range wordmark {
			l := c.Line()
			l.Add(p.Orange+p.Bold, m)
			c.Emit(l, StageHeader, false)
		}
	} else {
		l := c.Line()
		l.Add(p.Orange+p.Bold, "CONN")
		c.Emit(l, StageHeader, false)
	}
	c.Blank(StageHeader)
	l := c.Line()
	l.Add(p.Gray, "SCREEN ")
	l.Add(p.Ink, draw.Fit(strings.ToUpper(own.value), measure-len("SCREEN ")-len(" SMALL ")-1, false))
	l.To(measure - len(" SMALL "))
	l.Add(p.Chip, " SMALL ")
	c.Emit(l, StageHeader, false)
	l = c.Line()
	l.Add(p.Gray, "NEEDS ")
	l.Add(p.Ink, strconv.Itoa(draw.MinCols)+"×"+strconv.Itoa(need))
	c.Emit(l, StageHeader, false)
	for height > 0 && len(c.Rows) < height {
		c.Blank(StageHeader)
	}
	return c.Rows
}

// screenCheck is the screen itself: its size, what the terminal calls
// itself, and whether the console fits. Off a terminal there is no
// screen to check.
func screenCheck(term string, width, height, need int) check {
	if height == 0 {
		return check{label: "SCREEN", value: draw.Join(" · ", "NO TERMINAL", term), status: unchecked}
	}
	value := draw.Join(" · ", strconv.Itoa(width)+"×"+strconv.Itoa(height), term)
	if width < draw.MinCols || height < need {
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
func wide(r Report, own check) int {
	measure := draw.MinCols - 2*draw.Margin
	for _, side := range [][]draw.Fact{r.system, r.login} {
		for _, f := range side {
			measure = max(measure, 2*(ansi.StringWidth(draw.Cased(f.Value, f.Path))+draw.FactCol+2))
		}
	}
	for _, k := range append([]check{own}, r.checks...) {
		measure = max(measure, ansi.StringWidth(draw.Cased(k.value, k.path))+checkCol+StatusW+4)
	}
	return measure + 2*draw.Margin
}
