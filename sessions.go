package main

import (
	"cmp"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/w0zro/conn/internal/work"

	"github.com/w0zro/conn/internal/config"

	tea "charm.land/bubbletea/v2"
)

// The sessions view: a project's suspended sessions, filtered the way
// the project list is. A row is what a reader would recognize a session
// by — its branch and the last thing it was asked — with how long since
// it last moved against the right. Enter continues the one under the
// cursor, in a shell like any other; esc leaves the view without
// opening anything.
//
// The same view over every project is the recent view, r: what was
// being done anywhere, rather than what was being done here. The two
// are kept apart because they are different questions. A row there is
// named by its project in the branch's column, and says what the
// session is called rather than what it was last asked, since a list
// across every project is read for which piece of work it was.

// sessionsReport is the sessions view's words as things stand. A
// directory the sessions view looked under and found nothing in — no
// sessions had, or none yet read — is not an error; there is none for
// the sessions view to say.
type sessionsReport struct {
	project string            // the project it was opened on, as the panel names its block; empty for the recent view
	recent  bool              // every project's, named by project and by title
	names   map[string]string // the project each row's directory is in, by its leaf, for the recent view
	home    string
	now     time.Time
	loading bool
	rows    []work.Session
	total   int
	filter  string
	caret   int // where in the filter the caret is, in runes
}

// composeSessions words the sessions view: the filter's rows out of the
// whole number found.
func composeSessions(sessions []work.Session, project, filter, home string, roots []string, now time.Time, loading bool) sessionsReport {
	b := composeSessionsAt(sessions, project, filter, home, roots, now, loading)
	b.caret = utf8.RuneCountInString(filter)
	return b
}

// composeSessionsAt is composeSessions with the caret left at the
// start, for the view to put where it is. The project is named the way
// the panel names its block, from the root the checkouts are kept
// under; see projectName.
func composeSessionsAt(sessions []work.Session, project, filter, home string, roots []string, now time.Time, loading bool) sessionsReport {
	return sessionsReport{
		project: projectName(project, roots, home), home: home, now: now, loading: loading,
		rows: matchingSessions(sessions, filter), total: len(sessions), filter: filter,
	}
}

