package main

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// The watch: what is running, by place. Under a short header, each
// place work is happening in is a block — its path as a title, and a
// row for each process that stands for its own work there, nested
// under whatever runs it the way the processes actually are: its
// kind, what it was started as, its terminal, how long it has been at
// it, and the word for how it stands. A row under another indents,
// its kind and command shifted in together, the rest of its columns
// holding their own place. The newest work anywhere in a tree brings
// it, and its place, to the top. A cursor is on one row, which is
// drawn on a raised ground from edge to edge, and the rows scroll to
// keep it in view. What conn holds — a process in a pane of the
// server, which can be reached — is written in the ink; work conn can
// only report is dimmed a rank. The bottom row is kept clear for a
// note — what went wrong reaching something — and holds nothing
// otherwise. In the rail, which is narrower than the console, the
// terminal column is left off and the rest close up; the row on the
// right, in the slot, is in orange.

// The watch's words, composed from the places as of a moment.
type watchReport struct {
	station, clock string
	places         []watchPlace
	err            string // why the table could not be read, when it could not
	inside         bool   // conn is in its server, and rows can be reached
	note           string // a word for the bottom row, until a key
}

type watchPlace struct {
	path string
	rows []watchRow
}

type watchRow struct {
	pid                             int
	kind, command, tty, age, status string
	fault                           bool
	reach                           string // the pane that holds it, in conn's server
	shown                           bool   // it is in the slot, on the right
	depth                           int    // how deep under its place's own root
}

// composeWatch words the places; panes says which terminals are the
// server's, and slot which of them is on the right.
func composeWatch(places []place, panes map[string]pane, slot string, home string, now time.Time, station, clock, err string) watchReport {
	b := watchReport{station: station, clock: clock, err: err}
	for _, pl := range places {
		bp := watchPlace{path: tilde(pl.path, home)}
		if bp.path == "" {
			bp.path = "NO PLACE"
		}
		for _, e := range pl.entries {
			bp.rows = append(bp.rows, watchRow{
				pid: e.pid, kind: e.kind, command: e.command, tty: e.tty, age: age(e.started, now),
				status: e.status, fault: e.fault, reach: panes[e.tty].id,
				shown: slot != "" && e.tty == slot, depth: e.depth,
			})
		}
		b.places = append(b.places, bp)
	}
	return b
}

// The watch's columns, from the right: the status flush with the
// measure, the age and the terminal before it, and the command taking
// what is left after the kind. Under minCols the watch is a rail: the
// terminal column goes, the kind and the age close up.
const (
	kindW       = 8
	ttyW        = 10
	ageW        = 9
	railKindW   = 7
	railAgeW    = 7
	railMinCols = 40
	treeIndent  = 2 // columns a row gives up per level under its root
)

