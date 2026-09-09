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
// a chip, or the word that all is well. Uppercase throughout, by design.

// The palette is the handoff's tokens. Off a terminal every sequence is
// empty and the console is plain text.
type palette struct {
	ground, border, ink, gray, faint, orange, owed, parchment, bold, chip string
	normal, end                                                           string // ink on the ground again; the row's end
}

var (
	plain palette
	pal   = plain
)

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
	statusW    = 9  // the widest status: UNCHECKED
	minCols    = 80
	stationGap = 5 // between the wordmark and the station block
)

// columns are the measure's, for a terminal width columns wide: the
// second column of the readout, and where a check's leaders stop, short
// of the widest status and a space.
func columns(width int) (measure, rightCol, leaderEnd int) {
	measure = max(width, minCols) - 2*margin
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

// rowsNeeded is how many rows the console takes for a report: the header,
// the readout, the checks and the verdict, with their rules and air.
func rowsNeeded(r report) int {
	return 1 + len(wordmark) + 1 + 1 + max(len(r.system), len(r.session)) + 1 + 1 + 1 + len(r.checks) + 1 + 1 + 1
}

// screen renders the console for a terminal of the given size: rows the
// terminal's width, painted on the ground, to its height.
func screen(r report, width, height int) []row {
	p := pal
	own := screenCheck(r, width, height)
	width = max(width, minCols)
	measure, rightCol, leaderEnd := columns(width)
	var rows []row

	// A line is built from painted pieces; cells counts the columns.
	type line struct {
		b     strings.Builder
		cells int
	}
	add := func(l *line, color, s string) {
		if color != "" {
			l.b.WriteString(color)
		}
		l.b.WriteString(s)
		if color != "" {
			l.b.WriteString(p.normal)
		}
		l.cells += utf8.RuneCountInString(s)
	}
	to := func(l *line, col int) {
		if col > l.cells {
			add(l, "", strings.Repeat(" ", col-l.cells))
		}
	}
	// emit frames a line as a row: the margin — or, centered, the column
	// that centers it — the pieces, and the ground to the edge.
	emit := func(l *line, stage int, centered bool) {
		left := margin
		if centered {
			left = max((width-l.cells)/2, 0)
		}
		text := p.normal + strings.Repeat(" ", left) + l.b.String() + strings.Repeat(" ", max(width-left-l.cells, 0)) + p.end
		if p == plain {
			text = strings.TrimRight(text, " ")
		}
		rows = append(rows, row{text: text, stage: stage})
	}
	blank := func(stage int) { emit(&line{}, stage, false) }
	rule := func(stage int) {
		var l line
		add(&l, p.border, strings.Repeat("─", measure))
		emit(&l, stage, false)
	}
	title := func(l *line, col int, s string) {
		to(l, col)
		add(l, p.parchment+p.bold, s)
	}
	// leader is a label and short dots to a field's width.
	leader := func(l *line, label string, field int, dots string) {
		add(l, p.gray, label)
		add(l, "", " ")
		add(l, dots, strings.Repeat(".", max(field-l.cells, 1)))
		add(l, "", " ")
	}

	// The header: the wordmark, the station block beside it on its first
	// rows, and a rule under it.
	blank(stageHeader)
	markW := utf8.RuneCountInString(wordmark[0])
	station := [][2]string{
		{p.bold, strings.TrimSpace("CONN " + r.version)},
		{p.gray, "STATION  " + strings.ToUpper(r.station)},
		{p.gray, strings.ToUpper(r.clock)},
		{p.gray, r.build},
	}
	for i, m := range wordmark {
		var l line
		add(&l, p.orange+p.bold, m)
		if i < len(station) && station[i][1] != "" {
			to(&l, markW+stationGap)
			add(&l, station[i][0], station[i][1])
			if i == 0 && r.note != "" {
				add(&l, p.gray, " "+r.note)
			}
		}
		emit(&l, stageHeader, false)
	}
	rule(stageHeader)

	// The readout: the system in the left column, the session in the
	// right, each under its title.
	factLine := func(l *line, col, width int, f fact) {
		to(l, col)
		leader(l, strings.ToUpper(f.label), col+factCol-1, p.faint)
		add(l, p.ink, fit(strings.ToUpper(f.value), width-factCol-2))
	}
	{
		var l line
		title(&l, 0, "SYSTEM")
		title(&l, rightCol, "SESSION")
		emit(&l, stageReadout, false)
	}
	for i := 0; i < max(len(r.system), len(r.session)); i++ {
		var l line
		if i < len(r.system) {
			factLine(&l, 0, rightCol, r.system[i])
		}
		if i < len(r.session) {
			factLine(&l, rightCol, measure-rightCol, r.session[i])
		}
		emit(&l, stageReadout, false)
	}

	// The checks: the title, then the screen's own line and the report's,
	// each to the status column.
	blank(stageChecks)
	{
		var l line
		title(&l, 0, "START-UP CHECKS")
		emit(&l, stageChecks, false)
	}
	checks := append([]check{own}, r.checks...)
	faults := 0
	for i, c := range checks {
		var l line
		leader(&l, strings.ToUpper(c.label), checkCol-1, p.gray)
		add(&l, p.ink, fit(strings.ToUpper(c.value), leaderEnd-checkCol-2))
		add(&l, "", " ")
		add(&l, p.border, strings.Repeat(".", max(leaderEnd-l.cells, 1)))
		status := strings.ToUpper(c.status)
		if c.fault {
			faults++
			status = " " + status + " "
		}
		to(&l, measure-utf8.RuneCountInString(status))
		if c.fault {
			add(&l, p.chip, status)
		} else {
			add(&l, p.gray, status)
		}
		emit(&l, stageChecks+i, false)
	}

	// The verdict: a rule, then the count of faults as a chip, or the
	// word that all is well.
	last := lastStage(r)
	blank(last)
	rule(last)
	{
		var l line
		switch {
		case faults > 1:
			add(&l, p.chip, " "+strconv.Itoa(faults)+" SYSTEMS NOT NOMINAL ")
		case faults == 1:
			add(&l, p.chip, " 1 SYSTEM NOT NOMINAL ")
		default:
			add(&l, p.gray, "ALL SYSTEMS NOMINAL")
		}
		emit(&l, last, true)
	}
	for len(rows) < height {
		blank(last)
	}
	return rows
}

// screenCheck is the screen itself: its size, what the terminal calls
// itself, and whether the console fits. Off a terminal there is no
// screen to check.
func screenCheck(r report, width, height int) check {
	if height == 0 {
		return check{label: "SCREEN", value: join(" · ", "NO TERMINAL", r.term), status: "UNCHECKED"}
	}
	value := join(" · ", strconv.Itoa(width)+"×"+strconv.Itoa(height), r.term)
	if width < minCols || height < rowsNeeded(r) {
		return check{label: "SCREEN", value: value, status: "SMALL", fault: true}
	}
	return check{label: "SCREEN", value: value, status: nominal}
}

// stdoutIsTerminal says whether what conn prints is going to a person's
// screen, or to a pipe or file.
func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// fit holds a value to w columns. A path, which begins at / or ~, is
// shortened between its head and its end so the name it leads to is what
// survives; anything else is cut at the end. A note after the path, set
// off by " · ", keeps its place.
func fit(s string, w int) string {
	if utf8.RuneCountInString(s) <= w {
		return s
	}
	if w <= 1 {
		return ""
	}
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, "~") {
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
