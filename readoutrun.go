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
// conn's server the way the hold is. It reads for itself rather than
// being told: the panel's reading is the panel's, the page wants more
// of it than a row carries, and a pane that reads its own subject keeps
// saying the truth while the panel is busy elsewhere.
//
// A key processes focus back to the panel, the way the hold does. There
// is nothing to do on the page — it is a reading — and the keys that
// work the processes view all live on the panel, so the useful thing a
// keypress here can mean is "put me back where the keys are".
//
// With no pid it follows the panel's cursor, which is what i opens: the
// page is about whatever the cursor is on, so j and k read down the
// list with the page keeping up rather than leaving it on a row nobody
// is looking at any more. With a pid it stays on that pid, which is
// what `conn readout 123`, typed, is for.

// readoutBeat is how often the page reads its subject again. The
// processes view's own beat: the two are readings of the same table and
// there is no reason for one to be staler than the other.
//
// readoutPoll is how often it asks where the cursor is, which is a read
// of a few bytes rather than the whole table and can afford to be
// quick. It has to be: nothing else stands between a key on the panel
// and the page changing, so the poll is the whole of the wait, and a
// wait long enough to see is a page that trails the cursor down the
// list.
//
// gitEvery is how often git is asked about a project the page is
// already showing. Git is four processes a reading and the branch does
// not move on a beat; the table is read every beat because a process's
// status does, and that is one process.
const (
	readoutBeat = 2 * time.Second
	readoutPoll = 50 * time.Millisecond
	gitEvery    = 30 * time.Second
)

type readoutModel struct {
	srv           *server
	pid           int
	follow        bool   // the subject is the panel's cursor, not a pid given
	cursor        string // where the panel publishes it
	width, height int
	p             palette
	report        readoutReport
	table         readoutTable // what the page was last composed out of
	read          time.Time    // when the subject was last read in full
	inflight      bool         // a reading is out and has not landed
}

// readoutReadMsg carries a reading, and the pid it was of: several can
// be in flight at once when the cursor is moving, and one that lands
// after the subject has changed again is stale and dropped. The table
// it was made from is not dropped with it — that is a reading of the
// machine rather than of the row, and it is as good for one row as
// another.
type readoutReadMsg struct {
	pid    int
	report readoutReport
	table  readoutTable
}

