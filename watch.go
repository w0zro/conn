package main

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// The watch: what is running, by place. Under a short header, each
// place work is happening in is a block — its path as a title, and a
// row for each process that stands for work there: its kind, what it
// was started as, its terminal, how long it has been at it, and the
// word for how it stands. The newest work is at the top. A cursor
// is on one row, which is drawn on a raised ground from edge to edge,
// and the rows scroll to keep it in view. A row conn holds — one it can
// reach, because its process is in a pane of the server — carries a
// rule down the margin; the rest are work conn can only report. The bottom
// row is kept clear for a note — what went wrong reaching something —
// and holds nothing otherwise. In the rail, which is narrower than the
// console, the terminal column is left off and the rest close up; the
// row on the right, in the slot, is in orange.

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
	here                            bool
	reach                           string // the pane that holds it, in conn's server
	shown                           bool   // it is in the slot, on the right
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
				status: e.status, fault: e.fault, here: e.status == statusHere, reach: panes[e.tty].id,
				shown: slot != "" && e.tty == slot,
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
			command, kind, word := p.ink, p.gray, p.gray
			if r.shown || r.here {
				kind, word = p.orange+p.bold, p.orange+p.bold
			}
			// A row conn holds — its process is in a pane of the server, so
			// it can be reached — carries a rule down the margin. A run of
			// them draws one line, which is what conn holds at that place.
			if r.reach != "" {
				l.rule = "│"
			}
			if r.pid == cursor {
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
			l.add(kind, fit(r.kind, kindCol-1, false))
			l.to(kindCol)
			l.add(command, fit(r.command, commandW, false))
			if !rail {
				l.to(ttyCol)
				// A terminal the server holds is in gray; one it does not,
				// and so cannot be reached, is faint.
				ttyColor := p.gray
				if b.inside && r.reach == "" {
					ttyColor = p.faint
				}
				l.add(ttyColor, fit(strings.ToUpper(r.tty), ttyW, false))
			}
			l.to(ageCol)
			l.add(p.gray, r.age)
			if r.fault {
				l.to(measure - utf8.RuneCountInString(r.status) - 2)
				l.add(p.chip, " "+r.status+" ")
			} else {
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
	// What will not fit scrolls, so the cursor's row is in view, and the
	// rows out of view are counted on the last row.
	if height > 0 && len(c.rows)+len(body) > room {
		visible := max(room-len(c.rows)-1, 0)
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
		body = append(body, d.rows...)
	}
	c.rows = append(c.rows, body...)

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
