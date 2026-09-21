package main

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// The page of a contact: a handoff sheet rather than a table of facts. Who the contact is and where it is working;
// the question it stopped on, in a card with the bar down its left;
// what to do about it, as a numbered procedure of keys; what to be
// careful of; how the work has gone, as a bar and a sentence; and
// beside all that the specifications, with leaders, for the reader who
// wants the figures. A shell, a run, a service or a project keeps the
// page of groups; it is the contact that answers back, and the contact
// whose page is read before a decision.

// A contactPage is the sheet's words.
type contactPage struct {
	badge, name, where, with string // the pill, who it is, project · branch, and the agent and what it carries
	waiting                  bool
	waited                   string // how long, as the panel says it
	asked                    string // the question, in the contact's words
	askedWith                string // the tool and the moment
	standing                 string // what it is doing, where it is not waiting
	procedure                []keyHint
	caution                  string
	worked, rested           time.Duration // how the work has gone: what it worked, and what it has stood since
	restWord                 string        // what that standing is called: waiting, or idle
	story                    string        // the same, as a sentence
	specs                    []fact        // the specifications, a blank label for a row of air
}

// composeContact words a contact's page.
func composeContact(s readoutSubject, home string, now time.Time) contactPage {
	e := s.entry
	name := s.sess.Name
	if name == "" {
		name = program(e.asTyped())
	}
	badge := name
	if i := strings.LastIndex(name, "-"); i >= 0 && i+1 < len(name) {
		badge = name[i+1:]
	}
	// The sheet is headed by what the session is about where the
	// transcript says; the designation keeps the handle.
	c := contactPage{badge: strings.ToUpper(badge), name: name}
	if e.title != "" {
		c.name = e.title
	}
	c.where = join(" · ", projectName(s.project.path, nil, home), s.git.branch)
	if c.where == "" {
		c.where = tilde(s.project.path, home)
	}
	if a, ok := contacts[program(e.asTyped())]; ok {
		c.with = join(" ", a.name, s.sess.Version)
	}
	if s.carried.Carried > 0 {
		c.with = join(" · ", c.with, strings.ToLower(tokens(s.carried.Carried))+" of context carried")
	}

	// The question, where there is one: what it asked in its own words,
	// and with what, and when.
	c.waiting = e.status == statusWaiting
	if c.waiting {
		c.waited = minutes(now.Sub(e.since))
		if e.since.IsZero() {
			c.waited = ""
		}
		c.asked = s.carried.Ask.Detail
		if c.asked == "" {
			c.asked = s.carried.Ask.Said
		}
		if c.asked == "" {
			c.asked = e.asking
		}
		if s.carried.Ask.Tool != "" {
			c.askedWith = "Asked with " + s.carried.Ask.Tool
			if !e.since.IsZero() {
				c.askedWith += " at " + e.since.Local().Format("15:04")
			}
		}
	} else {
		c.standing = said(e.status)
		if e.doing != "" {
			c.standing += " · " + e.doing
		}
		if !e.since.IsZero() && e.status == statusWorking {
			c.standing += " · for " + span(now.Sub(e.since))
		}
	}

	// What to do about it: go in; leave it for the next thing waiting;
	// end it, which conn asks about first.
	if c.waiting {
		c.procedure = []keyHint{{"enter", "Go in and answer it"}, {"tab", "Leave it, take the next thing waiting"}, {"x", "End it — conn asks first"}}
	} else {
		c.procedure = []keyHint{{"enter", "Go in"}, {"x", "End it — conn asks first"}}
	}

	// What to be careful of: a contact that has the terminal and is
	// sleeping is answered, not signalled, and x reaches the contact
	// alone, not the processes under it.
	var caution []string
	if s.proc.foreground {
		state := strings.ToLower(strings.TrimSpace(strings.SplitN(stateWord(s.proc.state, false), " ·", 2)[0]))
		if state != "" {
			caution = append(caution, "It has the terminal and is "+state+".")
		} else {
			caution = append(caution, "It has the terminal.")
		}
	}
	if n := len(s.children); n > 0 {
		what := "the " + strconv.Itoa(n) + " processes under it keep going."
		if n == 1 {
			what = "the process under it keeps going."
		}
		caution = append(caution, "x signals the contact, not what it runs — "+what)
	}
	c.caution = strings.Join(caution, " ")

	// How the work has gone: what it worked, and what it has stood since;
	// then the last thing it was given, which is the work it is on. A
	// contact at work is working this moment, so its span reaches to
	// now. One at rest - stopped on an ask, or its turn simply over -
	// worked up to the moment it stopped and not a second past it, so
	// that span holds still and only the rest beside it grows. Of a
	// contact conn cannot read, conn knows neither, and says how long it
	// has been up instead. The moment it came up is known and the moment
	// it was last asked is not, so the two are not put in one clause as
	// though the one dated the other.
	if !e.started.IsZero() {
		stopped := e.since
		if stopped.Before(e.started) {
			stopped = time.Time{}
		}
		atRest := (c.waiting || e.status == statusIdle) && !stopped.IsZero()
		if atRest {
			c.worked = stopped.Sub(e.started)
			c.rested = max(now.Sub(stopped), 0)
			c.restWord = "waiting"
			if !c.waiting {
				c.restWord = "idle"
			}
		} else if e.status == statusWorking {
			c.worked = now.Sub(e.started)
		}
		story := "It came up at " + e.started.Local().Format("15:04")
		switch {
		case atRest && c.waiting:
			story += " and worked for " + span(c.worked) + ", then stopped to ask."
		case atRest:
			story += " and worked for " + span(c.worked) + ", then stopped."
		case e.status == statusWorking:
			story += " and has been at work for " + span(c.worked) + "."
		default:
			story += " and has been up " + span(now.Sub(e.started)) + "."
		}
		if s.carried.Prompt != "" {
			story += " You last gave it “" + s.carried.Prompt + "”."
		}
		c.story = story
	}

	// The specifications: the figures, with leaders, a row of air
	// between one kind of thing and the next.
	add := func(label, value string) {
		if value != "" || label == "" {
			c.specs = append(c.specs, fact{label: label, value: value})
		}
	}
	add("Designation", strings.ToUpper(name))
	if !e.started.IsZero() {
		add("Given", given(e.started, now))
	}
	if c.worked > 0 {
		add("Worked", span(c.worked))
	}
	if s.proc.cpu >= time.Second {
		add("Processor", span(s.proc.cpu))
	}
	if c.waiting {
		add("Waiting", c.waited)
	}
	add("", "")
	if s.git.repo {
		branch := s.git.branch
		if s.git.detached {
			branch = "detached"
		}
		if s.git.dirty > 0 {
			branch = join(" · ", branch, strconv.Itoa(s.git.dirty)+" changed")
		} else {
			branch = join(" · ", branch, "clean")
		}
		add("Branch", branch)
		add("Commit", s.git.commit)
		if s.git.upstream != "" {
			var moves []string
			if s.git.ahead > 0 {
				moves = append(moves, strconv.Itoa(s.git.ahead)+" ahead")
			}
			if s.git.behind > 0 {
				moves = append(moves, strconv.Itoa(s.git.behind)+" behind")
			}
			if len(moves) == 0 {
				moves = append(moves, "even")
			}
			add("Tracking", s.git.upstream+" · "+strings.Join(moves, ", "))
		}
		add("", "")
	} else if s.git.problem != "" {
		add("Git", s.git.problem)
		add("", "")
	}
	add("Folder", tilde(s.project.path, home))
	if e.cwd != "" && e.cwd != s.project.path {
		add("Working in", tilde(e.cwd, home))
	}
	switch {
	case !s.inside:
	case reachable(s.pane):
		add("Pane", s.pane.id)
	case s.pane.dead:
		add("Pane", s.pane.id+" · ended")
	case s.pane.id != "":
		add("Pane", s.pane.id+" · cannot be reached")
	default:
		add("Pane", "None · conn did not open it")
	}
	add("Process", strconv.Itoa(e.pid))
	add("Terminal", e.tty)
	// What stands around it: what runs it, and what it runs, each by
	// what it is and its program, the whole line being on its own page.
	if s.parent.pid != 0 {
		add("Under", said(s.parent.kind)+" "+program(s.parent.asTyped())+" · "+strconv.Itoa(s.parent.pid))
	}
	for i, k := range s.children {
		label := "Runs"
		if i > 0 {
			label = " "
		}
		add(label, said(k.kind)+" "+program(k.asTyped())+" · "+strconv.Itoa(k.pid))
	}
	add("", "")
	add("Command", e.asTyped())
	add("Model", s.carried.Model)
	if !strings.Contains(e.asTyped(), s.sess.SessionID) {
		add("Session", s.sess.SessionID)
	}
	if s.sess.Kind != "" && s.sess.Kind != interactiveSession {
		add("Running", s.sess.Kind)
	}
	// The branch the session recorded, where the project has since
	// left it.
	if s.carried.Branch != "" && !strings.EqualFold(s.carried.Branch, s.git.branch) {
		add("Its branch", s.carried.Branch)
	}
	if s.sess.SessionID == "" {
		add("Says", "Nothing conn can read")
	}
	for len(c.specs) > 0 && c.specs[len(c.specs)-1].label == "" {
		c.specs = c.specs[:len(c.specs)-1]
	}
	return c
}

