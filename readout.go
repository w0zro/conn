package main

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// The readout: what conn knows about a row, read without entering it.
// The panel puts it in the bay whenever the keys are on the panel and
// a row is under the cursor, beside the processes view rather than
// over it — the row it is about stays on screen under the cursor, and
// the page follows the cursor down the list; nothing is pressed for
// it. It is conn's own program in a pane of
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
	proc     record // the table's record, for what a row does not carry
	project  project
	parent   entry   // what runs it, where anything conn can see does
	children []entry // what it runs, in the order the tree has them
	pane     pane
	inside   bool
	sess     sessionFile // what a contact says of itself, when conn can ask
	carried  session
	git      gitStatus
	// What docker says of this row, where the row is a container, as
	// the panel published it; see cursor.go.
	container *container
}

// readoutReport is the readout's words as things stand, about one row.
type readoutReport struct {
	pid int
	// What the header calls the row, where a pid is not what it is known
	// by: a container's id.
	name   string
	gone   bool // the row was there when the readout opened, and is not now
	groups []readoutGroup
	// A contact's page is a sheet rather than groups; see page.go.
	contact *contactPage
}

// A readoutGroup is a title and the facts under it. A group with no
// facts is not drawn: a contact's title over a shell's row would be a
// heading over nothing, and a project that is no repository has no git
// to report.
type readoutGroup struct {
	title string
	facts []fact
}

// pageCol is where a value starts on the page, from the margin.
const pageCol = factCol

