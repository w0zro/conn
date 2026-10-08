package main

import (
	"strconv"
	"strings"
	"time"

	"github.com/w0zro/conn/internal/draw"
	"github.com/w0zro/conn/internal/room"
	"github.com/w0zro/conn/internal/work"

	"github.com/w0zro/conn/internal/config"

	"github.com/charmbracelet/x/ansi"
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
	specs                    []draw.Fact   // the specifications, a blank label for a row of air
}

// composeContact words a contact's page.
func composeContact(s readoutSubject, home string, now time.Time) contactPage {
	e := s.entry
	name := s.sess.Name
	if name == "" {
		name = work.Program(e.AsTyped())
	}
	badge := name
	if i := strings.LastIndex(name, "-"); i >= 0 && i+1 < len(name) {
		badge = name[i+1:]
	}
	// The sheet is headed by what the session is about where the
	// transcript says; the designation keeps the handle.
	c := contactPage{badge: strings.ToUpper(badge), name: name}
	if e.Title != "" {
		c.name = e.Title
	}
	c.where = draw.Join(" · ", projectName(s.project.Path, nil, home), s.git.branch)
	if c.where == "" {
		c.where = config.Tilde(s.project.Path, home)
	}
	if a, ok := work.Contacts[work.Program(e.AsTyped())]; ok {
		c.with = draw.Join(" ", a.Name, s.sess.Version)
	}
	if s.carried.Carried > 0 {
		c.with = draw.Join(" · ", c.with, strings.ToLower(tokens(s.carried.Carried))+" of context carried")
	}

	// The question, where there is one: what it asked in its own words,
	// and with what, and when.
	c.waiting = e.Status == work.StatusWaiting
	if c.waiting {
		c.waited = work.Minutes(now.Sub(e.Since))
		if e.Since.IsZero() {
			c.waited = ""
		}
		c.asked = s.carried.Ask.Detail
		if c.asked == "" {
			c.asked = s.carried.Ask.Said
		}
		if c.asked == "" {
			c.asked = e.Asking
		}
		if s.carried.Ask.Tool != "" {
			c.askedWith = "Asked with " + s.carried.Ask.Tool
			if !e.Since.IsZero() {
				c.askedWith += " at " + e.Since.Local().Format("15:04")
			}
		}
	} else {
		c.standing = work.Said(e.Status)
		if e.Doing != "" {
			c.standing += " · " + e.Doing
		}
		if !e.Since.IsZero() && e.Status == work.StatusWorking {
			c.standing += " · for " + span(now.Sub(e.Since))
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
	if !e.Started.IsZero() {
		stopped := e.Since
		if stopped.Before(e.Started) {
			stopped = time.Time{}
		}
		atRest := (c.waiting || e.Status == work.StatusIdle) && !stopped.IsZero()
		if atRest {
			c.worked = stopped.Sub(e.Started)
			c.rested = max(now.Sub(stopped), 0)
			c.restWord = "waiting"
			if !c.waiting {
				c.restWord = "idle"
			}
		} else if e.Status == work.StatusWorking {
			c.worked = now.Sub(e.Started)
		}
		story := "It came up at " + e.Started.Local().Format("15:04")
		switch {
		case atRest && c.waiting:
			story += " and worked for " + span(c.worked) + ", then stopped to ask."
		case atRest:
			story += " and worked for " + span(c.worked) + ", then stopped."
		case e.Status == work.StatusWorking:
			story += " and has been at work for " + span(c.worked) + "."
		default:
			story += " and has been up " + span(now.Sub(e.Started)) + "."
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
			c.specs = append(c.specs, draw.Fact{Label: label, Value: value})
		}
	}
	add("Designation", strings.ToUpper(name))
	if !e.Started.IsZero() {
		add("Given", given(e.Started, now))
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
			branch = draw.Join(" · ", branch, strconv.Itoa(s.git.dirty)+" changed")
		} else {
			branch = draw.Join(" · ", branch, "clean")
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
	add("Folder", config.Tilde(s.project.Path, home))
	if e.Cwd != "" && e.Cwd != s.project.Path {
		add("Working in", config.Tilde(e.Cwd, home))
	}
	switch {
	case !s.inside:
	case room.Reachable(s.pane):
		add("Pane", s.pane.ID)
	case s.pane.Dead:
		add("Pane", s.pane.ID+" · ended")
	case s.pane.ID != "":
		add("Pane", s.pane.ID+" · cannot be reached")
	default:
		add("Pane", "None · conn did not open it")
	}
	add("Process", strconv.Itoa(e.PID))
	add("Terminal", e.TTY)
	// What stands around it: what runs it, and what it runs, each by
	// what it is and its program, the whole line being on its own page.
	if s.parent.PID != 0 {
		add("Under", work.Said(s.parent.Kind)+" "+work.Program(s.parent.AsTyped())+" · "+strconv.Itoa(s.parent.PID))
	}
	for i, k := range s.children {
		label := "Runs"
		if i > 0 {
			label = " "
		}
		add(label, work.Said(k.Kind)+" "+work.Program(k.AsTyped())+" · "+strconv.Itoa(k.PID))
	}
	add("", "")
	add("Command", e.AsTyped())
	add("Model", s.carried.Model)
	if !strings.Contains(e.AsTyped(), s.sess.SessionID) {
		add("Session", s.sess.SessionID)
	}
	if s.sess.Kind != "" && s.sess.Kind != work.InteractiveSession {
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
	for len(c.specs) > 0 && c.specs[len(c.specs)-1].Label == "" {
		c.specs = c.specs[:len(c.specs)-1]
	}
	return c
}

// span is a duration as the sheet says it: 1h 25m, 2m 14s, in the
// lower case the sheet is written in. spell is the console's, in
// capitals, and the sheet is not the console.
func span(d time.Duration) string {
	return strings.ToLower(work.Spell(d))
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
func drawContact(b readoutReport, c contactPage, width, height int, p draw.Palette) []draw.Row {
	width = max(width, draw.PanelMinCols)
	measure := draw.MeasureOf(width)
	if width < twoColumns {
		cv := draw.Canvas{P: p, Width: width}
		cv.Rows = append(cv.Rows, drawSheet(c, measure, width, p)...)
		cv.Rows = append(cv.Rows, drawSpecs(c, measure, width, p)...)
		return draw.PadTo(cv, height)
	}
	// Side by side: each column is a canvas of its own width, framed
	// with its own margins, and a row of the page is a row of each
	// joined. A plain row is trimmed as it is framed, so the left is
	// padded back out to its width before the right is put after it.
	rightW := specW + 2*draw.Margin
	leftW := width - rightW
	left := drawSheet(c, leftW-2*draw.Margin, leftW, p)
	right := drawSpecs(c, specW, rightW, p)
	cv := draw.Canvas{P: p, Width: width}
	for i := 0; i < max(len(left), len(right)); i++ {
		l, r := draw.BlankRow(p, leftW), ""
		if i < len(left) {
			l = left[i].Text
		}
		if i < len(right) {
			r = right[i].Text
		}
		if p.Plain {
			l += strings.Repeat(" ", max(leftW-ansi.StringWidth(l), 0))
			cv.Rows = append(cv.Rows, draw.Row{Text: strings.TrimRight(l+r, " ")})
			continue
		}
		if r == "" {
			r = draw.BlankRow(p, rightW)
		}
		cv.Rows = append(cv.Rows, draw.Row{Text: l + r})
	}
	return draw.PadTo(cv, height)
}

// drawSheet is the left of the page: who, the card, the procedure, the
// caution and the story, at a width of its own.
func drawSheet(c contactPage, measure, width int, p draw.Palette) []draw.Row {
	cv := draw.Canvas{P: p, Width: width}
	blank := func() { cv.Blank(0) }
	newLine := func() *draw.Line { return cv.Line() }
	emit := func(l *draw.Line) { cv.Emit(l, 0, false) }

	blank()
	l := newLine()
	l.Eyebrow(0, "CONTACT", measure, "")
	emit(l)
	blank()
	l = newLine()
	l.Add(p.Chip, " "+c.badge+" ")
	l.Add("", "  ")
	l.Add(p.Ink+p.Bold, c.name)
	if c.where != "" {
		l.Add(p.Gray, "   "+draw.Fit(c.where, measure-l.Cells-3, true))
	}
	emit(l)
	if c.with != "" {
		l = newLine()
		l.To(ansi.StringWidth(c.badge) + 4)
		l.Add(p.Faint, draw.Fit(c.with, measure-l.Cells, false))
		emit(l)
	}

	if c.waiting {
		blank()
		cv.Card(draw.CardAbove, 0, measure, 0)
		cardLine := func(parts ...func(*draw.Line)) {
			l := newLine()
			l.P = p.Lifted()
			l.Add(l.P.Orange+l.P.Bold, draw.CursorBar)
			l.Add("", "  ")
			for _, part := range parts {
				part(l)
			}
			emit(l)
		}
		cardLine(func(l *draw.Line) {
			l.Add(l.P.Orange+l.P.Bold, "WAITING FOR YOU")
			if c.waited != "" {
				l.To(measure - ansi.StringWidth(c.waited))
				l.Add(l.P.Orange+l.P.Bold, c.waited)
			}
		})
		if c.asked != "" {
			cardLine()
			for _, part := range draw.WrapValue("“"+c.asked+"”", measure-4) {
				cardLine(func(l *draw.Line) { l.Add(l.P.Ink+l.P.Bold, part) })
			}
		}
		if c.askedWith != "" {
			cardLine()
			cardLine(func(l *draw.Line) { l.Add(l.P.Gray, draw.Fit(c.askedWith, measure-3, false)) })
		}
		cv.Card(draw.CardBelow, 0, measure, 0)
	} else if c.standing != "" {
		blank()
		l = newLine()
		l.Add(p.Gray, draw.Fit(c.standing, measure, false))
		emit(l)
	}

	blank()
	l = newLine()
	l.Eyebrow(0, "PROCEDURE", measure, "")
	emit(l)
	blank()
	keyCol := 0
	for _, h := range c.procedure {
		keyCol = max(keyCol, draw.KeyWidth(h.key)+3)
	}
	for i, h := range c.procedure {
		l = newLine()
		l.Add(p.Faint, strconv.Itoa(i+1))
		l.To(4)
		l.Key(h.key)
		l.To(4 + keyCol)
		l.Add(p.Ink, draw.Fit(h.does, measure-l.Cells, false))
		emit(l)
	}

	if c.caution != "" {
		blank()
		l = newLine()
		l.EyebrowIn(p.Orange+p.Bold, 0, "CAUTION", measure, "")
		emit(l)
		blank()
		for _, part := range draw.WrapValue(c.caution, measure) {
			l = newLine()
			l.Add(p.Ink, part)
			emit(l)
		}
	}

	if c.story != "" {
		blank()
		l = newLine()
		l.Eyebrow(0, "HOW THE WORK HAS GONE", measure, "")
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
				caption += " · " + c.restWord + " " + strings.ToLower(work.Brief(c.rested))
			}
			// The caption sits after the bar where the width has room
			// for both, and under it where it has not.
			barW := measure - ansi.StringWidth(caption) - 2
			beside := barW >= 10
			if !beside {
				barW = measure
			}
			total := c.worked + c.rested
			rested := 0
			if total > 0 && c.rested > 0 {
				rested = max(int(float64(barW)*float64(c.rested)/float64(total)), 1)
			}
			restInk := p.Gray
			if c.waiting {
				restInk = p.Orange
			}
			l = newLine()
			l.Add(p.Running, strings.Repeat("█", barW-rested))
			if rested > 0 {
				l.Add(restInk, strings.Repeat("█", rested))
			}
			if beside {
				l.Add("", "  ")
			} else {
				emit(l)
				l = newLine()
			}
			l.Add(p.Gray, draw.Fit(caption, measure, false))
			emit(l)
			blank()
		}
		for _, part := range draw.WrapValue(c.story, measure) {
			l = newLine()
			l.Add(p.Gray, part)
			emit(l)
		}
	}
	return cv.Rows
}

// drawSpecs is the specifications: the eyebrow, and the figures with
// leaders under it, a row of air where the sheet leaves one.
func drawSpecs(c contactPage, measure, width int, p draw.Palette) []draw.Row {
	cv := draw.Canvas{P: p, Width: width}
	cv.Blank(0)
	l := cv.Line()
	l.Eyebrow(0, "SPECIFICATIONS", measure, "")
	cv.Emit(l, 0, false)
	cv.Blank(0)
	for _, f := range c.specs {
		if f.Label == "" {
			cv.Blank(0)
			continue
		}
		for i, part := range draw.WrapValue(f.Value, measure-specCol) {
			l := cv.Line()
			if i == 0 && f.Label != " " {
				l.Leader(f.Label, specCol-1, p.Border)
			} else {
				l.To(specCol)
			}
			l.Add(p.Ink, part)
			cv.Emit(l, 0, false)
		}
	}
	return cv.Rows
}