// matchingSessions is the sessions a filter leaves: one answers by
// its branch, the last thing it was asked, or the directory it was had
// in, the same three a reader would recognize it by.
func matchingSessions(cs []work.Session, filter string) []work.Session {
	f := strings.ToLower(strings.TrimSpace(filter))
	if f == "" {
		return cs
	}
	var out []work.Session
	for _, c := range cs {
		if strings.Contains(strings.ToLower(c.Prompt), f) ||
			strings.Contains(strings.ToLower(c.Title), f) ||
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
func drawSessions(b sessionsReport, cursor, width, height int, p Palette) []Row {
	// The header: the name of the view, and against the right the count
	// — of everything found at the project, or of what the filter left out
	// of it.
	word := "SESSIONS"
	if b.recent {
		word = "RECENT"
	}
	right := strconv.Itoa(b.total) + " SUSPENDED"
	switch {
	case b.loading && b.total == 0:
		right = "" // nothing has been found yet, and none is not a count
	case b.filter != "":
		right = strconv.Itoa(len(b.rows)) + " OF " + strconv.Itoa(b.total)
	}
	c, measure := Head(word, right, width, p)

	// The project it is for, the way a project titles its block in the
	// processes view; the recent view is for every one.
	l := c.Line()
	if b.recent {
		l.Add(p.Parchment+p.Bold, "Every project")
	} else {
		l.Add(p.Parchment+p.Bold, Fit(b.project, measure, true))
	}
	c.Emit(l, 0, false)

	// The line typed into: the word, and the filter with the caret in
	// it, so it is plain that the keys go here.
	l = c.Line()
	before, after := TypedRuns(b.filter, b.caret, measure-findW-2, false)
	l.Field(0, measure-findW, "FIND", before, after)
	c.Emit(l, 0, false)

	var body []Row
	cursorRow := -1
	d := Canvas{P: p, Width: c.Width}
	say := func(color, s string) {
		d.Blank(0)
		l := d.Line()
		l.Add(color, s)
		d.Emit(l, 0, true)
		body = d.Rows
	}
	switch {
	case b.loading && len(b.rows) == 0:
		say(p.Gray, "LOOKING")
	case len(b.rows) == 0 && b.filter != "":
		say(p.Gray, "NOTHING ANSWERS TO "+strings.ToUpper(b.filter))
	case len(b.rows) == 0 && b.recent:
		say(p.Gray, "NOTHING SUSPENDED")
	case len(b.rows) == 0:
		say(p.Gray, "NOTHING SUSPENDED HERE")
	default:
		d.Blank(0)
		ageCol := measure - sessionsAgeW
		promptW := ageCol - 1 - branchW
		for i, cv := range b.rows {
			l := d.Line()
			if i == cursor {
				l.P = p.Chosen()
				if p.Plain {
					l.Mark = "▸"
				}
				cursorRow = len(d.Rows)
			}
			first, prompt := cv.Branch, cv.Prompt
			if b.recent {
				first, prompt = b.names[cv.Dir], cmp.Or(cv.Title, cv.Prompt)
			}
			l.Add(p.Gray, Fit(first, branchW-1, false))
			l.To(branchW)
			// A session with nothing read of it is named by where it
			// was had; one with a prompt is named by that instead, since it
			// is the more of the two a reader would recognize it by.
			path := false
			if prompt == "" {
				prompt, path = config.Tilde(cv.Dir, b.home), true
			}
			l.Add(p.Ink, Fit(prompt, promptW, path))
			l.To(ageCol)
			l.Add(p.Gray, work.Age(cv.When, b.now))
			d.Emit(l, 0, false)
		}
		body = d.Rows
	}
	return c.Foot(body, cursorRow, height)
}

// The sessions view as the panel holds it: a project's suspended
// sessions as last read, and the line typed into to narrow them, with
// the cursor among the rows it leaves.
type sessionList struct {
	project string   // what the view is for
	dirs    []string // the directories asked for; a stale answer's guard
	recent  bool     // every project's, and no directories asked for
	read    []work.Session
	loading bool
	find    typed
}

// landed takes a reading of sessions, and says whether it was this
// view's: one opened on another project since has moved past the
// answer.
func (l *sessionList) landed(msg sessionsMsg) bool {
	if msg.recent != l.recent || !slices.Equal(msg.dirs, l.dirs) {
		return false
	}
	l.read, l.loading = msg.sessions, false
	l.find.at = clamp(l.find.at, len(l.rows()))
	return true
}

// rows is the sessions the line leaves, which the cursor is an index
// into, and at the one the cursor is on, where there is one.
func (l sessionList) rows() []work.Session {
	return matchingSessions(l.read, l.find.text)
}

func (l sessionList) at() (work.Session, bool) {
	rows := l.rows()
	if l.find.at >= len(rows) {
		return work.Session{}, false
	}
	return rows[l.find.at], true
}

// report is the sessions view's words as things stand. rootOf is the
// project that holds a directory, which the recent view names a row by.
func (l sessionList) report(home string, roots []string, now time.Time, rootOf func(string) string) sessionsReport {
	b := composeSessionsAt(l.read, l.project, l.find.text, home, roots, now, l.loading)
	b.caret = l.find.cur
	if l.recent {
		b.recent, b.names = true, map[string]string{}
		for _, c := range b.rows {
			root := c.Dir
			if rootOf != nil {
				root = cmp.Or(rootOf(c.Dir), c.Dir)
			}
			b.names[c.Dir] = filepath.Base(root)
		}
	}
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

// openRecent opens the sessions view over every project's suspended
// sessions, newest first: the recent view. came is the pane the keys
// were in, for esc to go back to.
func (m model) openRecent(came string) (model, tea.Cmd) {
	if !m.inside {
		return m, nil
	}
	m.from = came
	m.view = viewSessions
	m.sessions = sessionList{recent: true, loading: true}
	projects := m.projects
	return m, func() tea.Msg {
		return sessionsMsg{recent: true, sessions: work.ClaudeRecent(projects)}
	}
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