// drawWatch renders the watch for a terminal of the given size, with
// the cursor on the row of the given pid.
func drawWatch(b watchReport, cursor int, width, height int, p palette) []row {
	rail := width < minCols
	width = max(width, railMinCols)
	measure, _, _ := columns(width)
	c := canvas{p: p, width: width}
	statusCol := measure - statusW
	ageCol := statusCol - 1 - ageW
	ttyCol := ageCol - 1 - ttyW
	commandW := ttyCol - 1 - kindW
	kindCol := kindW
	if rail {
		ageCol = statusCol - 1 - railAgeW
		ttyCol = -1
		kindCol = railKindW
		commandW = ageCol - 1 - kindCol
	}

	// The header: the name, and the station and clock against the right;
	// a rule; the column heads. The view goes unlabeled: it is what conn
	// is when it is up.
	c.blank(0)
	l := c.line()
	l.add(p.orange+p.bold, "CONN")
	right := strings.ToUpper(join("  ·  ", b.station, b.clock))
	if rail {
		_, right, _ = strings.Cut(strings.ToUpper(b.clock), "  ")
	}
	l.to(measure - utf8.RuneCountInString(right))
	l.add(p.gray, right)
	c.emit(l, 0, false)
	c.rule(0, measure)
	l = c.line()
	l.add(p.gray, "KIND")
	l.to(kindCol)
	l.add(p.gray, "COMMAND")
	if !rail {
		l.to(ttyCol)
		l.add(p.gray, "TTY")
	}
	l.to(ageCol)
	l.add(p.gray, "AGE")
	l.to(statusCol + statusW - len("STATUS"))
	l.add(p.gray, "STATUS")
	c.emit(l, 0, false)

	// The places, newest first; or the reason there are none.
	room := height - 1 // the bottom row is kept for a note
	if height == 0 {
		room = 1 << 30
	}
	var body []row
	cursorRow := -1
	place := func(bp watchPlace) {
		d := canvas{p: p, width: width}
		d.blank(0)
		l := d.line()
		count := strconv.Itoa(len(bp.rows)) + " PROCESS"
		if len(bp.rows) != 1 {
			count += "ES"
		}
		l.add(p.parchment+p.bold, fit(bp.path, measure-utf8.RuneCountInString(count)-2, true))
		l.to(measure - utf8.RuneCountInString(count))
		l.add(p.gray, count)
		d.emit(l, 0, false)
		for _, r := range bp.rows {
			l := d.line()
			cursored := r.pid == cursor
			// Three tiers, by what conn can do with the row. The one in
			// the slot is the orange — it is what you are looking at, and
			// the orange is "you, here" everywhere else in conn. What conn
			// holds a pane for is the ink: it can be reached, put in the
			// slot and come back to. What conn can only report is a rank
			// down, the whole row and not the command alone, since the
			// rest of the columns are the quiet gray already and dimming
			// one of six says nothing. Outside its server conn holds
			// nothing, so nothing is dimmed: the distinction would be
			// every row.
			kind, command, ttyColor, ageColor, word := p.gray, p.ink, p.gray, p.gray, p.gray
			switch {
			case r.shown:
				kind, word = p.orange+p.bold, p.orange+p.bold
				command, ttyColor, ageColor = p.orange, p.orange, p.orange
			case b.inside && r.reach == "":
				dim := p.faint
				if cursored {
					// Faint on the raised ground is barely there. The row
					// under the cursor is the one being read, so it gives
					// up a rank of the dimming rather than the reading.
					dim = p.gray
				}
				kind, command, ttyColor, ageColor, word = dim, dim, dim, dim, dim
			}
			if cursored {
				// The row under the cursor is the one on the raised ground,
				// edge to edge; where there is no color to raise it, it takes
				// a mark in the margin instead.
				l.p = p.chosen()
				if p.plain {
					l.mark = "▸"
				}
				command += p.bold
				cursorRow = len(body) + len(d.rows)
			}
			// A row under another indents, kind and command shifted in
			// together; the command gives up what the indent takes; a
			// tree too deep for the room there is stops taking more.
			indent := min(r.depth*treeIndent, max(commandW-4, 0))
			l.to(indent)
			l.add(kind, fit(r.kind, kindCol-1, false))
			l.to(kindCol + indent)
			l.add(command, fit(r.command, commandW-indent, false))
			if !rail {
				l.to(ttyCol)
				l.add(ttyColor, fit(strings.ToUpper(r.tty), ttyW, false))
			}
			l.to(ageCol)
			l.add(ageColor, r.age)
			switch {
			case r.fault:
				l.to(measure - utf8.RuneCountInString(r.status) - 2)
				l.add(p.chip, " "+r.status+" ")
			case r.status == statusWaiting:
				// The one word here that asks something of you, and the
				// only one worth finding without looking: it is not a
				// fault, so it takes the color rather than the chip.
				l.to(measure - utf8.RuneCountInString(r.status))
				l.add(p.owed+p.bold, r.status)
			default:
				l.to(measure - utf8.RuneCountInString(r.status))
				l.add(word, r.status)
			}
			d.emit(l, 0, false)
		}
		body = append(body, d.rows...)
	}
	switch {
	case b.err != "":
		d := canvas{p: p, width: width}
		d.blank(0)
		l := d.line()
		l.add(p.chip, " "+strings.ToUpper(b.err)+" ")
		d.emit(l, 0, true)
		body = d.rows
	case len(b.places) == 0:
		d := canvas{p: p, width: width}
		d.blank(0)
		l := d.line()
		l.add(p.gray, "NOTHING ON WATCH")
		d.emit(l, 0, true)
		body = d.rows
	default:
		for _, bp := range b.places {
			place(bp)
		}
	}
	c.rows = append(c.rows, scrolled(body, cursorRow, room-len(c.rows), width, p)...)

	// The bottom row is a note's, when there is one, and otherwise the
	// ground: the keys are learned once, and a legend on every row of
	// every reading is a thing to read past forever.
	if height > 0 {
		for len(c.rows) < height-1 {
			c.blank(0)
		}
		if b.note == "" {
			c.blank(0)
		} else {
			l := c.line()
			l.add(p.owed, fit(b.note, measure, false))
			c.emit(l, 0, true)
		}
	}
	return c.rows
}
