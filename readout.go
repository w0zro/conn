package main

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// The readout: what conn knows about a row, read without entering it. i
// on the processes view opens it, and it opens in the bay, beside the
// processes view rather than over it — the row it is about stays on
// screen under the cursor, and moving the cursor and pressing i again
// is how you read down a list. It is conn's own program in a pane of
// the server, `conn readout`, the way the hold is, so the bay holds it
// the way it holds anything else and a real pane reaching the bay is
// rid of it.
//
// A row of the processes view is six columns on a panel and most of
// what conn reads of a process does not fit in that; it is dropped
// rather than shortened, which is right for the processes view and
// leaves the dropped part said nowhere. This is where it is said. What
// the process is and was started as, whole. Where it actually is,
// against the project its tree belongs to. What runs it and what it
// runs. What it has spent. Of a contact, which session it is carrying
// and what it is stopped on. Of the project, what git says of it — the
// branch, whether the tree is clean, what the last commit was — since a
// row stands for work and the work is in a repository.
//
// It is a reading of facts, so it is written the way conn's other
// reading of facts is: a label, a dotted leader, a value, grouped under
// a title. The console says what the machine is in that form, and the
// readout says what one row of it is; they are the same instrument
// speaking, and there is no reason for them to speak differently.

// readoutSubject is everything the page is composed from: the row as
// the processes view has it, the table's own record behind it, what
// stands around it in the tree, and what the things conn can ask —
// claude, git — say of it. The readout's own program gathers this;
// composing is then all wording and no reading, and can be held to by a
// test.
type readoutSubject struct {
	entry    entry
	proc     process // the table's record, for what a row does not carry
	project  project
	parent   entry   // what runs it, where anything conn can see does
	children []entry // what it runs, in the order the tree has them
	pane     pane
	inside   bool
	sess     sessionFile // what a contact says of itself, when conn can ask
	carried  session
	git      gitStatus
}

// readoutReport is the readout's words as things stand, about one row.
type readoutReport struct {
	pid    int
	gone   bool // the row was there when the readout opened, and is not now
	groups []readoutGroup
}

// A readoutGroup is a title and the facts under it. A group with no
// facts is not drawn: a contact's title over a shell's row would be a
// heading over nothing, and a project that is no repository has no git
// to report.
type readoutGroup struct {
	title string
	facts []fact
}

// labelW is the widest a label may be. The leader pads to factCol-1
// and then adds a space, so a label that fills the field leaves one dot
// and starts its value a column past every other value on the page. No
// label here needs the room, and a page whose values do not line up is
// a page nobody can run an eye down.
const labelW = factCol - 3

// add puts a fact in a group, and drops one with nothing to say: most
// of what the page reports is absent on some row or other, and a label
// with an empty value after it says less than no label at all.
func (g *readoutGroup) add(label, value string) {
	if value != "" {
		g.facts = append(g.facts, fact{label: label, value: value})
	}
}

// addAsWritten is add for a value that is the world's text rather than
// conn's vocabulary — a command, a prompt, a commit's subject — which
// is kept in the case it was written in.
func (g *readoutGroup) addAsWritten(label, value string) {
	// The page folds its own lines to the pane's width. A value that
	// came with newlines of its own — a command with a here-document in
	// it, a prompt somebody wrote over three lines — would otherwise
	// start those lines at column nothing, under the leaders rather
	// than beside them.
	if value = flatten(value); value != "" {
		g.facts = append(g.facts, fact{label: label, value: value, verbatim: true})
	}
}

// addPath is add for a path, which keeps its case and is elided from
// the head when it has to be, the tail being the telling part.
func (g *readoutGroup) addPath(label, value string) {
	if value != "" {
		g.facts = append(g.facts, fact{label: label, value: value, path: true})
	}
}