// span is a duration as the sheet says it: 1h 25m, 2m 14s, in the
// lower case the sheet is written in. spell is the console's, in
// capitals, and the sheet is not the console.
func span(d time.Duration) string {
	return strings.ToLower(spell(d))
}

// given is when a contact was given its work, as a person says it:
// the time alone today, the day and the time otherwise.
func given(at, now time.Time) string {
	at, today := at.Local(), now.Local()
	if at.Year() == today.Year() && at.YearDay() == today.YearDay() {
		return "Today, " + at.Format("15:04")
	}
	return at.Format("2 Jan, 15:04")
}

// specW is the specifications' column, where the page is wide enough
// for two; under twoColumns the specifications go under the sheet.
// specCol is where a specification's value starts: the labels run to
// Designation, wider than the console's, and every value lines up.
const (
	specW      = 40
	specCol    = 15
	twoColumns = 110
)

// drawContact draws the sheet at a width: two columns where there is
// room for them, the sheet on the left and the specifications on the
// right; one column otherwise, the specifications under the sheet.
func drawContact(b readoutReport, c contactPage, width, height int, p palette) []row {
	width = max(width, panelMinCols)
	measure, _, _ := columns(width)
	if width < twoColumns {
		cv := canvas{p: p, width: width}
		cv.rows = append(cv.rows, drawSheet(c, measure, width, p)...)
		cv.rows = append(cv.rows, drawSpecs(c, measure, width, p)...)
		return padTo(cv, height)
	}
	// Side by side: each column is a canvas of its own width, framed
	// with its own margins, and a row of the page is a row of each
	// joined. A plain row is trimmed as it is framed, so the left is
	// padded back out to its width before the right is put after it.
	rightW := specW + 2*margin
	leftW := width - rightW
	left := drawSheet(c, leftW-2*margin, leftW, p)
	right := drawSpecs(c, specW, rightW, p)
	cv := canvas{p: p, width: width}
	for i := 0; i < max(len(left), len(right)); i++ {
		l, r := blankRow(p, leftW), ""
		if i < len(left) {
			l = left[i].text
		}
		if i < len(right) {
			r = right[i].text
		}
		if p.plain {
			l += strings.Repeat(" ", max(leftW-utf8.RuneCountInString(l), 0))
			cv.rows = append(cv.rows, row{text: strings.TrimRight(l+r, " ")})
			continue
		}
		if r == "" {
			r = blankRow(p, rightW)
		}
		cv.rows = append(cv.rows, row{text: l + r})
	}
	return padTo(cv, height)
}

