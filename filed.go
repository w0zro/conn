package main

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// The panel, drawn: each project under its eyebrow with what it wants
// at the end of the rule, a row of air before each. A row is a mark for
// what it is and a word for what it is doing, and at the right its own
// word where it has one to say. The rows of a project stand in the
// panel's order, by kind: see byKind. A working row turns a spinner in
// the margin, in the column between the cursor's bar and the mark, as
// the readings come, so what is at work is seen to be. A serving row's
// mark is in the running color and its port follows its command, being
// where you would go. What is not running is struck through.

// What a row says at its right, and only where there is something to
// say: how long a wait has waited, a fault's word, or the word for a
// row that is not running. Everything else says nothing there — a row
// at work turns a spinner, a row that serves says its port, a row at
// rest has nothing to report — and a word on every row is a column of
// words that are read past to find the one that is not.
//
// The wait is stamped and blinks on the console's cadence, since the
// row that wants you should be seen before it is read and the row that
// moves is the one the corner of an eye finds; a fault wears the same
// stamp and holds still, which is the difference between a thing to
// look at and a thing to answer. A wait says its age rather than its
// word: that both rows are waiting is said by the two stamps, and which
// of them to answer first is said by nothing else.
func rowWord(r processRow) (word string, stamped, blinks bool) {
	switch {
	case r.status == statusWaiting:
		word = strings.ToUpper(r.age)
		if word == "" {
			word = statusWaiting
		}
		return word, true, true
	case r.fault:
		return r.status, true, false
	case over(r.status):
		return r.status, false, false
	}
	return "", false, false
}

// verdict is what a block says of itself at the end of its rule: the
// worst of what stands under it, counted where there is more than one
// of it, and nothing where the project wants nothing. Several faults
// are counted rather than named, since two words cannot both be the
// one word there is room for and the rows say which is which.
func verdict(rows []processRow) (word string, stamped, blinks bool) {
	waiting, faults, down, fault := 0, 0, 0, ""
	for _, r := range rows {
		switch stateOf(r.status, r.fault) {
		case standWaiting:
			waiting++
		case standFault:
			faults++
			if fault == "" {
				fault = r.status
			}
		case standDown:
			down++
		}
	}
	switch {
	case waiting > 0:
		return counted(waiting, statusWaiting), true, true
	case faults == 1:
		return fault, true, false
	case faults > 1:
		return strconv.Itoa(faults) + " FAULTS", true, false
	case down > 0:
		return counted(down, statusDown), false, false
	}
	return "", false, false
}

// counted is a word for n of a thing: the word alone for one, and the
// figure before it for more.
func counted(n int, word string) string {
	if n == 1 {
		return word
	}
	return strconv.Itoa(n) + " " + word
}

// What a row keeps of its command when the width is short, before the
// word at its right yields the row.
const commandLeast = 10

// The spinner's frames: the cell full but for one dot, the gap going
// round, a full turn in eight, and a turn a second (spinEvery, in
// tui.go). A single dot going round was a trace too faint to be seen
// turning beside a row of text; the full cell has the weight of the
// dot beside it, and the gap is what moves. It turns in the margin,
// where the rows at work make a column of their own and the text they
// are about keeps its line.
var spinner = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

// drawFiled renders the panel for a terminal of the given size, with
// the cursor on the row of the given pid. The blocks are the projects,
// folded.
func drawFiled(b processesReport, cursor int, width, height int, p palette) []row {
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
		// Every block is at the margin, with a row of air before it.
		// The projects are a list and not a tree here; see flat.
		d.blank(0)
		l := d.line()
		wants, wantStamped, wantBlinks := verdict(bp.rows)
		wantW := utf8.RuneCountInString(wants)
		if wantStamped {
			wantW = stampWidth(wants, p)
		}
		l.eyebrowTail(p.parchment+p.bold, 0, fit(bp.path, max(measure-wantW-1, 1), true), measure, wantW)
		switch {
		case wantBlinks && !b.lit:
		case wantStamped:
			l.stamp(wants)
		case wants != "":
			l.add(p.ink+p.bold, wants)
		}
		d.emit(l, 0, false)
		for _, r := range bp.rows {
			l := d.line()
			l.pid = r.pid
			cursored := r.pid == cursor
			stand := stateOf(r.status, r.fault)
			// The mark is the row's kind and never its state; the color
			// on it is how the kind stands. See the marks in pieces.go.
			tone := p.faint
			switch {
			case stand == standWaiting:
				tone = p.orange + p.bold
			case stand == standFault:
				tone = p.orange
			case stand == standOver, stand == standDown:
				// The faint it has already. A row that is not running is
				// said by its command struck through and by its word at
				// the right, and it keeps the mark of what it is, so that
				// a service that is down still reads as a service. It is
				// named here rather than left to fall through, so that a
				// declared row holding a port it no longer answers on
				// cannot be taken for one at work.
			case stand == standWorking, r.kind != kindContact && len(r.ports) > 0:
				// A contact stands by what it asks of you and never by
				// what it has open, as serving has it; anything else
				// alive on a port is at its work.
				tone = p.running
			}
			command, ports, word := p.ink, p.gray, p.gray
			if stand == standWaiting {
				command += p.bold
			}
			if over(r.status) {
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
				command, ports, word = dim, dim, dim
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
			if r.status == statusWorking {
				l.turn = spinner[b.spin%len(spinner)]
			}
			// The marks stand in one column down the block and the
			// commands start in one column beside it: the two are what
			// the panel is read down, and a column that steps in and out
			// is not one. Nothing is indented here — the rows are a list
			// in the panel's own order, by kind; see byKind.
			l.dot(tone, markOf(r.stands))
			// The right of a row is one column, and two things want it:
			// the word a row stands by, and the ports it serves on. The
			// word takes it wherever there is one — a row that is
			// waiting, or at fault, or over is telling you the thing to
			// know about it, and where it is going is not that — and
			// otherwise the ports stand there. So every row says one
			// thing at the edge, and the ports of every row that has
			// nothing else to say line up down it.
			say, stamped, blinks := rowWord(r)
			tail, tailColor := say, word
			if say == "" {
				tail, tailColor = portsColumn(r.ports), ports
			}
			tailW := utf8.RuneCountInString(tail)
			if stamped {
				tailW = stampWidth(tail, p)
			}
			activity := r.command
			if r.name != "" {
				activity = r.name
			}
			l.add(command, fit(activity, max(measure-l.cells-tailW-1, 0), false))
			switch {
			case blinks && !b.lit:
				// The column is the word's for as long as the word is
				// the row's, dark half or lit: a port coming up in the
				// gap would be the row saying something else every
				// second, and a blink is one thing appearing and not
				// two things taking turns.
			case stamped:
				l.to(measure - tailW)
				l.stamp(tail)
			case tail != "":
				l.to(measure - tailW)
				l.add(tailColor, tail)
			}
			d.emit(l, 0, false)
		}
		// What is wrong with the project's .conn, under its rows, as a
		// fault is stamped: the file was written to be read, and a
		// project that shows none of what it declares should say why.
		if bp.note != "" {
			l := d.line()
			l.to(3)
			l.add(p.chip, " "+fit(strings.ToUpper(bp.note), max(measure-5, 1), false)+" ")
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
