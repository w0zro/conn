package main

import (
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
// holding their own place. Everything sits where it started and stays
// there for as long as it lives, oldest first, so what is new goes on
// the end and nothing above it moves. A cursor is on one row, which is
// drawn on a raised ground from edge to edge, and the rows scroll to
// keep it in view. What conn holds — a process in a pane of the
// server, which can be reached — is written in the ink; work conn can
// only report is dimmed a rank. In the rail, which is narrower than
// the console, the terminal column is left off and the rest close up;
// the row on the right, in the slot, is in orange.

// The watch's words, composed from the places as of a moment.
type watchReport struct {
	places []watchPlace
	err    string // why the table could not be read, when it could not
	inside bool   // conn is in its server, and rows can be reached
	lit    bool   // the annunciators' lit half; see the waiting word below
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

// headOf is the first row of a terminal in the places as read: the
// process its pane was opened on, which everything else in that pane
// hangs under. It is what the slot's mark goes on and what the cursor
// belongs on once the pane is reached, and both ask here so that the
// two can never disagree about which row the pane is. It answers the
// row's place in the reading too, for the cursor to hold.
func headOf(places []place, tty string) (pid, at int, ok bool) {
	if tty == "" {
		return 0, 0, false
	}
	i := 0
	for _, pl := range places {
		for _, e := range pl.entries {
			if e.tty == tty {
				return e.pid, i, true
			}
			i++
		}
	}
	return 0, 0, false
}

// rowOf is an entry by its pid, wherever it stands.
func rowOf(places []place, pid int) (entry, bool) {
	for _, pl := range places {
		for _, e := range pl.entries {
			if e.pid == pid {
				return e, true
			}
		}
	}
	return entry{}, false
}

// composeWatch words the places; panes says which terminals are the
// server's, and slot which of them is on the right.
//
// A pane holds a whole tree, and all of it is equally in the slot, but
// saying so on every row of it paints a block rather than a mark. Only
// the head of that tree is marked shown. What hangs under it reads as
// what it is: in a pane conn holds, like any other row conn can reach.
func composeWatch(places []place, panes map[string]pane, slot string, roots []string, home string, now time.Time, err string) watchReport {
	b := watchReport{err: err}
	head, _, marked := headOf(places, slot)
	for _, pl := range places {
		bp := watchPlace{path: placeName(pl.path, roots, home)}
		if bp.path == "" {
			bp.path = "NO PROJECT"
		}
		for _, e := range pl.entries {
			bp.rows = append(bp.rows, watchRow{
				pid: e.pid, kind: e.kind, command: e.command, tty: e.tty, age: age(e.started, now),
				status: e.status, fault: e.fault, reach: panes[e.tty].id,
				shown: marked && e.pid == head, depth: e.depth,
			})
		}
		b.places = append(b.places, bp)
	}
	return b
}

// placeName is what the watch writes over a block: what is left of the
// path once the root the checkouts are kept under is taken off it.
// ~/projects/w0zro/conn is w0zro/conn. The root is the same for every
// project on the list and says nothing that tells one from another, and
// it is said at the head of every block — the rail is forty-four columns
// wide, and the part that tells them apart is the part that should have
// them.
//
// A place outside every root is written from ~ and whole: there is
// nothing shared to take off it, and where it is is the only thing the
// line has to say. A root itself is written the same way, since what is
// left of it after itself is nothing.
func placeName(path string, roots []string, home string) string {
	for _, root := range roots {
		if path != root && within(path, root) {
			return relName(root, path)
		}
	}
	return tilde(path, home)
}

// The watch's columns, from the right: the status flush with the
// measure, the age and the terminal before it, and the command taking
// what is left after the kind. Under minCols the watch is a rail: the
// terminal column goes, the kind and the age close up.
const (
	kindW       = 8
	ttyW        = 10
	ageW        = 9
	railKindW   = 8
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

	// The header: a rule and the column heads. The view goes unlabeled:
	// it is what conn is when it is up. The name stood over this row and
	// is the bar's now, at the bottom left of the window where a name
	// belongs — it is the whole program's and not the watch's, and the
	// watch is the one view that was carrying it for all of them.
	c.blank(0)
	c.rule(0, measure)
	l := c.line()
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

	// The places, in the order work began in them; or the reason there
	// are none.
	room := height
	if height == 0 {
		room = 1 << 30
	}
	var body []row
	cursorRow := -1
	place := func(bp watchPlace) {
		d := canvas{p: p, width: width}
		d.blank(0)
		// The place's title alone. It carried a count of its rows on the
		// right, which was the kernel's word for them and a figure the
		// operator never asks for: the rows are right there under it.
		l := d.line()
		l.add(p.parchment+p.bold, fit(bp.path, measure, true))
		d.emit(l, 0, false)
		for _, r := range bp.rows {
			l := d.line()
			cursored := r.pid == cursor
			// What conn can do with a row is said two ways, and they are
			// not the same kind of saying. Dimming is a rank the whole
			// row drops: what conn can only report — a terminal it did
			// not open, and cannot attach to — goes faint in every
			// column, since the rest of them are the quiet gray already
			// and dimming one of six says nothing. Outside its server
			// conn holds nothing, so nothing is dimmed: the distinction
			// would be every row.
			//
			// The slot is a mark, and one cell of one row is all a mark
			// needs: the kind of the head of what is in the slot, in the
			// orange, which is "you, here" everywhere else in conn. A
			// row is a lot of orange, and the status column especially
			// is not the orange's to take — WAITING is already a color
			// close to it, and the two together say neither.
			kind, command, ttyColor, ageColor, word := p.gray, p.ink, p.gray, p.gray, p.gray
			switch {
			case r.shown:
				kind = p.orange + p.bold
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
				// fault, so it takes the color rather than the chip, and
				// it blinks, which is the one thing on a screen that
				// reaches the corner of an eye. Reading down a list of
				// rows that all say something, the row that wants you is
				// the row that moves.
				//
				// On the dark half the cells are the ground and nothing
				// around them moves, the way the console's verdict goes
				// dark: a word that jumped its neighbours about would be
				// worse than one that never blinked.
				if b.lit {
					l.to(measure - utf8.RuneCountInString(r.status))
					l.add(p.waiting+p.bold, r.status)
				}
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
		l.add(p.gray, "NO PROCESSES")
		d.emit(l, 0, true)
		body = d.rows
	default:
		for _, bp := range b.places {
			place(bp)
		}
	}
	c.rows = append(c.rows, scrolled(body, cursorRow, room-len(c.rows), width, p)...)

	// The ground fills what the rows do not: the keys are learned once,
	// and a legend on every row of every reading is a thing to read
	// past forever.
	if height > 0 {
		for len(c.rows) < height {
			c.blank(0)
		}
	}
	return c.rows
}
