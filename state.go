package main

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// The panel by state, drawn: each group under its eyebrow with its
// count at the end of the rule, a row of air before each. A row is a
// dot, what it is doing, and at the right the project it is in, in the
// faint. A waiting row is stamped with how long it has waited there
// instead, since that is the one figure that says which of two to
// answer first; a fault says its word, stamped. A working row's dot is
// followed by a spinner that turns as the readings come, so what is at
// work is seen to be. A serving row's dot is the running color too,
// still, and its port follows its command, being where you would go.
// What is not running is struck through.

// What a filed row keeps of its own when the width is short: the
// ports, whole, then this much of the command, before the project at
// the right yields, and the project is never cut below this.
const (
	commandLeast = 10
	projectLeast = 8
)

// portsGap is the air between the column of ports and the commands.
const portsGap = 2

// The spinner's frames: the cell full but for one dot, the gap going
// round, a full turn in eight, and a turn a second (spinEvery, in
// tui.go). A single dot going round was a trace too faint to be seen
// turning beside a row of text; the full cell has the weight of the
// dot before it, and the gap is what moves.
var spinner = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

// drawState renders the panel by state for a terminal of the given size,
// with the cursor on the row of the given pid. The blocks are groups,
// as byState files them.
func drawState(b processesReport, cursor int, width, height int, p palette) []row {
	width = max(width, panelMinCols)
	measure := measureAt(width)
	c := canvas{p: p, width: width}
	room := height
	if height == 0 {
		room = 1 << 30
	}

	var body []row
	cursorRow := -1
	d := canvas{p: p, width: width}
	for _, bp := range b.projects {
		// The ports come first, in a column of their own after the
		// dot, and the group's commands start together after it. What
		// is at :3000 is the fact a serving row is looked at for, and
		// read down the group it is a column, where after each command
		// it stood at its own distance and was the first thing a narrow
		// row lost. The column is the group's own, as wide as its
		// widest, and gone from a group where nothing serves: the eye
		// reads down one group at a time, and an idle row has nothing
		// to say there, so lining its command up with a serving row's
		// cost it the cells and read as a gap.
		portsW := 0
		for _, r := range bp.rows {
			portsW = max(portsW, utf8.RuneCountInString(portsColumn(r.ports)))
		}
		d.blank(0)
		l := d.line()
		l.eyebrow(0, groupTitle(bp.path), measure, strconv.Itoa(len(bp.rows)))
		d.emit(l, 0, false)
		for _, r := range bp.rows {
			l := d.line()
			cursored := r.pid == cursor
			glyph, tone := dotRests, p.faint
			switch {
			case r.fault:
				glyph, tone = dotWants, p.orange
			case r.status == statusWaiting:
				glyph, tone = dotWants, p.orange+p.bold
			case r.status == statusWorking, bp.path == groupServing:
				glyph, tone = dotWorks, p.running
			case bp.path == groupNotRunning:
				glyph = dotOver
			}
			command, right := p.ink, p.faint
			if r.status == statusWaiting {
				command += p.bold
			}
			if bp.path == groupNotRunning {
				command = p.faint + p.struck
			}
			if b.inside && (r.reach == "" || r.over) && !r.shown {
				// A row conn can only report, or a declared process
				// that has ended and holds its pane for its output: a
				// rank down, and every column of it.
				dim := p.faint
				if cursored {
					dim = p.gray
				}
				command, right, tone = dim, dim, dim
			}
			if r.shown {
				l.mark = cursorBar
			}
			if cursored {
				// The row under the cursor is on the raised ground with
				// the bar in the margin; in plain text, the mark alone.
				l.p = p.chosen()
				l.mark = cursorBar
				if p.plain {
					l.mark = "▸"
				}
				command += p.bold
				cursorRow = len(body) + len(d.rows)
			}
			l.dot(tone, glyph)
			if portsW > 0 {
				start := l.cells
				l.add(command, portsColumn(r.ports))
				l.to(start + portsW + portsGap)
			}
			activity := r.command
			if r.name != "" {
				activity = r.name
			}
			// What stands at the right: for a wait, how long, stamped,
			// and blinking on the console's cadence the way the wide
			// view's WAITING does, since the row that wants you should
			// be seen before it is read and the row that moves is the
			// one the corner of an eye finds; for a fault, its word,
			// stamped and holding still; else the project. The wait's
			// stamp is dark on the dark half, its cells the ground, so
			// nothing around it moves.
			tail, tailColor, tailW, project, stamped := r.from, right, 0, true, ""
			if r.from == "" {
				tail = "~"
			}
			switch {
			case r.status == statusWaiting:
				stamped, project = strings.ToUpper(r.age), false
				if stamped == "" {
					stamped = statusWaiting
				}
			case r.fault:
				stamped, project = r.status, false
			}
			spin := ""
			if r.status == statusWorking {
				spin = " " + spinner[b.spin%len(spinner)]
			}
			// The project keeps to its half of the row at most: a path
			// outside every root is written whole, and elided from the
			// left. It takes what the command leaves, since the command
			// is the row's own and the name is the same on every row of
			// the block; a command longer than the row leaves it the
			// least, enough to tell one project from another.
			tailMax := max(measure/2, 12)
			if project {
				room := measure - l.cells - 2 - utf8.RuneCountInString(spin)
				need := max(utf8.RuneCountInString(activity), commandLeast)
				if spare := room - need; spare < tailMax {
					tailMax = max(spare, projectLeast)
				}
			}
			tail = fit(tail, tailMax, true)
			tailW = utf8.RuneCountInString(tail)
			if stamped != "" {
				tailW = stampWidth(stamped, p)
			}
			l.add(command, fit(activity, measure-l.cells-tailW-2-utf8.RuneCountInString(spin), false))
			if spin != "" {
				l.add(p.running, spin)
			}
			switch {
			case stamped != "" && r.status == statusWaiting && !b.lit:
			case stamped != "":
				l.to(measure - tailW)
				l.stamp(stamped)
			case tail != "":
				l.to(measure - tailW)
				l.add(tailColor, tail)
			}
			d.emit(l, 0, false)
		}
	}
	body = d.rows
	c.rows = append(c.rows, scrolled(body, cursorRow, room-len(c.rows), width, p)...)
	c.rows = append(c.rows, notes(b, width, measure, p)...)
	if height > 0 {
		for len(c.rows) < height {
			c.blank(0)
		}
	}
	return c.rows
}
