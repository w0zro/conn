package main

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// The sessions view: a project's suspended sessions, filtered the way
// the project list is. A row is what a reader would recognize a session
// by — its branch and the last thing it was asked — with how long since
// it last moved against the right. Enter continues the one under the
// cursor, in a shell like any other; esc leaves the view without
// opening anything.

// sessionsReport is the sessions view's words as things stand. A
// directory the sessions view looked under and found nothing in — no
// sessions had, or none yet read — is not an error; there is none for
// the sessions view to say.
type sessionsReport struct {
	project string // the project it was opened on, tilde'd
	home    string
	now     time.Time
	loading bool
	rows    []session
	total   int
	filter  string
}

// composeSessions words the sessions view: the filter's rows out of the
// whole number found.
func composeSessions(sessions []session, project, filter, home string, now time.Time, loading bool) sessionsReport {
	return sessionsReport{
		project: tilde(project, home), home: home, now: now, loading: loading,
		rows: matchingSessions(sessions, filter), total: len(sessions), filter: filter,
	}
}

// matchingSessions is the sessions a filter leaves: one answers by
// its branch, the last thing it was asked, or the directory it was had
// in, the same three a reader would recognize it by.
func matchingSessions(cs []session, filter string) []session {
	f := strings.ToLower(strings.TrimSpace(filter))
	if f == "" {
		return cs
	}
	var out []session
	for _, c := range cs {
		if strings.Contains(strings.ToLower(c.Prompt), f) ||
			strings.Contains(strings.ToLower(c.Branch), f) ||
			strings.Contains(strings.ToLower(c.Dir), f) {
			out = append(out, c)
		}
	}
	return out
}

// branchW is the sessions view's one column of its own: the branch,
// from the margin. The age takes a column of its own, flush with the
// right; the prompt takes what is left between them. It keeps two units
// where the processes view's own column went to one: a session is
// picked from a list of them by how long ago it was had, which is a
// thing to compare rather than to glance at.
const (
	branchW      = 14
	sessionsAgeW = 9 // the widest age, in two units
)

// drawSessions renders the sessions view for a terminal of the given
// size, with the cursor on the given row.
func drawSessions(b sessionsReport, cursor, width, height int, p palette) []row {
	width = max(width, panelMinCols)
	measure, _, _ := columns(width)
	c := canvas{p: p, width: width}

	// The header: the name of the view, and against the right the count
	// — of everything found at the project, or of what the filter left out
	// of it.
	c.blank(0)
	l := c.line()
	l.add(p.orange+p.bold, "SESSIONS")
	right := strconv.Itoa(b.total) + " SUSPENDED"
	switch {
	case b.loading && b.total == 0:
		right = "" // nothing has been found yet, and none is not a count
	case b.filter != "":
		right = strconv.Itoa(len(b.rows)) + " OF " + strconv.Itoa(b.total)
	}
	l.to(measure - utf8.RuneCountInString(right))
	l.add(p.gray, right)
	c.emit(l, 0, false)
	c.rule(0, measure)

	// The project it is for, the way a project titles its block in the
	// processes view.
	l = c.line()
	l.add(p.parchment+p.bold, fit(b.project, measure, true))
	c.emit(l, 0, false)

	// The line typed into: the word, and the filter with the caret after
	// it, so it is plain that the keys go here.
	l = c.line()
	l.add(p.gray, "FIND")
	l.to(findW)
	l.add(p.ink+p.bold, fit(b.filter, measure-findW-1, false))
	l.add(p.orange+p.bold, caret)
	c.emit(l, 0, false)

	room := height
	if height == 0 {
		room = 1 << 30
	}
	var body []row
	cursorRow := -1
	d := canvas{p: p, width: width}
	say := func(color, s string) {
		d.blank(0)
		l := d.line()
		l.add(color, s)
		d.emit(l, 0, true)
		body = d.rows
	}
	switch {
	case b.loading && len(b.rows) == 0:
		say(p.gray, "LOOKING")
	case len(b.rows) == 0 && b.filter != "":
		say(p.gray, "NOTHING ANSWERS TO "+strings.ToUpper(b.filter))
	case len(b.rows) == 0:
		say(p.gray, "NOTHING SUSPENDED HERE")
	default:
		d.blank(0)
		ageCol := measure - sessionsAgeW
		promptW := ageCol - 1 - branchW
		for i, cv := range b.rows {
			l := d.line()
			if i == cursor {
				l.p = p.chosen()
				if p.plain {
					l.mark = "▸"
				}
				cursorRow = len(d.rows)
			}
			l.add(p.gray, fit(cv.Branch, branchW-1, false))
			l.to(branchW)
			// A session with nothing read of it is named by where it
			// was had; one with a prompt is named by that instead, since it
			// is the more of the two a reader would recognize it by.
			prompt, path := cv.Prompt, false
			if prompt == "" {
				prompt, path = tilde(cv.Dir, b.home), true
			}
			l.add(p.ink, fit(prompt, promptW, path))
			l.to(ageCol)
			l.add(p.gray, age(cv.When, b.now))
			d.emit(l, 0, false)
		}
		body = d.rows
	}
	c.rows = append(c.rows, scrolled(body, cursorRow, room-len(c.rows), width, p)...)

	// The ground fills what the rows do not, as in projects.
	if height > 0 {
		for len(c.rows) < height {
			c.blank(0)
		}
	}
	return c.rows
}