// labelW is the widest a label may be. The leader pads to pageCol-1
// and then adds a space, so a label that fills the field leaves one dot
// and starts its value a column past every other value on the page. No
// label here needs the room, and a page whose values do not line up is
// a page nobody can run an eye down.
const labelW = pageCol - 3

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
	// A row with no process — a declaration down, a service down — is
	// filed under a number of conn's own, below zero where no process
	// is, so the cursor can hold it. The header names the row instead:
	// saying the number would be conn showing the operator its own
	// filing, and a pid that is not one is worse than no pid.
	if e.pid < 0 {
		b.name = program(e.asTyped())
	}

	// What the contact is stopped on goes first, ahead of what the row
	// even is. It is the whole reason to open the page on a waiting row,
	// the one thing the processes view has no column wide enough for, and
	// the page is cut off at the pane's height rather than scrolled — so
	// the part that must not be cut is the part that goes at the top.
	if e.asking != "" {
		ask := readoutGroup{title: "WAITING"}
		ask.add("On", e.asking)
		ask.add("For", elapsed(e.since, now))
		// The thing itself, in the contact's words: the tool it asked to
		// use and what for, or what it last said, which is the question
		// when a turn ended on one.
		if s.carried.Ask.Tool != "" {
			ask.addAsWritten("Asks", s.carried.Ask.String())
		} else {
			ask.addAsWritten("Said", s.carried.Ask.Said)
		}
		b.groups = append(b.groups, ask)
	}

	// A container is not a process, and the groups below ask the process
	// table things it has no answer for — a kernel state, a start, a
	// processor time. What there is to say of it is what docker says,
	// and it is said here instead.
	if s.container != nil {
		return composeService(b, *s.container, s.pane, s.inside, home, now)
	}

	// A contact's page is the sheet, drawn instead of the groups; the
	// groups are composed all the same, being what the tests of the
	// wording read.
	if e.kind == kindContact {
		sheet := composeContact(s, home, now)
		b.contact = &sheet
	}

	what := readoutGroup{title: "WHAT"}
	what.add("Kind", said(e.kind))
	// The command as it was written, which is a thing somebody might
	// retype — and so without the note conn appends when it raises a
	// contact. That note is a thousand characters of conn's own prose
	// with newlines through it, and printing it here was conn showing
	// the operator the noise conn made, at length, above everything the
	// page is actually for. The processes view has always dropped it.
	what.addAsWritten("Command", e.asTyped())
	// The pid is on the header, an inch above this, and is not said
	// again here.
	// How it stands, and how long it has stood that way. Only a contact
	// says the moment; a stopped or ended process gets no clause at all,
	// since the moment conn holds for it is when a contact last changed
	// what it says of itself, which has nothing to do with when
	// something stopped it.
	status := said(e.status)
	// The clause is dropped where the group above already carries it:
	// on a dense page a thing said twice reads as two things.
	if !e.since.IsZero() && !e.fault && e.asking == "" {
		status += " · for " + elapsed(e.since, now)
	}
	what.add("Status", status)
	what.add("State", stateWord(s.proc.state, s.proc.foreground))
	if !e.started.IsZero() {
		what.add("Up", join(" · ", elapsed(e.started, now), "since "+stamp(e.started)))
	}
	// What it has actually spent, which is the measure behind WORKING and
	// is nowhere in the processes view. Under a second is none worth
	// saying.
	if s.proc.cpu >= time.Second {
		what.add("CPU", span(s.proc.cpu)+" spent")
	}
	b.groups = append(b.groups, what)

	where := readoutGroup{title: "WHERE"}
	where.addPath("Project", tilde(s.project.path, home))
	// The project is the tree's, and a process below the root can have
	// cd'd anywhere since; where it actually is is worth saying only
	// when it is somewhere else.
	if e.cwd != "" && e.cwd != s.project.path {
		where.addPath("CWD", tilde(e.cwd, home))
	}
	where.add("TTY", e.tty)
	// Whether conn can put you in front of this row, by the one rule
	// that decides it everywhere: enter, the ring, and the bay taking
	// the next thing when its own ends all ask the same question, and
	// the page answers it the same way.
	switch {
	case !s.inside:
		// conn holds no panes outside its server, so saying this row is
		// in none of them says nothing about the row.
	case reachable(s.pane):
		where.add("Pane", s.pane.id+" · can be reached")
	case s.pane.dead:
		where.add("Pane", s.pane.id+" · its pane has ended")
	case s.pane.id != "":
		where.add("Pane", s.pane.id+" · cannot be reached")
	default:
		where.add("Pane", "None · conn did not open it")
	}
	b.groups = append(b.groups, where)

	// What it has open to the world: what it listens on, what it is
	// connected to, and the unix sockets it holds by path. A server is
	// told from a shell at its prompt by nothing else on the page.
	b.groups = append(b.groups, socketGroup(e.sockets))

	// Which session a contact is carrying, and what it was last
	// asked — the two things that say which of several claudes this one
	// is, where the command line only says that it is one.
	contact := readoutGroup{title: "CONTACT"}
	// Who the contact is with. The station sees a process and a
	// transcript; this says what is at the other end of it and whose
	// it is, which is the first thing anybody asks of a thing that
	// answers back.
	if a, ok := contacts[program(e.asTyped())]; ok {
		contact.add("With", join(" · ", a.name, a.maker))
	}
	// Which model is answering, by the name the API knows it by. A
	// session can change model part way through, so this is the one
	// that answered last and not the one it was raised on.
	contact.add("Model", s.carried.Model)
	// What that turn was given to read: what was sent with it and what
	// it read back out of the cache. It is what the contact is hauling,
	// and the readable half of the question people ask about a context
	// window. How large the window is is nowhere Claude Code writes it
	// down, so conn does not say.
	if s.carried.Carried > 0 {
		contact.add("Context", tokens(s.carried.Carried)+" carried")
	}
	// A resumed contact names its session on its own command line, an
	// inch above, and this would be the second place to read one id.
	if !strings.Contains(e.asTyped(), s.sess.SessionID) {
		contact.add("Session", s.sess.SessionID)
	}
	contact.add("Name", s.sess.Name)
	contact.add("Version", s.sess.Version)
	// What sort of session it is, said only when it is not somebody's
	// own. Every contact conn can put you in front of is interactive,
	// so a row saying so on every one of them is a row nobody reads;
	// one running behind another session is worth the line, and is the
	// answer to a claude in the list nobody remembers starting.
	if s.sess.Kind != interactiveSession {
		contact.add("Running", s.sess.Kind)
	}
	// The branch the session recorded, where that is not the branch the
	// project is on now. The same word under two titles reads as two
	// facts; a contact working a branch the project has since left is
	// the case worth a line of its own.
	if !strings.EqualFold(s.carried.Branch, s.git.branch) {
		contact.add("Branch", s.carried.Branch)
	}
	contact.addAsWritten("Last ask", s.carried.Prompt)
	// A contact that gives no account of itself. conn reads what Claude
	// Code writes down about the instance it is running; another
	// maker's agent writes nothing conn knows how to read, and neither
	// does a claude old enough to predate the file. The group would
	// otherwise be one row saying who it is with and then nothing,
	// which reads as a page that gave up rather than a channel with
	// nothing on it.
	if e.kind == kindContact && s.sess.SessionID == "" {
		contact.add("Says", "Nothing conn can read")
	}
	b.groups = append(b.groups, contact)

	// What git says of the project. A row stands for work, and the branch
	// it is on and whether the tree is clean are the first two things
	// anyone asks of work.
	b.groups = append(b.groups, gitGroups(s.git, now)...)

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
		tree.addAsWritten("Parent", said(s.parent.kind)+" "+program(s.parent.asTyped())+" · "+strconv.Itoa(s.parent.pid))
	}
	for i, k := range s.children {
		label := "Runs"
		if i > 0 {
			label = ""
		}
		tree.facts = append(tree.facts, fact{
			label:    label,
			value:    said(k.kind) + " " + program(k.asTyped()) + " · " + strconv.Itoa(k.pid) + " · " + said(k.status),
			verbatim: true,
		})
	}
	b.groups = append(b.groups, tree)

	return b
}

