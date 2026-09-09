package main

import (
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The boot console, as the design hands it off: the wordmark with the
// station's identification beside it; under a rule, the system block,
// eight facts in two columns; the start-up checks, five lines to one
// status column; and under a second rule the verdict band, pulled tight
// — the count of faults as a chip with what the first one means for conn,
// then the greeting. Uppercase throughout, by design.

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

// The measure the console is set to, from a left margin; the columns
// within it; and the size under which the terminal is called small.
const (
	margin     = 3
	measure    = 72
	factCol    = 10 // a fact's value, from the margin
	rightCol   = 36 // the second column of facts
	checkCol   = 12 // a check's value
	leaderEnd  = 54 // where a check's leaders stop
	statusCol  = 56 // a check's status
	minCols    = 80
	minRows    = 24
	stationGap = 5 // between the wordmark and the station block
)

// A row of the console and the stage of the sequence it comes on at.
type row struct {
	text  string
	stage int
}

// The stages: the header at once, the system block, each check in turn,
// and the verdict. lastStage is the verdict's.
const (
	stageHeader = iota
	stageSystem
	stageChecks // the first check; each after is one more
	checkCount  = 5
	lastStage   = stageChecks + checkCount
)

// screen renders the console for a terminal of the given size: rows the
// terminal's width, painted on the ground, to its height.
func screen(r report, width, height int) []row {
	p := pal
	width = max(width, minCols)
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
	fit := func(s string, w int) string {
		if utf8.RuneCountInString(s) <= w {
			return s
		}
		if w <= 1 {
			return ""
		}
		return string([]rune(s)[:w-1]) + "…"
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
	}
	for i, m := range wordmark {
		var l line
		add(&l, p.orange+p.bold, m)
		if i < len(station) {
			to(&l, markW+stationGap)
			add(&l, station[i][0], station[i][1])
			if i == 0 && r.note != "" {
				add(&l, p.gray, " "+r.note)
			}
		}
		emit(&l, stageHeader, false)
	}
	rule(stageHeader)

	// The system block: the title and the facts, four to a column.
	{
		var l line
		add(&l, p.parchment+p.bold, "SYSTEM")
		emit(&l, stageSystem, false)
	}
	factLine := func(l *line, col int, f fact) {
		to(l, col)
		leader(l, strings.ToUpper(f.label), col+factCol-1, p.faint)
		if f.value != "" {
			add(l, p.ink, fit(strings.ToUpper(f.value), rightCol-factCol-2))
		}
		if f.anomaly != "" {
			if f.value != "" {
				add(l, p.gray, " · ")
			}
			add(l, p.owed, strings.ToUpper(f.anomaly))
		}
	}
	for i := 0; i < 4; i++ {
		var l line
		if i < len(r.facts) {
			factLine(&l, 0, r.facts[i])
		}
		if 4+i < len(r.facts) {
			factLine(&l, rightCol, r.facts[4+i])
		}
		emit(&l, stageSystem, false)
	}

	// The checks: the title, then the terminal's own line and the four
	// from the report, each to the status column.
	blank(stageChecks)
	{
		var l line
		add(&l, p.parchment+p.bold, "START-UP CHECKS")
		emit(&l, stageChecks, false)
	}
	checks := append([]check{terminalCheck(r.term, width, height)}, r.checks...)
	faults, consequence := 0, ""
	for i, c := range checks {
		var l line
		leader(&l, strings.ToUpper(c.label), checkCol-1, p.gray)
		add(&l, p.ink, fit(strings.ToUpper(c.value), leaderEnd-checkCol-2))
		add(&l, "", " ")
		add(&l, p.border, strings.Repeat(".", max(leaderEnd-l.cells, 1)))
		if c.fault {
			faults++
			if consequence == "" {
				consequence = c.consequence
			}
			to(&l, statusCol-1)
			add(&l, p.chip, " "+strings.ToUpper(c.status)+" ")
		} else {
			to(&l, statusCol)
			add(&l, p.gray, strings.ToUpper(c.status))
		}
		emit(&l, stageChecks+i, false)
	}

	// The verdict band: a rule, the count of faults and what the first
	// means, and the greeting. All nominal, it is the greeting alone.
	blank(lastStage)
	rule(lastStage)
	if faults > 0 {
		var l line
		count := "1 SYSTEM NOT NOMINAL"
		if faults > 1 {
			count = strconv.Itoa(faults) + " SYSTEMS NOT NOMINAL"
		}
		add(&l, p.chip, " "+count+" ")
		if consequence != "" {
			add(&l, "", "  ")
			add(&l, p.gray, strings.ToUpper(consequence))
		}
		emit(&l, lastStage, true)
	}
	{
		var l line
		add(&l, p.orange, "***")
		add(&l, p.gray, "  "+strings.ToUpper(greeting)+"  ")
		add(&l, p.orange, "***")
		emit(&l, lastStage, true)
	}
	for len(rows) < height {
		blank(lastStage)
	}
	return rows
}

// terminalCheck is the terminal itself: what it calls itself, its size,
// and whether the console fits it.
func terminalCheck(term string, width, height int) check {
	value := join(" · ", term, strconv.Itoa(width)+"×"+strconv.Itoa(height))
	if width < minCols || height < minRows {
		return check{label: "TERMINAL", value: value, status: "SMALL", fault: true,
			consequence: "TERMINAL SMALL — CONN RUNS, THE BOARD WILL BE CRAMPED"}
	}
	return check{label: "TERMINAL", value: value, status: nominal}
}

// stdoutIsTerminal says whether what conn prints is going to a person's
// screen, or to a pipe or file.
func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
