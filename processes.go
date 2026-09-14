package main

import (
	"strings"
	"time"
	"unicode/utf8"
)

// The processes view: what is running, by project. Under a short
// header, each project work is happening in is a block — its path as a
// title, and a row for each process that stands for its own work there,
// nested under whatever runs it the way the processes actually are: its
// kind, what it is doing, its terminal, how long it has stood as it
// does, and the word for how it stands. A row under another indents,
// its kind and command shifted in together, the rest of its columns
// staying where they are. Everything sits where it started and stays
// there for as long as it lives, oldest first, so what is new goes on
// the end and nothing above it moves. A cursor is on one row, which is
// drawn on a raised ground from edge to edge, and the rows scroll to
// keep it in view. What conn holds — a process in a pane of the server,
// which can be reached — is written in the ink; work conn can only
// report is dimmed a rank. In the panel, which is narrower than the
// console, the terminal column is left off and the rest close up; the
// row on the right, in the bay, is in orange.

// The processes view's words, composed from the projects as of a
// moment.
type processesReport struct {
	projects []projectBlock
	err      string // why the table could not be read, when it could not
	inside   bool   // conn is in its server, and rows can be reached
	lit      bool   // the annunciators' lit half; see the waiting word below
}

type projectBlock struct {
	path string
	rows []processRow
}

type processRow struct {
	pid                               int
	kind, command, tty, since, status string
	fault                             bool
	reach                             string // the pane that holds it, in conn's server
	shown                             bool   // it is in the bay, on the right
	depth                             int    // how deep under its project's own root
}

// headOf is the first row of a terminal in the projects as read: the
// process its pane was opened on, which everything else in that pane
// hangs under. It is what the bay's mark goes on and what the cursor
// belongs on once the pane is reached, and both ask here so that the
// two can never disagree about which row the pane is. It answers the
// row's place in the reading too, for the cursor to hold.
func headOf(projects []project, tty string) (pid, at int, ok bool) {
	if tty == "" {
		return 0, 0, false
	}
	i := 0
	for _, pl := range projects {
		for _, e := range pl.entries {
			if e.tty == tty {
				return e.pid, i, true
			}
			i++
		}
	}
	return 0, 0, false
}

// composeProcesses words the projects; panes says which terminals are
// the server's, and bay which of them is on the right.
//
// A pane holds a whole tree, and all of it is equally in the bay, but
// saying so on every row of it paints a block rather than a mark. Only
// the head of that tree is marked shown. What hangs under it reads as
// what it is: in a pane conn holds, like any other row conn can reach.
func composeProcesses(projects []project, panes map[string]pane, bay string, roots []string, home string, now time.Time, err string) processesReport {
	b := processesReport{err: err}
	head, _, marked := headOf(projects, bay)
	for _, pl := range projects {
		bp := projectBlock{path: projectName(pl.path, roots, home)}
		if bp.path == "" {
			bp.path = "NO PROJECT"
		}
		for _, e := range pl.entries {
			bp.rows = append(bp.rows, processRow{
				pid: e.pid, kind: e.kind, command: activityOf(e), tty: e.tty, since: sinceWord(e.since, now),
				status: e.status, fault: e.fault, reach: panes[e.tty].id,
				shown: marked && e.pid == head, depth: e.depth,
			})
		}
		b.projects = append(b.projects, bp)
	}
	return b
}

// activityOf is what a row's middle column says: for a working contact
// the tool it has in flight, and for anything else its command as
// typed, whose arguments are what it is doing. A contact with nothing
// in flight says its command, which reads as the intelligence
// composing.
func activityOf(e entry) string {
	if e.doing != "" {
		return e.doing
	}
	return e.asTyped()
}

// projectName is what the processes view writes over a block: what is
// left of the path once the root the checkouts are kept under is taken
// off it. ~/projects/w0zro/conn is w0zro/conn. The root is the same for
// every project shown and says nothing that tells one from another, and
// it is said at the head of every block — the panel is forty-four
// columns wide, and the part that tells them apart is the part that
// should have them.
//
// A project outside every root is written from ~ and whole: there is
// nothing shared to take off it, and where it is is the only thing the
// line has to say. A root itself is written the same way, since what is
// left of it after itself is nothing.
func projectName(path string, roots []string, home string) string {
	for _, root := range roots {
		if path != root && within(path, root) {
			return relName(root, path)
		}
	}
	return tilde(path, home)
}

