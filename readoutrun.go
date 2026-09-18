package main

import (
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// conn readout <pid> — the readout's own program, run in a pane of
// conn's server the way the hold is. It reads the panel's reading of
// the machine rather than the machine: the panel publishes what it
// shows beside its cursor (see cursor.go), and the page says the row
// the panel says, with what stands around it, and asks the machine
// nothing. What it asks for itself is about one row only — what a
// contact says of its session, and what git says of the project — and
// those it asks on its own beat and keeps.
//
// A key passes focus back to the panel, the way the hold does. There
// is nothing to do on the page — it is a reading — and the keys that
// work the processes view all live on the panel, so the useful thing a
// keypress here can mean is "put me back where the keys are".
//
// With no pid it follows the panel's cursor, which is how the panel
// opens it: the page is about whatever the cursor is on, so j and k
// read down the list with the page keeping up rather than leaving it
// on a row nobody is looking at any more. With a pid it stays on that
// pid, which is what `conn readout 123`, typed, is for.

// readoutBeat is how often the page asks again after what is about the
// one row: the contact's session and, under its own longer while, git.
// The processes view's own beat, so the page is no staler than the
// panel about the things the panel does not carry.
//
// readoutPoll is how often it asks what the panel has said, which is a
// read of a small file and can afford to be quick. It has to be:
// nothing else stands between a key on the panel and the page
// changing, so the poll is the whole of the wait, and a wait long
// enough to see is a page that trails the cursor down the list.
//
// gitEvery is how often git is asked about a project the page is
// already showing. Git is four processes a reading and the branch does
// not move on a beat.
const (
	readoutBeat = 2 * time.Second
	readoutPoll = 50 * time.Millisecond
	gitEvery    = 30 * time.Second
)

type readoutModel struct {
	srv           *server
	at            subject
	follow        bool   // the subject is the panel's cursor, not a pid given
	cursor        string // where the panel publishes it
	note          string // what the panel last said there, to tell a change by
	width, height int
	p             palette
	report        readoutReport
	table         readoutTable // what the page was last composed out of
	read          time.Time    // when the subject was last asked after in full
	inflight      bool         // an asking is out and has not landed
}

// readoutReadMsg carries an asking, and the pid it was of: several can
// be in flight at once when the cursor is moving, and one that lands
// after the subject has changed again is stale and dropped. The table
// it was made from is not dropped with it — what git and claude said
// is as good for the next row as for the one it was asked for, and is
// kept.
type readoutReadMsg struct {
	at     subject
	report readoutReport
	table  readoutTable
	ok     bool // the subject was found, and the report is about it
}

func runReadout(srv *server, pid int, home string, p palette) error {
	m := readoutModel{srv: srv, at: subject{pid: pid}, follow: pid == 0, cursor: cursorPath(home), p: p,
		report: readoutReport{pid: pid}}
	_, err := tea.NewProgram(m, programOptions()...).Run()
	return err
}

func (m readoutModel) Init() tea.Cmd {
	_, cmd := m.reading()
	return cmd
}

func (m readoutModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case readoutReadMsg:
		// An asking about a subject that has since moved on is no longer
		// about anything, and putting it up would be a page flicking
		// back to a row the cursor has left. What was asked is kept
		// whatever row it was asked for.
		m.table.sess, m.table.git, m.table.carried = msg.table.sess, msg.table.git, msg.table.carried
		if msg.at == m.at {
			if msg.ok {
				m.report = msg.report
			}
			m.inflight = false
		}
		// A page pinned to a pid that has gone has nothing left to ask
		// after: the row will not come back.
		if !m.follow && m.report.gone {
			return m, nil
		}
	case readoutTickMsg:
		// What the panel says, then whether that is news. The reading
		// the panel published is taken as the machine; a subject that
		// moved is asked after at once, and one that has not is asked
		// after on the beat, so a page nobody is moving still keeps up
		// with its row.
		if note := readCursor(m.cursor); note != m.note {
			m.note = note
			at, r := parseCursor(note)
			if r != nil {
				m.table = m.table.on(*r)
			}
			moved := false
			if m.follow && !at.none() && at != m.at {
				m.at, moved = at, true
			}
			// The row is answered now, out of what the panel said, and
			// the asking only adds to that answer what is about the row
			// alone. A row that stayed put and changed its word is
			// answered the same way: the panel said so, and the page
			// says it now. A row the panel's reading has not got is a
			// row that has gone, since the panel's cursor is always in
			// the panel's own reading and a pinned pid is the only kind
			// that can be missing from it.
			if page, ok := readoutPage(m.at, m.table); ok {
				m.report = page
			} else if m.table.published && m.at.pid != 0 {
				m.report = readoutReport{pid: m.at.pid, gone: true}
			}
			if moved {
				return m.reading()
			}
		}
		// One asking at a time on the beat: git under its wait can take
		// longer than a beat, and a second asking behind it would only
		// queue a third.
		if time.Since(m.read) >= readoutBeat && !m.inflight {
			return m.reading()
		}
		return m, m.tick()
	case tea.KeyPressMsg:
		if m.srv != nil {
			return m, func() tea.Msg { _ = m.srv.focusPanel(); return nil }
		}
	}
	return m, nil
}

