package main

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// The look: what conn knows about a row, read without entering it. i on
// the watch opens it on the row under the cursor, and esc or i again
// comes back. A row of the watch is six columns wide and has to fit a
// hundred of them on a rail; most of what conn reads of a process does
// not fit in that and is dropped rather than shortened. The look is
// where the dropped part is said — the whole command rather than its
// head, the directory the process is actually in rather than the place
// its tree belongs to, how long it has stood as it does rather than
// only how it stands, and, of an agent stopped on you, what it is
// stopped on.
//
// It is a reading of facts, so it is written the way conn's other
// reading of facts is: a label, a dotted leader, a value, grouped under
// a title. The console says what the machine is in that form, and the
// look says what one row of it is; they are the same instrument
// speaking, and there is no reason for them to speak differently.

// lookReport is the look's words as things stand, about one row.
type lookReport struct {
	pid    int
	gone   bool // the row was there when the look opened, and is not now
	groups []lookGroup
}

// A lookGroup is a title and the facts under it. A group with no facts
// is not drawn: an agent's group on a shell's row would be a heading
// over nothing.
type lookGroup struct {
	title string
	facts []fact
}

// composeLook words one row. pane is what conn holds for the row's
// terminal, and is empty for a terminal conn did not open; inside says
// whether conn is in its server at all, since outside it holding
// nothing is the ordinary case rather than something to say of the row.
func composeLook(e entry, pl place, pane pane, inside bool, home string, now time.Time) lookReport {
	b := lookReport{pid: e.pid}

	// What the agent is stopped on goes first, ahead of what the row
	// is. It is the whole reason to open the page on a waiting row, the
	// one thing the watch has no column wide enough for, and the page
	// is cut off at the terminal's height rather than scrolled — so the
	// part that must not be cut is the part that goes at the top.
	if e.asking != "" {
		b.groups = append(b.groups, lookGroup{title: "WAITING ON YOU", facts: []fact{
			{label: "asking", value: e.asking},
		}})
	}

	what := lookGroup{title: "WHAT"}
	what.facts = append(what.facts,
		fact{label: "kind", value: e.kind},
		fact{label: "command", value: e.command, verbatim: true},
		fact{label: "pid", value: strconv.Itoa(e.pid)},
	)
	// How it stands, and how long it has stood that way. Only an agent
	// says the moment, so the rest read status alone; the clause is the
	// answer to "how long has this been the case", which is the first
	// thing asked of a row that is waiting on you.
	standing := e.status
	// A stopped or ended process gets no clause: the moment conn has is
	// the moment an agent last changed what it says of itself, which
	// has nothing to do with when it was stopped, and dating one from
	// the other would be a plain lie.
	if !e.since.IsZero() && !e.fault {
		standing += " · FOR " + age(e.since, now)
	}
	what.facts = append(what.facts,
		fact{label: "status", value: standing},
		fact{label: "up", value: age(e.started, now)},
	)
	b.groups = append(b.groups, what)

	where := lookGroup{title: "WHERE"}
	where.facts = append(where.facts, fact{label: "place", value: tilde(pl.path, home), path: true})
	// The place is the tree's, and a process below the root can have
	// cd'd anywhere since; where it actually is is worth saying only
	// when it is somewhere else.
	if e.cwd != "" && e.cwd != pl.path {
		where.facts = append(where.facts, fact{label: "cwd", value: tilde(e.cwd, home), path: true})
	}
	where.facts = append(where.facts, fact{label: "tty", value: e.tty})
	switch {
	case !inside:
		// conn holds no panes outside its server, so saying this row is
		// in none of them says nothing about the row.
	case pane.id != "":
		where.facts = append(where.facts, fact{label: "pane", value: pane.id + " · CAN BE REACHED"})
	default:
		where.facts = append(where.facts, fact{label: "pane", value: "NONE · CONN DID NOT OPEN IT"})
	}
	b.groups = append(b.groups, where)

	return b
}

// drawLook renders the look for a terminal of the given size.
func drawLook(b lookReport, width, height int, p palette) []row {
	width = max(width, railMinCols)
	measure, _, _ := columns(width)
	c := canvas{p: p, width: width}

	// The header: the view's name, and against the right the pid, which
	// is what the page is about and the one thing about a row that
	// cannot be mistaken for another row.
	c.blank(0)
	l := c.line()
	l.add(p.orange+p.bold, "LOOK")
	right := "PID " + strconv.Itoa(b.pid)
	l.to(measure - utf8.RuneCountInString(right))
	l.add(p.gray, right)
	c.emit(l, 0, false)
	c.rule(0, measure)

	if b.gone {
		c.blank(0)
		l := c.line()
		l.add(p.chip, " THE ROW IS NO LONGER ON WATCH ")
		c.emit(l, 0, false)
		return padTo(c, height)
	}

	// The groups: a title, then a fact a line, the value wrapped rather
	// than cut — the look is where what does not fit elsewhere is said,
	// so cutting it here would leave it said nowhere.
	for _, g := range b.groups {
		if len(g.facts) == 0 {
			continue
		}
		c.blank(0)
		l := c.line()
		l.title(0, g.title)
		c.emit(l, 0, false)
		for _, f := range g.facts {
			for i, part := range wrapValue(cased(f.value, f.path || f.verbatim), measure-factCol-1) {
				l := c.line()
				if i == 0 {
					l.leader(strings.ToUpper(f.label), factCol-1, p.faint)
				} else {
					l.to(factCol)
				}
				l.add(p.ink, part)
				c.emit(l, 0, false)
			}
		}
	}
	return padTo(c, height)
}

// padTo fills the page out to the terminal's height, so a short reading
// does not leave the rail's old rows showing under it.
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
// path is none, and both have to arrive whole.
func wrapValue(s string, width int) []string {
	if width < 1 {
		width = 1
	}
	var out []string
	for utf8.RuneCountInString(s) > width {
		cut := -1
		n := 0
		for i, r := range s {
			if n >= width {
				break
			}
			if r == ' ' {
				cut = i
			}
			n++
		}
		if cut <= 0 {
			cut = len(string([]rune(s)[:width]))
			out = append(out, s[:cut])
			s = s[cut:]
			continue
		}
		out = append(out, s[:cut])
		s = strings.TrimLeft(s[cut:], " ")
	}
	return append(out, s)
}
