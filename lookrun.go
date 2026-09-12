package main

import (
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// conn look <pid> — the look's own program, run in a pane of conn's
// server the way the hold is. It reads for itself rather than being
// told: the rail's reading is the rail's, the page wants more of it
// than a row carries, and a pane that reads its own subject keeps
// saying the truth while the rail is busy elsewhere.
//
// A key hands focus back to the rail, the way the hold does. There is
// nothing to do on the page — it is a reading — and the keys that work
// the watch all live on the rail, so the useful thing a keypress here
// can mean is "put me back where the keys are".
//
// With no pid it follows the rail's cursor, which is what i opens: the
// page is about whatever the cursor is on, so j and k read down the
// list with the page keeping up rather than leaving it on a row nobody
// is looking at any more. With a pid it stays on that pid, which is
// what `conn look 123` by hand is for.

// lookBeat is how often the page reads its subject again. The watch's
// own beat: the two are readings of the same table and there is no
// reason for one to be staler than the other.
//
// lookPoll is how often it asks where the cursor is, which is a read of
// a few bytes rather than the whole table and can afford to be quick.
// It has to be: nothing else stands between a key on the rail and the
// page changing, so the poll is the whole of the wait, and a wait long
// enough to see is a page that trails the cursor down the list.
const (
	lookBeat = 2 * time.Second
	lookPoll = 50 * time.Millisecond
)

type lookModel struct {
	srv           *server
	pid           int
	follow        bool   // the subject is the rail's cursor, not a pid given
	cursor        string // where the rail publishes it
	width, height int
	p             palette
	report        lookReport
	table         lookTable // what the page was last composed out of
	read          time.Time // when the subject was last read in full
}

// lookReadMsg carries a reading, and the pid it was of: several can be
// in flight at once when the cursor is moving, and one that lands after
// the subject has changed again is stale and dropped. The table it was
// made from is not dropped with it — that is a reading of the machine
// rather than of the row, and it is as good for one row as another.
type lookReadMsg struct {
	pid    int
	report lookReport
	table  lookTable
}

func runLook(srv *server, pid int, home string, p palette) error {
	m := lookModel{srv: srv, pid: pid, follow: pid == 0, cursor: cursorPath(home), p: p,
		report: lookReport{pid: pid}}
	_, err := tea.NewProgram(m, programOptions()...).Run()
	return err
}

func (m lookModel) Init() tea.Cmd {
	_, cmd := m.reading()
	return cmd
}

func (m lookModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case lookReadMsg:
		// A reading of a subject that has since moved on is no longer
		// about anything, and putting it up would be a page flicking
		// back to a row the cursor has left. The table is kept whatever
		// row it was read for: it is the machine, and the next row the
		// cursor lands on is in it too.
		m.table = msg.table
		if msg.pid == m.pid {
			m.report = msg.report
		}
	case lookTickMsg:
		// Where the cursor is, then whether that is news. A subject that
		// changed is read at once; one that has not is read on the beat,
		// so a page nobody is moving still keeps up with its row.
		if m.follow {
			if pid := askCursor(m.cursor); pid != 0 && pid != m.pid {
				m.pid = pid
				// The row is answered now, out of the table already in
				// hand, and the reading only replaces that answer with
				// a newer one. Reading the machine takes a tenth of a
				// second, which is long enough to see, and waiting it
				// out would leave the page on the row the cursor just
				// left — while the table read a moment ago has the new
				// row in it, as true as the rail's own list is.
				if r, ok := lookPage(pid, m.table); ok {
					m.report = r
				}
				return m.reading()
			}
		}
		if time.Since(m.read) >= lookBeat {
			return m.reading()
		}
		return m, m.tick()
	case tea.KeyPressMsg:
		if m.srv != nil {
			return m, func() tea.Msg { _ = m.srv.focusRail(); return nil }
		}
	}
	return m, nil
}

type lookTickMsg struct{}

func (m lookModel) tick() tea.Cmd {
	return tea.Tick(lookPoll, func(time.Time) tea.Msg { return lookTickMsg{} })
}

// reading gathers the subject and words it, off the loop: the process
// table, the tree the row sits in, what claude says of it, and what git
// says of its place are each a reading, and git is a process besides.
// It marks the model as having read, so the two go together and neither
// can be done without the other.
//
// The table the page holds goes with it, so what conn has already asked
// of a place or a conversation is not asked again from nothing.
func (m lookModel) reading() (lookModel, tea.Cmd) {
	m.read = time.Now()
	pid, srv, held := m.pid, m.srv, m.table
	return m, tea.Batch(
		func() tea.Msg {
			report, table := lookOf(pid, srv, held)
			return lookReadMsg{pid: pid, report: report, table: table}
		},
		m.tick(),
	)
}