type readoutTickMsg struct{}

func (m readoutModel) tick() tea.Cmd {
	return tea.Tick(readoutPoll, func(time.Time) tea.Msg { return readoutTickMsg{} })
}

// reading asks after the subject and words the page, off the loop:
// what claude says of the row's session and what git says of its
// project are each an asking, and git is a process besides. It marks
// the model as having asked, so the two go together and neither can be
// done without the other.
//
// The table the page holds goes with it, so what conn has already
// asked of a project or a session is not asked again from nothing.
func (m readoutModel) reading() (readoutModel, tea.Cmd) {
	m.read, m.inflight = time.Now(), true
	at, held := m.at, m.table
	return m, tea.Batch(
		func() tea.Msg {
			report, table, ok := readoutOf(at, held)
			return readoutReadMsg{at: at, report: report, table: table, ok: ok}
		},
		m.tick(),
	)
}

// readoutTable is what a page is composed out of: the panel's reading
// of the machine, the same for every row — the rows, the record behind
// each, the panes, the containers — and beside it what claude and git
// have said of the rows asked after so far, which are askings about
// one row and are kept rather than thrown away with the page they were
// for.
type readoutTable struct {
	reading
	published bool // the panel has published a reading at all
	sess      map[int]sessionFile
	git       map[string]gitStatus // what git said of a project, by its path
	carried   map[int]session      // which session a row was carrying
}

// on is the table with the panel's latest reading for its machine, and
// the askings kept.
func (t readoutTable) on(r reading) readoutTable {
	t.reading, t.published = r, true
	return t
}

// readoutOf is the page for a subject as things stand and the table it
// was composed from, and whether the subject was found in it at all. No
// subject is a page waiting on a cursor that has not said where it is
// yet.
func readoutOf(at subject, held readoutTable) (readoutReport, readoutTable, bool) {
	if at.none() {
		return readoutReport{}, held, false
	}
	t := readoutGather(at, held)
	r, ok := readoutPage(at, t)
	return r, t, ok
}

// readoutGather asks what only the row's own project and session can
// answer. What it is told of those joins what it was told of the rows
// asked after before, since a list walked down and back up again is
// the same few projects over and over and git is a process each time.
func readoutGather(at subject, held readoutTable) readoutTable {
	// Copied rather than written into, because the page goes on reading
	// the table it holds while this one is being made.
	t := held
	t.git, t.carried = maps.Clone(held.git), maps.Clone(held.carried)
	if t.git == nil {
		t.git = map[string]gitStatus{}
	}
	if t.carried == nil {
		t.carried = map[int]session{}
	}
	// A project is asked after by git alone: the rows in it are the
	// panel's, already in hand. A session the same, for the project it
	// was had in: what it says of itself the panel read already.
	if at.path != "" {
		t.askGit(at.path)
		return t
	}
	if at.session != "" {
		if c := t.sessionOf(at.session); c != nil {
			t.askGit(c.Dir)
		}
		return t
	}
	pid := at.pid
	t.sess = claudeSessions()

	s, ok := subjectOf(pid, t.projects, t.records)
	if !ok {
		return t
	}
	// Which session a contact is carrying — the session file names
	// it, and the transcript is where the branch and the last ask are.
	if s.entry.kind == kindContact {
		if f := t.sess[pid]; f.SessionID != "" && f.wroteBy(s.entry.started) {
			dir := s.entry.cwd
			if f.Cwd != "" {
				dir = f.Cwd
			}
			c := session{ID: f.SessionID, Dir: dir}
			readSessionMeta(sessionPath(dir, f.SessionID), &c)
			// What it is waiting on is read for a waiting row, and read
			// again only when its status changed: the transcript is
			// the same file until it does.
			if s.entry.status == statusWaiting {
				if was, ok := held.carried[pid]; ok && was.Ask != (ask{}) && was.AskAt.Equal(s.entry.since) {
					c.Ask, c.AskAt = was.Ask, was.AskAt
				} else {
					c.Ask, c.AskAt = readAsk(sessionPath(dir, f.SessionID)), s.entry.since
				}
			}
			t.carried[pid] = c
		}
	}
	t.askGit(s.project.path)
	return t
}