func runReadout(srv *server, pid int, home string, p palette) error {
	m := readoutModel{srv: srv, pid: pid, follow: pid == 0, cursor: cursorPath(home), p: p,
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
		// A reading of a subject that has since moved on is no longer
		// about anything, and putting it up would be a page flicking
		// back to a row the cursor has left. The table is kept whatever
		// row it was read for: it is the machine, and the next row the
		// cursor lands on is in it too.
		m.table = msg.table
		if msg.pid == m.pid {
			m.report = msg.report
			m.inflight = false
		}
		// A page pinned to a pid that has gone has nothing left to read:
		// the row will not come back, and a reading every beat forever
		// to say so is a process every beat for nothing.
		if !m.follow && m.report.gone {
			return m, nil
		}
	case readoutTickMsg:
		// Where the cursor is, then whether that is news. A subject that
		// changed is read at once; one that has not is read on the beat,
		// so a page nobody is moving still keeps up with its row.
		if m.follow {
			if pid := askCursor(m.cursor); pid != 0 && pid != m.pid {
				m.pid = pid
				// The row is answered now, out of the table already
				// read, and the reading only replaces that answer with
				// a newer one. Reading the machine takes a tenth of a
				// second, which is long enough to see, and waiting it
				// out would leave the page on the row the cursor just
				// left — while the table read a moment ago has the new
				// row in it, as true as the panel's own list is.
				if r, ok := readoutPage(pid, m.table); ok {
					m.report = r
				}
				return m.reading()
			}
		}
		// One reading at a time on the beat: git under its wait can take
		// longer than a beat, and a second reading behind it would only
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

// reading gathers the subject and words it, off the loop: the process
// table, the tree the row sits in, what claude says of it, and what git
// says of its project are each a reading, and git is a process besides.
// It marks the model as having read, so the two go together and neither
// can be done without the other.
//
// The table the page holds goes with it, so what conn has already asked
// of a project or a session is not asked again from nothing.
func (m readoutModel) reading() (readoutModel, tea.Cmd) {
	m.read, m.inflight = time.Now(), true
	pid, srv, held := m.pid, m.srv, m.table
	return m, tea.Batch(
		func() tea.Msg {
			report, table := readoutOf(pid, srv, held)
			return readoutReadMsg{pid: pid, report: report, table: table}
		},
		m.tick(),
	)
}

// readoutTable is what a page is composed out of: the process table and
// the projects conn makes of it, what conn's server holds of panes,
// what claude says of its sessions — readings of the whole machine, the
// same for every row — and beside them what git and claude's
// transcripts have said of the rows read so far, which are askings
// about one row and are kept rather than thrown away with the page they
// were for.
//
// The page holds the last one, because the cursor moves faster than the
// machine can be read and every row the cursor can land on is already
// in the table that was read for the row it is leaving.
type readoutTable struct {
	procs    []process
	projects []project
	panes    map[string]pane
	inside   bool // there was a server to ask about panes
	sess     map[int]sessionFile
	git      map[string]gitStatus // what git said of a project, by its path
	carried  map[int]session      // which session a row was carrying
	// Each row as this reading saw it stand, and when it was taken, so
	// the next reading can date a row's status the way the panel does.
	stood  map[int]stood
	readAt time.Time
}

// readoutOf is the page for a pid as things stand and the table it was
// read from, or the page that says the row has gone. A pid of nothing
// is a page waiting on a cursor that has not said where it is yet.
func readoutOf(pid int, srv *server, held readoutTable) (readoutReport, readoutTable) {
	if pid == 0 {
		return readoutReport{}, held
	}
	t := readoutGather(pid, srv, held)
	r, ok := readoutPage(pid, t)
	if !ok {
		return readoutReport{pid: pid, gone: true}, t
	}
	return r, t
}

// readoutGather reads the machine, and then asks what only the row's
// own project and session can answer. What it is told of those joins
// what it was told of the rows read before, since a list walked down
// and back up again is the same few projects over and over and git is a
// process each time.
func readoutGather(pid int, srv *server, held readoutTable) readoutTable {
	// Copied rather than written into, because the page goes on reading
	// the table it holds while this one is being made.
	t := readoutTable{git: maps.Clone(held.git), carried: maps.Clone(held.carried)}
	if t.git == nil {
		t.git = map[string]gitStatus{}
	}
	if t.carried == nil {
		t.carried = map[int]session{}
	}

	uid := os.Getuid()
	procs, err := readProcesses(uid)
	if err != nil {
		return t
	}
	t.procs = procs
	home, _ := os.UserHomeDir()
	roots, _ := projectRoots(home)
	isProject := projectDirs(roots)
	t.projects = projectsFrom(procs, uid, rootFinder(isProject), isProject, contactStatuses(procs))
	t.readAt = time.Now()
	t.stood = sinceSeen(t.projects, held.stood, held.readAt, t.readAt)

	// What conn holds for the rows' terminals, when there is a server to
	// ask. Outside one there is nothing to say of panes.
	if srv != nil {
		if panes, err := srv.panes(); err == nil {
			t.panes, t.inside = panes, true
		}
	}
	t.sess = claudeSessions()

	s, ok := subjectOf(pid, t.projects, t.procs)
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
	// What git says of the project, asked again only after its own while:
	// a page left open on one row is the same project every beat.
	if g, ok := t.git[s.project.path]; !ok || time.Since(g.read) >= gitEvery {
		g = readGit(s.project.path)
		g.read = time.Now()
		t.git[s.project.path] = g
	}
	return t
}

// readoutPage words one row out of a table, whether that table was just
// read or is the one the page already had. It says the row is not there
// rather than that it has gone: of a table just read that is a row that
// ended, but of the table already read it may only be a row that
// started since, and the reading on its way will have it.
func readoutPage(pid int, t readoutTable) (readoutReport, bool) {
	s, ok := subjectOf(pid, t.projects, t.procs)
	if !ok {
		return readoutReport{}, false
	}
	if t.inside {
		s.pane, s.inside = t.panes[s.entry.tty], true
	}
	if s.entry.kind == kindContact {
		s.sess, s.carried = t.sess[pid], t.carried[pid]
	}
	s.git = t.git[s.project.path]

	home, _ := os.UserHomeDir()
	return composeReadout(s, home, time.Now()), true
}

// subjectOf finds a pid among the projects and gathers what stands
// around it: the table's own record, its project, what runs it and what
// it runs.
func subjectOf(pid int, projects []project, procs []process) (readoutSubject, bool) {
	byPid := map[int]process{}
	for _, p := range procs {
		byPid[p.pid] = p
	}
	for _, pl := range projects {
		for i, e := range pl.entries {
			if e.pid != pid {
				continue
			}
			s := readoutSubject{entry: e, proc: byPid[pid], project: pl}
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