// The processes view's columns, from the right: the status flush with
// the measure, the time in that status and the terminal before it, and
// the command taking what is left after the kind. Under minCols the
// view is a panel: the terminal column goes, and the kind closes up.
const (
	kindW        = 8
	ttyW         = 10
	sinceW       = 5
	panelKindW   = 8
	panelMinCols = 40
	treeIndent   = 2 // columns a row gives up per level under its root
)

// drawProcesses renders the processes view for a terminal of the given
// size, with the cursor on the row of the given pid.
func drawProcesses(b processesReport, cursor int, width, height int, p palette) []row {
	panel := width < minCols
	width = max(width, panelMinCols)
	measure, _, _ := columns(width)
	c := canvas{p: p, width: width}
	statusCol := measure - statusW
	sinceCol := statusCol - 1 - sinceW
	ttyCol := sinceCol - 1 - ttyW
	commandW := ttyCol - 1 - kindW
	kindCol := kindW
	if panel {
		ttyCol = -1
		kindCol = panelKindW
		commandW = sinceCol - 1 - kindCol
	}

	// The header: a rule and the column heads. The view goes unlabeled: it
	// is what conn is when it is up. The name stood over this row and the
	// status line carries it now, at the bottom left of the window where a
	// name belongs — it names the whole program rather than this one view,
	// which was carrying it for all of them.
	c.blank(0)
	c.rule(0, measure)
	l := c.line()
	l.add(p.gray, "KIND")
	l.to(kindCol)
	l.add(p.gray, "ACTIVITY")
	if !panel {
		l.to(ttyCol)
		l.add(p.gray, "TTY")
	}
	l.to(sinceCol)
	l.add(p.gray, "SINCE")
	l.to(statusCol + statusW - len("STATUS"))
	l.add(p.gray, "STATUS")
	c.emit(l, 0, false)

	// The projects, in the order work began in them; or the reason there
	// are none.
	room := height
	if height == 0 {
		room = 1 << 30
	}
	var body []row
	cursorRow := -1
	project := func(bp projectBlock) {
		d := canvas{p: p, width: width}
		d.blank(0)
		// The project's title alone. It carried a count of its rows on the
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
			// The bay is a mark, and one cell of one row is all a mark
			// needs: the kind of the head of what is in the bay, in the
			// orange, which is "you, here" everywhere else in conn. A
			// row is a lot of orange, and the status column especially
			// is not the orange's to take — WAITING is already a color
			// close to it, and the two together say neither.
			kind, command, ttyColor, sinceColor, word := p.gray, p.ink, p.gray, p.gray, p.gray
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
				kind, command, ttyColor, sinceColor, word = dim, dim, dim, dim, dim
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
			if !panel {
				l.to(ttyCol)
				l.add(ttyColor, fit(strings.ToUpper(r.tty), ttyW, false))
			}
			l.to(sinceCol)
			l.add(sinceColor, r.since)
			switch {
			case r.fault:
				l.to(measure - utf8.RuneCountInString(r.status) - 2)
				l.add(p.chip, " "+r.status+" ")
			case r.status == statusWaiting:
				// The one word here that asks something of you, and the
				// only one worth finding without looking. It is stamped
				// the way the console stamps a fault and the status line
				// stamps the keys: a block of the orange with the word
				// knocked out of it. A block is not read but seen, and
				// the thing that wants you should be seen before it is
				// read.
				//
				// And it blinks, on the console's own cadence and off
				// the same turn, which is the one thing on a screen that
				// reaches the corner of an eye. Reading down a list of
				// rows that all say something, the row that wants you is
				// the row that moves. A fault beside it wears the same
				// stamp and holds still, which is the difference between
				// a thing to look at and a thing to answer.
				//
				// On the dark half the cells are the ground and nothing
				// around them moves, the way the console's verdict goes
				// dark: a word that jumped its neighbours about would be
				// worse than one that never blinked.
				if b.lit {
					l.to(measure - utf8.RuneCountInString(r.status) - 2)
					l.add(p.chip, " "+r.status+" ")
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
	case len(b.projects) == 0:
		d := canvas{p: p, width: width}
		d.blank(0)
		l := d.line()
		l.add(p.gray, "NO PROCESSES")
		d.emit(l, 0, true)
		body = d.rows
	default:
		for _, bp := range b.projects {
			project(bp)
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