// lookTable is what a page is composed out of: the process table and
// the places conn makes of it, what conn's server holds of panes, what
// claude says of its sessions — readings of the whole machine, the same
// for every row — and beside them what git and claude's transcripts
// have said of the rows read so far, which are askings about one row
// and are kept rather than thrown away with the page they were for.
//
// The page holds the last one, because the cursor moves faster than the
// machine can be read and every row the cursor can land on is already
// in the table that was read for the row it is leaving.
type lookTable struct {
	procs  []process
	places []place
	panes  map[string]pane
	inside bool // there was a server to ask about panes
	sess   map[int]sessionFile
	git    map[string]gitStanding // what git said of a place, by its path
	convo  map[int]conversation   // which conversation a row was carrying
}

// lookOf is the page for a pid as things stand and the table it was
// read from, or the page that says the row has gone. A pid of nothing
// is a page waiting on a cursor that has not said where it is yet.
func lookOf(pid int, srv *server, held lookTable) (lookReport, lookTable) {
	if pid == 0 {
		return lookReport{}, held
	}
	t := lookGather(pid, srv, held)
	r, ok := lookPage(pid, t)
	if !ok {
		return lookReport{pid: pid, gone: true}, t
	}
	return r, t
}

// lookGather reads the machine, and then asks what only the row's own
// place and conversation can answer. What it is told of those joins
// what it was told of the rows read before, since a list walked down
// and back up again is the same few places over and over and git is a
// process each time.
func lookGather(pid int, srv *server, held lookTable) lookTable {
	// Copied rather than written into, because the page goes on reading
	// the table it holds while this one is being made.
	t := lookTable{git: maps.Clone(held.git), convo: maps.Clone(held.convo)}
	if t.git == nil {
		t.git = map[string]gitStanding{}
	}
	if t.convo == nil {
		t.convo = map[int]conversation{}
	}

	uid := os.Getuid()
	procs, err := readProcesses(uid)
	if err != nil {
		return t
	}
	t.procs = procs
	home, _ := os.UserHomeDir()
	isProject := projectDirs(projectRoots(home))
	t.places = watch(procs, uid, placeRoots(isProject), isProject, agentStandings(procs))

	// What conn holds for the rows' terminals, when there is a server to
	// ask. Outside one there is nothing to say of panes.
	if srv != nil {
		if panes, err := srv.panes(); err == nil {
			t.panes, t.inside = panes, true
		}
	}
	t.sess = claudeSessions()

	s, ok := subjectOf(pid, t.places, t.procs)
	if !ok {
		return t
	}
	// Which conversation an agent is carrying — the session file names
	// it, and the transcript is where the branch and the last ask are.
	if s.entry.kind == kindAgent {
		if f := t.sess[pid]; f.SessionID != "" {
			c := conversation{ID: f.SessionID, Dir: s.entry.cwd}
			readConvoMeta(convoPath(s.entry.cwd, f.SessionID), &c)
			t.convo[pid] = c
		}
	}
	t.git[s.place.path] = readGit(s.place.path)
	return t
}

// lookPage words one row out of a table, whether that table was just
// read or is the one the page already had. It says the row is not there
// rather than that it has gone: of a table just read that is a row that
// ended, but of the table in hand it may only be a row that started
// since, and the reading on its way will have it.
func lookPage(pid int, t lookTable) (lookReport, bool) {
	s, ok := subjectOf(pid, t.places, t.procs)
	if !ok {
		return lookReport{}, false
	}
	if t.inside {
		s.pane, s.inside = t.panes[s.entry.tty], true
	}
	if s.entry.kind == kindAgent {
		s.sess, s.convo = t.sess[pid], t.convo[pid]
	}
	s.git = t.git[s.place.path]

	home, _ := os.UserHomeDir()
	return composeLook(s, home, time.Now()), true
}

// subjectOf finds a pid among the places and gathers what stands around
// it: the table's own record, its place, what runs it and what it runs.
func subjectOf(pid int, places []place, procs []process) (lookSubject, bool) {
	byPid := map[int]process{}
	for _, p := range procs {
		byPid[p.pid] = p
	}
	for _, pl := range places {
		for i, e := range pl.entries {
			if e.pid != pid {
				continue
			}
			s := lookSubject{entry: e, proc: byPid[pid], place: pl}
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
	return lookSubject{}, false
}

// convoPath is where claude files a conversation had in a directory.
func convoPath(dir, id string) string {
	return filepath.Join(claudeConfigDir(), "projects", encodePath(dir), id+".jsonl")
}

func (m lookModel) View() tea.View {
	rows := drawLook(m.report, max(m.width, 1), m.height, m.p)
	texts := make([]string, len(rows))
	for i, r := range rows {
		texts[i] = r.text
	}
	v := tea.NewView(strings.Join(texts, "\n"))
	v.AltScreen = true
	return v
}
