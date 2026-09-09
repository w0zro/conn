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
// word for how it stands. The newest work is at the top. The bottom
// row says which keys the watch answers to.

// The watch's words, composed from the places as of a moment.
type watchReport struct {
	station, clock string
	places         []watchPlace
	err            string // why the table could not be read, when it could not
}

type watchPlace struct {
	path string
	rows []watchRow
}

type watchRow struct {
	kind, command, tty, age, status string
	fault                           bool
	here                            bool
}

// composeWatch words the places.
func composeWatch(places []place, home string, now time.Time, station, clock, err string) watchReport {
	b := watchReport{station: station, clock: clock, err: err}
	for _, pl := range places {
		bp := watchPlace{path: tilde(pl.path, home)}
		if bp.path == "" {
			bp.path = "NO PLACE"
		}
		for _, e := range pl.entries {
			bp.rows = append(bp.rows, watchRow{
				kind: e.kind, command: e.command, tty: e.tty, age: age(e.started, now),
				status: e.status, fault: e.fault, here: e.status == statusHere,
			})
		}
		b.places = append(b.places, bp)
	}
	return b
}

// The watch's columns, from the right: the status flush with the
// measure, the age and the terminal before it, and the command taking
// what is left after the kind.
const (
	kindW    = 8
	ttyW     = 10
	ageW     = 9
	watchKey = "Q CLOSES · C CONSOLE"
)

// drawWatch renders the watch for a terminal of the given size.
func drawWatch(b watchReport, width, height int, p palette) []row {
	width = max(width, minCols)
	measure, _, _ := columns(width)
	c := canvas{p: p, width: width}
	statusCol := measure - statusW
	ageCol := statusCol - 1 - ageW
	ttyCol := ageCol - 1 - ttyW
	commandW := ttyCol - 1 - kindW

	// The header: the name, the view, and the station and clock against
	// the right; a rule; the column heads.
	c.blank(0)
	l := c.line()
	l.add(p.orange+p.bold, "CONN")
	l.add(p.parchment+p.bold, "  WATCH")
	right := strings.ToUpper(join("  ·  ", b.station, b.clock))
	l.to(measure - utf8.RuneCountInString(right))
	l.add(p.gray, right)
	c.emit(l, 0, false)
	c.rule(0, measure)
	l = c.line()
	l.add(p.gray, "KIND")
	l.to(kindW)
	l.add(p.gray, "COMMAND")
	l.to(ttyCol)
	l.add(p.gray, "TTY")
	l.to(ageCol)
	l.add(p.gray, "AGE")
	l.to(statusCol + statusW - len("STATUS"))
	l.add(p.gray, "STATUS")
	c.emit(l, 0, false)

	// The places, newest first; or the reason there are none.
	room := height - 1 // the bottom row is the keys
	if height == 0 {
		room = 1 << 30
	}
	var body []row
	place := func(bp watchPlace) {
		d := canvas{p: p, width: width}
		d.blank(0)
		l := d.line()
		l.add(p.parchment+p.bold, bp.path)
		count := strconv.Itoa(len(bp.rows)) + " PROCESS"
		if len(bp.rows) != 1 {
			count += "ES"
		}
		l.to(measure - utf8.RuneCountInString(count))
		l.add(p.gray, count)
		d.emit(l, 0, false)
		for _, r := range bp.rows {
			l := d.line()
			l.add(p.gray, r.kind)
			l.to(kindW)
			l.add(p.ink, fit(r.command, commandW, false))
			l.to(ttyCol)
			l.add(p.gray, fit(strings.ToUpper(r.tty), ttyW, false))
			l.to(ageCol)
			l.add(p.gray, r.age)
			switch {
			case r.fault:
				l.to(measure - utf8.RuneCountInString(r.status) - 2)
				l.add(p.chip, " "+r.status+" ")
			case r.here:
				l.to(measure - utf8.RuneCountInString(r.status))
				l.add(p.orange+p.bold, r.status)
			default:
				l.to(measure - utf8.RuneCountInString(r.status))
				l.add(p.gray, r.status)
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
	// What will not fit is counted on the last row that does.
	if over := len(c.rows) + len(body) - room; over > 0 && height > 0 {
		keep := max(room-len(c.rows)-1, 0)
		hidden := len(body) - keep
		body = body[:keep]
		d := canvas{p: p, width: width}
		l := d.line()
		l.add(p.gray, "… "+strconv.Itoa(hidden)+" MORE ROWS")
		d.emit(l, 0, false)
		body = append(body, d.rows...)
	}
	c.rows = append(c.rows, body...)

	// The keys, on the bottom row.
	if height > 0 {
		for len(c.rows) < height-1 {
			c.blank(0)
		}
		l := c.line()
		l.add(p.gray, watchKey)
		c.emit(l, 0, true)
	}
	return c.rows
}