// blankRow is a row of nothing at a width, in a palette, as emit frames
// one.
func blankRow(p palette, width int) string {
	cv := canvas{p: p, width: width}
	cv.blank(0)
	return cv.rows[0].text
}

// drawSheet is the left of the page: who, the card, the procedure, the
// caution and the story, at a width of its own.
func drawSheet(c contactPage, measure, width int, p palette) []row {
	cv := canvas{p: p, width: width}
	blank := func() { cv.blank(0) }
	newLine := func() *line { return cv.line() }
	emit := func(l *line) { cv.emit(l, 0, false) }

	blank()
	l := newLine()
	l.eyebrow(0, "CONTACT", measure, "")
	emit(l)
	blank()
	l = newLine()
	l.add(p.chip, " "+c.badge+" ")
	l.add("", "  ")
	l.add(p.ink+p.bold, c.name)
	if c.where != "" {
		l.add(p.gray, "   "+fit(c.where, measure-l.cells-3, true))
	}
	emit(l)
	if c.with != "" {
		l = newLine()
		l.to(utf8.RuneCountInString(c.badge) + 4)
		l.add(p.faint, fit(c.with, measure-l.cells, false))
		emit(l)
	}

	if c.waiting {
		blank()
		cv.card(cardAbove, 0, measure, 0)
		cardLine := func(parts ...func(*line)) {
			l := newLine()
			l.p = p.lifted()
			l.add(l.p.orange+l.p.bold, cursorBar)
			l.add("", "  ")
			for _, part := range parts {
				part(l)
			}
			emit(l)
		}
		cardLine(func(l *line) {
			l.add(l.p.orange+l.p.bold, "WAITING FOR YOU")
			if c.waited != "" {
				l.to(measure - utf8.RuneCountInString(c.waited))
				l.add(l.p.orange+l.p.bold, c.waited)
			}
		})
		if c.asked != "" {
			cardLine()
			for _, part := range wrapValue("“"+c.asked+"”", measure-4) {
				cardLine(func(l *line) { l.add(l.p.ink+l.p.bold, part) })
			}
		}
		if c.askedWith != "" {
			cardLine()
			cardLine(func(l *line) { l.add(l.p.gray, fit(c.askedWith, measure-3, false)) })
		}
		cv.card(cardBelow, 0, measure, 0)
	} else if c.standing != "" {
		blank()
		l = newLine()
		l.add(p.gray, fit(c.standing, measure, false))
		emit(l)
	}

	blank()
	l = newLine()
	l.eyebrow(0, "PROCEDURE", measure, "")
	emit(l)
	blank()
	keyCol := 0
	for _, h := range c.procedure {
		keyCol = max(keyCol, keyWidth(h.key)+3)
	}
	for i, h := range c.procedure {
		l = newLine()
		l.add(p.faint, strconv.Itoa(i+1))
		l.to(4)
		l.key(h.key)
		l.to(4 + keyCol)
		l.add(p.ink, fit(h.does, measure-l.cells, false))
		emit(l)
	}

	if c.caution != "" {
		blank()
		l = newLine()
		l.eyebrowIn(p.orange+p.bold, 0, "CAUTION", measure, "")
		emit(l)
		blank()
		for _, part := range wrapValue(c.caution, measure) {
			l = newLine()
			l.add(p.ink, part)
			emit(l)
		}
	}

	if c.story != "" {
		blank()
		l = newLine()
		l.eyebrow(0, "HOW THE WORK HAS GONE", measure, "")
		emit(l)
		blank()
		// The bar: what was worked in the running green, what it has
		// stood since beside it - in the accent where somebody is being
		// waited on, quietly where the turn is merely over - to the
		// width the caption leaves. A contact with no span conn can
		// call work gets the sentence alone; a full bar would say the
		// whole of it was work, which conn does not know.
		if c.worked > 0 || c.rested > 0 {
			caption := "worked " + span(c.worked)
			if c.rested > 0 {
				caption += " · " + c.restWord + " " + strings.ToLower(brief(c.rested))
			}
			// The caption sits after the bar where the width has room
			// for both, and under it where it has not.
			barW := measure - utf8.RuneCountInString(caption) - 2
			beside := barW >= 10
			if !beside {
				barW = measure
			}
			total := c.worked + c.rested
			rested := 0
			if total > 0 && c.rested > 0 {
				rested = max(int(float64(barW)*float64(c.rested)/float64(total)), 1)
			}
			restInk := p.gray
			if c.waiting {
				restInk = p.orange
			}
			l = newLine()
			l.add(p.running, strings.Repeat("█", barW-rested))
			if rested > 0 {
				l.add(restInk, strings.Repeat("█", rested))
			}
			if beside {
				l.add("", "  ")
			} else {
				emit(l)
				l = newLine()
			}
			l.add(p.gray, fit(caption, measure, false))
			emit(l)
			blank()
		}
		for _, part := range wrapValue(c.story, measure) {
			l = newLine()
			l.add(p.gray, part)
			emit(l)
		}
	}
	return cv.rows
}

// drawSpecs is the specifications: the eyebrow, and the figures with
// leaders under it, a row of air where the sheet leaves one.
func drawSpecs(c contactPage, measure, width int, p palette) []row {
	cv := canvas{p: p, width: width}
	cv.blank(0)
	l := cv.line()
	l.eyebrow(0, "SPECIFICATIONS", measure, "")
	cv.emit(l, 0, false)
	cv.blank(0)
	for _, f := range c.specs {
		if f.label == "" {
			cv.blank(0)
			continue
		}
		for i, part := range wrapValue(f.value, measure-specCol) {
			l := cv.line()
			if i == 0 && f.label != " " {
				l.leader(f.label, specCol-1, p.border)
			} else {
				l.to(specCol)
			}
			l.add(p.ink, part)
			cv.emit(l, 0, false)
		}
	}
	return cv.rows
}