// composeReadout words one row.
func composeReadout(s readoutSubject, home string, now time.Time) readoutReport {
	e := s.entry
	b := readoutReport{pid: e.pid}

	// What the contact is stopped on goes first, ahead of what the row
	// even is. It is the whole reason to open the page on a waiting row,
	// the one thing the processes view has no column wide enough for, and
	// the page is cut off at the pane's height rather than scrolled — so
	// the part that must not be cut is the part that goes at the top.
	if e.asking != "" {
		ask := readoutGroup{title: "WAITING"}
		ask.add("on", e.asking)
		ask.add("for", age(e.since, now))
		// The thing itself, in the contact's words: the tool it asked to
		// use and what for, or what it last said, which is the question
		// when a turn ended on one.
		if s.carried.Ask.Tool != "" {
			ask.addAsWritten("asks", s.carried.Ask.String())
		} else {
			ask.addAsWritten("said", s.carried.Ask.Said)
		}
		b.groups = append(b.groups, ask)
	}

	what := readoutGroup{title: "WHAT"}
	what.add("kind", e.kind)
	// The command as it was written, which is a thing somebody might
	// retype — and so without the note conn appends when it raises a
	// contact. That note is a thousand characters of conn's own prose
	// with newlines through it, and printing it here was conn showing
	// the operator the noise conn made, at length, above everything the
	// page is actually for. The processes view has always dropped it.
	what.addAsWritten("command", e.asTyped())
	// The pid is on the header, an inch above this, and is not said
	// again here.
	// How it stands, and how long it has stood that way. Only a contact
	// says the moment; a stopped or ended process gets no clause at all,
	// since the moment conn holds for it is when a contact last changed
	// what it says of itself, which has nothing to do with when
	// something stopped it.
	status := e.status
	// The clause is dropped where the group above already carries it:
	// on a dense page a thing said twice reads as two things.
	if !e.since.IsZero() && !e.fault && e.asking == "" {
		status += " · FOR " + age(e.since, now)
	}
	what.add("status", status)
	what.add("state", stateWord(s.proc.state, s.proc.foreground))
	what.add("up", join(" · ", age(e.started, now), "SINCE "+stamp(e.started)))
	// What it has actually spent, which is the measure behind WORKING and
	// is nowhere in the processes view. Under a second is none worth
	// saying.
	if s.proc.cpu >= time.Second {
		what.add("cpu", spell(s.proc.cpu)+" SPENT")
	}
	b.groups = append(b.groups, what)

	where := readoutGroup{title: "WHERE"}
	where.addPath("project", tilde(s.project.path, home))
	// The project is the tree's, and a process below the root can have
	// cd'd anywhere since; where it actually is is worth saying only
	// when it is somewhere else.
	if e.cwd != "" && e.cwd != s.project.path {
		where.addPath("cwd", tilde(e.cwd, home))
	}
	where.add("tty", e.tty)
	switch {
	case !s.inside:
		// conn holds no panes outside its server, so saying this row is
		// in none of them says nothing about the row.
	case s.pane.id != "":
		where.add("pane", s.pane.id+" · CAN BE REACHED")
	default:
		where.add("pane", "NONE · CONN DID NOT OPEN IT")
	}
	b.groups = append(b.groups, where)

	// Which session a contact is carrying, and what it was last
	// asked — the two things that say which of several claudes this one
	// is, where the command line only says that it is one.
	contact := readoutGroup{title: "CONTACT"}
	// Who the contact is with. The station sees a process and a
	// transcript; this says what is at the other end of it and whose
	// it is, which is the first thing anybody asks of a thing that
	// answers back.
	if a, ok := contacts[program(e.asTyped())]; ok {
		contact.add("with", join(" · ", a.name, a.maker))
	}
	// Which model is answering, by the name the API knows it by. A
	// session can change model part way through, so this is the one
	// that answered last and not the one it was raised on.
	contact.add("model", s.carried.Model)
	// What that turn was given to read: what was sent with it and what
	// it read back out of the cache. It is what the contact is hauling,
	// and the readable half of the question people ask about a context
	// window. How large the window is is nowhere Claude Code writes it
	// down, so conn does not say.
	if s.carried.Carried > 0 {
		contact.add("context", tokens(s.carried.Carried)+" CARRIED")
	}
	// A resumed contact names its session on its own command line, an
	// inch above, and this would be the second place to read one id.
	if !strings.Contains(e.asTyped(), s.sess.SessionID) {
		contact.add("session", s.sess.SessionID)
	}
	contact.add("name", s.sess.Name)
	contact.add("version", s.sess.Version)
	// What sort of session it is, said only when it is not somebody's
	// own. Every contact conn can put you in front of is interactive,
	// so a row saying so on every one of them is a row nobody reads;
	// one running behind another session is worth the line, and is the
	// answer to a claude in the list nobody remembers starting.
	if s.sess.Kind != interactiveSession {
		contact.add("running", s.sess.Kind)
	}
	// The branch the session recorded, where that is not the branch the
	// project is on now. The same word under two titles reads as two
	// facts; a contact working a branch the project has since left is
	// the case worth a line of its own.
	if !strings.EqualFold(s.carried.Branch, s.git.branch) {
		contact.add("branch", s.carried.Branch)
	}
	contact.addAsWritten("last ask", s.carried.Prompt)
	b.groups = append(b.groups, contact)

	// What git says of the project. A row stands for work, and the branch
	// it is on and whether the tree is clean are the first two things
	// anyone asks of work.
	if s.git.repo {
		g := readoutGroup{title: "PROJECT"}
		branch := s.git.branch
		if s.git.detached {
			branch = "DETACHED"
		}
		if s.git.dirty > 0 {
			branch = join(" · ", branch, strconv.Itoa(s.git.dirty)+" CHANGED")
		} else {
			branch = join(" · ", branch, "CLEAN")
		}
		g.add("branch", branch)
		g.addAsWritten("commit", join(" · ", s.git.commit, s.git.subject))
		g.add("committed", age(s.git.when, now)+" AGO")
		// Against what it tracks, when it tracks anything: a branch with
		// no upstream is not behind by nothing, there is nothing for it
		// to be behind.
		if s.git.upstream != "" {
			var moves []string
			if s.git.ahead > 0 {
				moves = append(moves, strconv.Itoa(s.git.ahead)+" AHEAD")
			}
			if s.git.behind > 0 {
				moves = append(moves, strconv.Itoa(s.git.behind)+" BEHIND")
			}
			if len(moves) == 0 {
				moves = append(moves, "EVEN")
			}
			g.add("tracking", s.git.upstream+" · "+strings.Join(moves, ", "))
		}
		b.groups = append(b.groups, g)
	}

	// What stands around it. The processes view draws the tree already,
	// but it draws it indented across a whole project; here it is the one
	// row's own line of descent, said plainly.
	// A relative is named by what it is and what program it is, and not
	// by its whole command line. A shell a contact runs carries the
	// environment snapshot it was started with, which is a screen of
	// somebody else's shell quoting, and four of them buried the page
	// that was meant to be about this row. The whole line of any of
	// them is on that row's own page, an i away.
	tree := readoutGroup{title: "TREE"}
	if s.parent.pid != 0 {
		tree.addAsWritten("parent", s.parent.kind+" "+program(s.parent.asTyped())+" · "+strconv.Itoa(s.parent.pid))
	}
	for i, k := range s.children {
		label := "runs"
		if i > 0 {
			label = ""
		}
		tree.facts = append(tree.facts, fact{
			label:    label,
			value:    k.kind + " " + program(k.asTyped()) + " · " + strconv.Itoa(k.pid) + " · " + k.status,
			verbatim: true,
		})
	}
	b.groups = append(b.groups, tree)

	return b
}

