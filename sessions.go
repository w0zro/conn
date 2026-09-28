package main

import (
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/w0zro/conn/internal/config"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
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
	caret   int // where in the filter the caret is, in runes
}

// composeSessions words the sessions view: the filter's rows out of the
// whole number found.
func composeSessions(sessions []session, project, filter, home string, now time.Time, loading bool) sessionsReport {
	b := composeSessionsAt(sessions, project, filter, home, now, loading)
	b.caret = utf8.RuneCountInString(filter)
	return b
}

// composeSessionsAt is composeSessions with the caret left at the
// start, for the view to put where it is.
func composeSessionsAt(sessions []session, project, filter, home string, now time.Time, loading bool) sessionsReport {
	return sessionsReport{
		project: config.Tilde(project, home), home: home, now: now, loading: loading,
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
	measure := measureAt(width)
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
	l.to(measure - ansi.StringWidth(right))
	l.add(p.gray, right)
	c.emit(l, 0, false)
	c.rule(0, measure)

	// The project it is for, the way a project titles its block in the
	// processes view.
	l = c.line()
	l.add(p.parchment+p.bold, fit(b.project, measure, true))
	c.emit(l, 0, false)

	// The line typed into: the word, and the filter with the caret in
	// it, so it is plain that the keys go here.
	l = c.line()
	before, after := typedRuns(b.filter, b.caret, measure-findW-2, false)
	l.field(0, measure-findW, "FIND", before, after)
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
				prompt, path = config.Tilde(cv.Dir, b.home), true
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

// The sessions view as the panel holds it: a project's suspended
// sessions as last read, and the line typed into to narrow them, with
// the cursor among the rows it leaves.
type sessionList struct {
	project string   // what the view is for
	dirs    []string // the directories asked for; a stale answer's guard
	read    []session
	loading bool
	find    typed
}

// landed takes a reading of sessions, and says whether it was this
// view's: one opened on another project since has moved past the
// answer.
func (l *sessionList) landed(msg sessionsMsg) bool {
	if !slices.Equal(msg.dirs, l.dirs) {
		return false
	}
	l.read, l.loading = msg.sessions, false
	l.find.at = clamp(l.find.at, len(l.rows()))
	return true
}

// rows is the sessions the line leaves, which the cursor is an index
// into, and at the one the cursor is on, where there is one.
func (l sessionList) rows() []session {
	return matchingSessions(l.read, l.find.text)
}

func (l sessionList) at() (session, bool) {
	rows := l.rows()
	if l.find.at >= len(rows) {
		return session{}, false
	}
	return rows[l.find.at], true
}

// report is the sessions view's words as things stand.
func (l sessionList) report(home string, now time.Time) sessionsReport {
	b := composeSessionsAt(l.read, l.project, l.find.text, home, now, l.loading)
	b.caret = l.find.cur
	return b
}

// openSessions opens the sessions view over a project's suspended
// sessions: project is what it is for, and dirs the directories a
// transcript could be filed under, which for a group is a repository
// under it, not the folder that names it.
func (m model) openSessions(project string, dirs []string) (model, tea.Cmd) {
	m.view = viewSessions
	m.sessions = sessionList{project: project, dirs: dirs, loading: true}
	return m, m.scanSessions(dirs)
}

// sessionsKey answers a key on the sessions view, which is a line typed
// into the same way the list is; see typed. What is the view's own:
// enter continues the session under the cursor and goes back to the
// processes view, esc goes back without continuing anything, and ctrl+c
// is what it is everywhere.
func (m model) sessionsKey(k string) (model, tea.Cmd) {
	switch {
	case m.sessions.find.edit(k, len(m.sessions.rows())):
	case k == "ctrl+c":
		return m.leave()
	case k == "esc":
		return m.backFrom()
	case k == "enter":
		if c, ok := m.sessions.at(); m.inside && !m.sessions.loading && ok {
			var cmd tea.Cmd
			m, cmd = m.toProcesses()
			return m, tea.Batch(cmd, m.openResumed(c.Dir, c.ID))
		}
	}
	return m, nil
}