// socketGroup is what a row has open to the world, each socket a
// line: the ones that listen first, then the connections, then the
// unix sockets, the label given once for each kind.
func socketGroup(sockets []socket) readoutGroup {
	g := readoutGroup{title: "SOCKETS"}
	var listens, connected, unix []string
	for _, s := range sockets {
		switch {
		case s.proto == "unix":
			unix = append(unix, s.addr)
		case s.listening():
			listens = append(listens, s.String())
		case s.proto == "TCP":
			connected = append(connected, s.String())
		}
	}
	for _, kind := range []struct {
		label string
		lines []string
	}{{"Listens", listens}, {"Connected", connected}, {"Unix", unix}} {
		for i, line := range kind.lines {
			label := kind.label
			if i > 0 {
				label = ""
			}
			g.facts = append(g.facts, fact{label: label, value: line, verbatim: true})
		}
	}
	return g
}

// gitGroups is what git says of a project, as the page words it. A
// channel with nothing on it says so. A directory that is no
// repository has nothing to report and no group appears, which is the
// page saying nothing of what there is none of; a git conn could not
// ask is a reading conn went for and did not get, and that is the
// group's to say rather than to swallow. The two looked the same from
// here until git.go learned to tell them apart.
func gitGroups(git gitStatus, now time.Time) []readoutGroup {
	var out []readoutGroup
	if git.problem != "" {
		g := readoutGroup{title: "PROJECT"}
		g.add("Git", git.problem)
		out = append(out, g)
	}
	if git.repo {
		g := readoutGroup{title: "PROJECT"}
		branch := git.branch
		if git.detached {
			branch = "detached"
		}
		if git.dirty > 0 {
			branch = join(" · ", branch, strconv.Itoa(git.dirty)+" changed")
		} else {
			branch = join(" · ", branch, "clean")
		}
		g.add("Branch", branch)
		g.addAsWritten("Commit", join(" · ", git.commit, git.subject))
		g.add("Committed", elapsed(git.when, now)+" ago")
		// Against what it tracks, when it tracks anything: a branch with
		// no upstream is not behind by nothing, there is nothing for it
		// to be behind.
		if git.upstream != "" {
			var moves []string
			if git.ahead > 0 {
				moves = append(moves, strconv.Itoa(git.ahead)+" ahead")
			}
			if git.behind > 0 {
				moves = append(moves, strconv.Itoa(git.behind)+" behind")
			}
			if len(moves) == 0 {
				moves = append(moves, "even")
			}
			g.add("Tracking", git.upstream+" · "+strings.Join(moves, ", "))
		}
		out = append(out, g)
	}
	return out
}