// stateWord is what the table's one letter for a process means, with
// whether its group holds the terminal — which is the difference
// between a thing you are talking to and a thing running behind it.
func stateWord(state byte, foreground bool) string {
	var word string
	switch state {
	case 'R':
		word = "RUNNING"
	case 'S':
		word = "SLEEPING"
	case 'I':
		word = "IDLE"
	case 'T':
		word = "STOPPED"
	case 'Z':
		word = "ENDED"
	case 'D':
		word = "IN DISK WAIT"
	default:
		return ""
	}
	if foreground {
		word += " · HAS THE TERMINAL"
	}
	return word
}

// stamp is a moment as the console writes one: the day and the time, in
// Zulu, so two readings on two machines can be set beside each other.
func stamp(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return strings.ToUpper(at.UTC().Format("02-Jan 15:04")) + " Z"
}

// drawReadout renders the readout for a pane of the given size.
func drawReadout(b readoutReport, width, height int, p palette) []row {
	width = max(width, panelMinCols)
	measure, _, _ := columns(width)
	c := canvas{p: p, width: width}

	// The header: the view's name, and against the right the pid, which
	// is what the page is about and the one thing about a row that
	// cannot be mistaken for another row.
	c.blank(0)
	l := c.line()
	l.add(p.orange+p.bold, "READOUT")
	right := "PID " + strconv.Itoa(b.pid)
	l.to(measure - utf8.RuneCountInString(right))
	l.add(p.gray, right)
	c.emit(l, 0, false)
	c.rule(0, measure)

	if b.gone {
		c.blank(0)
		l := c.line()
		l.add(p.chip, " THE ROW IS NO LONGER LISTED ")
		c.emit(l, 0, false)
		return padTo(c, height)
	}

	// The groups: a title, then a fact a line, the value wrapped rather
	// than cut — the readout is where what does not fit elsewhere is said,
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
				if i == 0 && f.label != "" {
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

// padTo fills the page out to the pane's height, so a short reading
// does not leave older rows showing under it.
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

// tokens is a count of tokens, in the largest unit that keeps it short:
// 572K rather than 571,592, which is a number nobody reads and a
// precision nobody has a use for.
func tokens(n int) string {
	switch {
	case n >= 1_000_000:
		return strconv.Itoa(n/100_000/10) + "." + strconv.Itoa(n/100_000%10) + "M"
	case n >= 1_000:
		return strconv.Itoa(n/1_000) + "K"
	}
	return strconv.Itoa(n)
}
