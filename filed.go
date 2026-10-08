package main

import (
	"strconv"
	"strings"

	"github.com/w0zro/conn/internal/draw"
	"github.com/w0zro/conn/internal/work"

	"github.com/charmbracelet/x/ansi"
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
// row that is not running. Everything else says no word there — a row
// at work turns a spinner, a row that serves says its port, a contact
// says the context it carries, a row at rest has nothing to report —
// and a word on every row is a column of words that are read past to
// find the one that is not.
//
// The wait is stamped and blinks on the console's cadence, since the
// row that wants you should be seen before it is read and the row that
// moves is the one the corner of an eye finds; a fault wears the same
// stamp and holds still, which is the difference between a thing to
// look at and a thing to answer. A wait says its age rather than its
// word: that both rows are waiting is said by the two stamps, and which
// of them to answer first is said by nothing else.
//
// A contact carrying past claude.HeavyContext is stamped with the figure
// and blinks: it asks something of you too, a session to end or
// compact, and a figure that held still all afternoon would stop being
// seen. A wait or a fault outranks it, and the figure is back when
// either is over.
func rowWord(r processRow) (word string, stamped, blinks bool) {
	switch {
	case r.status == work.StatusWaiting:
		word = strings.ToUpper(r.age)
		if word == "" {
			word = work.StatusWaiting
		}
		return word, true, true
	case r.fault:
		return r.status, true, false
	case r.heavy != "":
		return r.heavy, true, true
	case work.Over(r.status):
		return r.status, false, false
	}
	return "", false, false
}

// verdict is what a block says of itself at the end of its rule: the
// worst of what stands under it, counted where there is more than one
// of it, and nothing where the project wants nothing. Several faults
// are counted rather than named, since two words cannot both be the
// one word there is room for and the rows say which is which.
//
// What wants you is a wait or a fault. A declared process that is down
// was once said here too, and it wants nothing: it is at rest, often on
// purpose — a check run when it is wanted, a server not wanted today —
// and the row says DOWN where it stands. Said again on the rule, one
// such row read as the project being down, which a project on the panel
// never is: one with nothing up is not listed.
func verdict(rows []processRow) (word string, stamped, blinks bool) {
	waiting, faults, fault := 0, 0, ""
	for _, r := range rows {
		switch stateOf(r.status, r.fault) {
		case standWaiting:
			waiting++
		case standFault:
			faults++
			if fault == "" {
				fault = r.status
			}
		}
	}
	switch {
	case waiting > 0:
		return counted(waiting, work.StatusWaiting), true, true
	case faults == 1:
		return fault, true, false
	case faults > 1:
		return strconv.Itoa(faults) + " FAULTS", true, false
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

// drawFiled renders the panel for a terminal of the given size, with
// the cursor on the row of the given pid. The blocks are the projects,
// folded.
func drawFiled(b processesReport, cursor int, width, height int, p draw.Palette) []draw.Row {
	width = max(width, draw.PanelMinCols)
	measure := draw.MeasureAt(width)
	c := draw.Canvas{P: p, Width: width}
	room := height
	if height == 0 {
		room = 1 << 30
	}

	var body []draw.Row
	cursorRow := -1
	numbers := false
	for _, bp := range b.projects {
		for _, r := range bp.rows {
			numbers = numbers || r.num != ""
		}
	}
	d := draw.Canvas{P: p, Width: width}
	for _, bp := range b.projects {
		// Every block is at the margin, with a row of air before it.
		// The projects are a list and not a tree here; see flat.
		d.Blank(0)
		l := d.Line()
		wants, wantStamped, wantBlinks := verdict(bp.rows)
		wantW := ansi.StringWidth(wants)
		if wantStamped {
			wantW = draw.StampWidth(wants, p)
		}
		l.EyebrowTail(p.Parchment+p.Bold, 0, draw.Fit(bp.path, max(measure-wantW-1, 1), true), measure, wantW)
		switch {
		case wantBlinks && !b.lit:
		case wantStamped:
			l.Stamp(wants)
		case wants != "":
			l.Add(p.Ink+p.Bold, wants)
		}
		d.Emit(l, 0, false)
		for _, r := range bp.rows {
			l := d.Line()
			l.PID = r.pid
			cursored := r.pid == cursor
			stand := stateOf(r.status, r.fault)
			// The mark is the row's kind and never its state; the color
			// on it is how the kind stands. See the marks in internal/draw.
			tone := p.Faint
			switch {
			case stand == standWaiting:
				tone = p.Orange + p.Bold
			case stand == standFault:
				tone = p.Orange
			case stand == standOver, stand == standDown:
				// The faint it has already. A row that is not running is
				// said by its command struck through and by its word at
				// the right, and it keeps the mark of what it is, so that
				// a service that is down still reads as a service. It is
				// named here rather than left to fall through, so that a
				// declared row holding a port it no longer answers on
				// cannot be taken for one at work.
			case stand == standWorking, r.kind != work.KindContact && len(r.ports) > 0:
				// A contact stands by what it asks of you and never by
				// what it has open, as serving has it; anything else
				// alive on a port is at its work.
				tone = p.Running
			}
			command, ports, word := p.Ink, p.Gray, p.Gray
			if stand == standWaiting {
				command += p.Bold
			}
			if work.Over(r.status) {
				command = p.Faint + p.Struck
			}
			if b.inside && (r.reach == "" || r.over) && !r.shown {
				// A row conn can only report, or a declared process
				// that has ended and holds its pane for its output: a
				// rank down, and every column of it.
				dim := p.Faint
				if cursored {
					dim = p.Gray
				}
				command, ports, word = dim, dim, dim
			}
			if r.shown {
				l.Mark = draw.CursorBar
			}
			if cursored {
				// The row under the cursor is on the raised ground with
				// the bar in the margin; in plain text, the mark alone.
				l.P = p.Chosen()
				l.Mark = draw.CursorBar
				if p.Plain {
					l.Mark = "▸"
				}
				command += p.Bold
				cursorRow = len(body) + len(d.Rows)
			}
			if r.status == work.StatusWorking {
				l.Turn = draw.Spinner[b.spin%len(draw.Spinner)]
			}
			// The marks stand in one column down the block and the
			// commands start in one column beside it: the two are what
			// the panel is read down, and a column that steps in and out
			// is not one. Nothing is indented here — the rows are a list
			// in the panel's own order, by kind; see byKind.
			// The digit that goes to a contact stands between the mark
			// and the command, a step under the mark: it is a key to
			// press, and the mark is what the row is. Where any row has
			// one, every row makes the room, whether or not the digits
			// are drawn, so the commands keep their one column and do
			// not step as the keys come and go.
			if numbers {
				num := " "
				if b.digits && r.num != "" {
					num = r.num
				}
				l.Add(tone, draw.MarkOf(r.stands))
				l.Add("", " ")
				l.Add(p.Faint+p.Dim, num)
				l.Add("", " ")
			} else {
				l.Dot(tone, draw.MarkOf(r.stands))
			}
			// The right of a row is one column, and two things want it:
			// the word a row stands by, and the ports it serves on. The
			// word takes it wherever there is one — a row that is
			// waiting, or at fault, or over is telling you the thing to
			// know about it, and where it is going is not that — and
			// otherwise the ports stand there, or for a contact, which
			// stands by what it asks of you and not what it has open,
			// the context it carries. So every row says one thing at the
			// edge, and the figures of every row that has nothing else
			// to say line up down it.
			say, stamped, blinks := rowWord(r)
			tail, tailColor := say, word
			switch {
			case say != "":
			case r.carried != "":
				tail, tailColor = r.carried, ports
			default:
				tail, tailColor = draw.PortsColumn(r.ports), ports
			}
			tailW := ansi.StringWidth(tail)
			if stamped {
				tailW = draw.StampWidth(tail, p)
			}
			activity := r.command
			if r.name != "" {
				activity = r.name
			}
			l.Add(command, draw.Fit(activity, max(measure-l.Cells-tailW-1, 0), false))
			switch {
			case blinks && !b.lit:
				// The column is the word's for as long as the word is
				// the row's, dark half or lit: a port coming up in the
				// gap would be the row saying something else every
				// second, and a blink is one thing appearing and not
				// two things taking turns.
			case stamped:
				l.To(measure - tailW)
				l.Stamp(tail)
			case tail != "":
				l.To(measure - tailW)
				l.Add(tailColor, tail)
			}
			d.Emit(l, 0, false)
		}
		// What is wrong with the project's .conn, under its rows, as a
		// fault is stamped: the file was written to be read, and a
		// project that shows none of what it declares should say why.
		if bp.note != "" {
			l := d.Line()
			l.To(3)
			l.Add(p.Chip, " "+draw.Fit(strings.ToUpper(bp.note), max(measure-5, 1), false)+" ")
			d.Emit(l, 0, false)
		}
	}
	body = d.Rows
	c.Rows = append(c.Rows, draw.Scrolled(body, cursorRow, room-len(c.Rows), width, p)...)
	c.Rows = append(c.Rows, notes(b, width, measure, p)...)
	if height > 0 {
		for len(c.Rows) < height {
			c.Blank(0)
		}
	}
	return c.Rows
}