// composeProject words a project: the page the list's cursor gets on a
// project row, where the processes view's cursor is always on a
// process. It is where the project is, what git says of it, and the
// rows conn has running in it — the same three things the processes
// view and the page say of a row, said of the place instead.
func composeProject(path string, t readoutTable, home string, now time.Time) readoutReport {
	b := readoutReport{name: tilde(path, home)}
	where := readoutGroup{title: "WHERE"}
	where.addPath("Project", tilde(path, home))
	b.groups = append(b.groups, where)
	b.groups = append(b.groups, gitGroups(t.git[path], now)...)
	// What is running there, as the processes view lists it: each row
	// at its own depth, so a tree reads as one. A project with nothing
	// running in it has no group, which is the page saying so.
	running := readoutGroup{title: "RUNNING"}
	for _, pl := range unfiled(t.projects) {
		if pl.path != path {
			continue
		}
		for _, e := range pl.entries {
			label := ""
			if len(running.facts) == 0 {
				label = "Rows"
			}
			running.facts = append(running.facts, fact{
				label:    label,
				value:    strings.Repeat("  ", e.depth) + said(e.kind) + " " + activityOf(e) + " · " + strconv.Itoa(e.pid) + " · " + said(e.status),
				verbatim: true,
			})
		}
	}
	b.groups = append(b.groups, running)
	return b
}

// composeSession words a suspended session: the page the sessions
// list's cursor gets. It is what a reader would pick a session up by —
// when it last moved, the branch it was on, the last thing asked of it
// — what answered it and what it was carrying, where it was had and
// what git says of that, and the command that picks it back up, which
// is what enter runs and is worth knowing by name.
func composeSession(c session, t readoutTable, home string, now time.Time) readoutReport {
	b := readoutReport{name: c.ID}
	what := readoutGroup{title: "WHAT"}
	what.add("Kind", said("SESSION"))
	if a, ok := contacts[contactProgram]; ok {
		what.add("With", join(" · ", a.name, a.maker))
	}
	if !c.When.IsZero() {
		what.add("Moved", elapsed(c.When, now)+" ago · "+stamp(c.When))
	}
	what.add("Branch", c.Branch)
	what.addAsWritten("Last ask", c.Prompt)
	what.add("Model", c.Model)
	if c.Carried > 0 {
		what.add("Context", tokens(c.Carried)+" carried")
	}
	what.addAsWritten("Resume", contactProgram+" --resume "+c.ID)
	b.groups = append(b.groups, what)

	where := readoutGroup{title: "WHERE"}
	where.addPath("Project", tilde(c.Dir, home))
	b.groups = append(b.groups, where)
	b.groups = append(b.groups, gitGroups(t.git[c.Dir], now)...)
	return b
}

// elapsed is how long since a moment, as the page says it: in the
// lower case the page is written in, and nothing for a moment that is
// not known.
func elapsed(since, now time.Time) string {
	if since.IsZero() {
		return ""
	}
	return span(now.Sub(since))
}

// stateWord is what the table's one letter for a process means, with
// whether its group holds the terminal — which is the difference
// between a thing you are talking to and a thing running behind it.
func stateWord(state byte, foreground bool) string {
	var word string
	switch state {
	case 'R':
		word = "Running"
	case 'S':
		word = "Sleeping"
	case 'I':
		word = "Idle"
	case 'T':
		word = "Stopped"
	case 'Z':
		word = "Ended"
	case 'D':
		word = "In disk wait"
	default:
		return ""
	}
	if foreground {
		word += " · has the terminal"
	}
	return word
}

// stamp is a moment as the console writes one: the day and the time, in
// Zulu, so two readings on two machines can be set beside each other.
func stamp(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return at.UTC().Format("02-Jan 15:04") + " Z"
}