// askGit asks git about a project, again only after its own while: a
// page left open on one row is the same project every beat.
func (t readoutTable) askGit(path string) {
	if g, ok := t.git[path]; !ok || time.Since(g.read) >= gitEvery {
		g = readGit(path)
		g.read = time.Now()
		t.git[path] = g
	}
}

// readoutPage words a subject out of a table. A project is always there
// to be worded, being a place rather than a row. Of a row it says
// whether the row is there at all: a row the panel's reading has not
// got is not on the page, and what that means — gone, or not yet said —
// is the caller's to decide.
func readoutPage(at subject, t readoutTable) (readoutReport, bool) {
	home, _ := os.UserHomeDir()
	if at.path != "" {
		return composeProject(at.path, t, home, time.Now()), true
	}
	if at.session != "" {
		c := t.sessionOf(at.session)
		if c == nil {
			return readoutReport{}, false
		}
		return composeSession(*c, t, home, time.Now()), true
	}
	pid := at.pid
	s, ok := subjectOf(pid, t.projects, t.records)
	if !ok {
		return readoutReport{}, false
	}
	s.container = t.containerOf(s.entry)
	s.brew = t.brewOf(s.entry)
	if t.inside {
		s.pane, s.inside = t.panes[s.entry.tty], true
	}
	if s.entry.kind == kindContact {
		s.sess, s.carried = t.sess[pid], t.carried[pid]
	}
	s.git = t.git[s.project.path]
	return composeReadout(s, home, time.Now()), true
}

// subjectOf finds a pid among the projects and gathers what stands
// around it: the table's own record, its project, what runs it and what
// it runs.
func subjectOf(pid int, projects []project, records map[int]record) (readoutSubject, bool) {
	for _, pl := range projects {
		for i, e := range pl.entries {
			if e.pid != pid {
				continue
			}
			s := readoutSubject{entry: e, proc: records[pid], project: rowsBlock(projects, e, pl)}
			// The tree is written depth first, so what runs this row is
			// the nearest row above it that is a level shallower, and
			// what it runs is the rows below it until the depth comes
			// back to its own.
			for j := i - 1; j >= 0; j-- {
				if pl.entries[j].depth < e.depth {
					s.parent = pl.entries[j]
					break
				}
			}
			for j := i + 1; j < len(pl.entries); j++ {
				if pl.entries[j].depth <= e.depth {
					break
				}
				if pl.entries[j].depth == e.depth+1 {
					s.children = append(s.children, pl.entries[j])
				}
			}
			// A row filed by state stands alone, and what runs it and what
			// it runs are filed elsewhere: they share its terminal, and
			// stood a level above and a level below it in the tree it was
			// read from.
			if e.filed {
				s.parent, s.children = entry{}, nil
				for _, pl := range projects {
					for _, c := range pl.entries {
						if c.pid == e.pid || c.tty != e.tty {
							continue
						}
						switch {
						case depthOf(c) < e.fromDepth:
							if s.parent.pid == 0 || depthOf(c) > depthOf(s.parent) {
								s.parent = c
							}
						case depthOf(c) == e.fromDepth+1:
							s.children = append(s.children, c)
						}
					}
				}
			}
			return s, true
		}
	}
	return readoutSubject{}, false
}

// sessionPath is where claude files a session had in a directory.
func sessionPath(dir, id string) string {
	return filepath.Join(claudeConfigDir(), "projects", encodePath(dir), id+".jsonl")
}

func (m readoutModel) View() tea.View {
	rows := drawReadout(m.report, max(m.width, 1), m.height, m.p)
	texts := make([]string, len(rows))
	for i, r := range rows {
		texts[i] = r.text
	}
	v := tea.NewView(strings.Join(texts, "\n"))
	v.AltScreen = true
	return v
}