// drawReadout renders the readout for a pane of the given size.
func drawReadout(b readoutReport, width, height int, p palette) []row {
	width = max(width, panelMinCols)
	measure, _, _ := columns(width)
	c := canvas{p: p, width: width}

	// A contact's page is a sheet of its own, with no header over it:
	// who it is stands at the head; see page.go.
	if b.contact != nil && !b.gone {
		return drawContact(b, *b.contact, width, height, p)
	}

	// The header: the view's name, and against the right the pid, which
	// is what the page is about and the one thing about a row that
	// cannot be mistaken for another row.
	c.blank(0)
	l := c.line()
	right := "PID " + strconv.Itoa(b.pid)
	// A container is known by its id. The number conn files it under is
	// its own bookkeeping — below zero, where no process is, so the
	// cursor can hold the row between readings — and saying it here
	// would be conn showing the operator conn's own filing.
	if b.name != "" {
		right = b.name
	}
	l.eyebrow(0, "READOUT", measure, right)
	c.emit(l, 0, false)

	if b.gone {
		c.blank(0)
		l := c.line()
		l.stamp("The row is no longer listed")
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
		// The group's name, with a rule running off it to the edge of
		// the page: the same mark a group's name takes on the panel,
		// since the page and the panel are the same instrument speaking.
		l.eyebrow(0, g.title, measure, "")
		c.emit(l, 0, false)
		for _, f := range g.facts {
			for i, part := range wrapValue(f.value, measure-pageCol-1) {
				l := c.line()
				if i == 0 && f.label != "" {
					l.leader(f.label, pageCol-1, p.faint)
				} else {
					l.to(pageCol)
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

// composeService words the page for a container. It is its own composing
// rather than a few clauses bolted onto the process page, because almost
// nothing carries over: a container has no kernel state, no processor
// time, no parent that started it and no terminal of its own. What it
// has is an image, a service name its siblings are named beside, ports
// it publishes on the host, and a health check that may disagree with
// the fact that it is running.
func composeService(b readoutReport, c container, p pane, inside bool, home string, now time.Time) readoutReport {
	b.name = c.id

	what := readoutGroup{title: "WHAT"}
	what.add("Kind", said(kindService))
	what.add("Service", c.service)
	what.add("Image", c.image)
	// docker's own sentence, which says the state and its age together
	// and says them better than conn would by taking them apart: Up 3
	// minutes (healthy), Exited (3) 8 seconds ago.
	what.addAsWritten("Status", said(c.status))
	// And what the row made of it, which is the word the processes view
	// used and the reason it wears a mark or does not.
	word, fault := containerStatus(c)
	if fault {
		what.add("Wrong", said(word))
	}
	// A health check disagreeing with a container that is up is the
	// whole reason to have one, and the sentence above buries it in
	// parentheses.
	if c.health != "" {
		what.add("Health", said(c.health))
	}
	if !c.since.IsZero() {
		// Docker gives how long ago the status became true, which for a
		// running container is when it started and for a stopped one is
		// when it stopped. Neither is "up", so neither is called it.
		what.add("Since", elapsed(c.since, now)+" · "+stamp(c.since))
	}
	b.groups = append(b.groups, what)

	where := readoutGroup{title: "WHERE"}
	where.addPath("Project", tilde(c.dir, home))
	// The compose project, which is what its siblings share and what
	// docker compose down would take with it.
	if c.project != "" {
		where.add("Compose", c.project)
	}
	where.add("Name", c.name)
	// Where you would go to reach it, which is the one thing a service
	// has that a process row has no column for.
	if len(c.ports) > 0 {
		where.add("Ports", "localhost:"+strings.Join(c.ports, " · localhost:"))
	}
	// A container has no terminal. The pane is the one conn opened to
	// watch it, when it has, and that is what enter goes into.
	switch {
	case !inside:
	case reachable(p):
		where.add("Pane", p.id+" · can be reached")
	case p.dead:
		where.add("Pane", p.id+" · its pane has ended")
	default:
		where.add("Pane", "None · Enter opens its log")
	}
	b.groups = append(b.groups, where)
	return b
}
