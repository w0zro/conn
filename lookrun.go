package main

import (
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
// It has to be: j held down moves the cursor faster than the beat, and
// a page that lagged a second behind the cursor would be worse than one
// that did not follow at all.
const (
	lookBeat = 2 * time.Second
	lookPoll = 150 * time.Millisecond
)

type lookModel struct {
	srv           *server
	pid           int
	follow        bool   // the subject is the rail's cursor, not a pid given
	cursor        string // where the rail publishes it
	width, height int
	p             palette
	report        lookReport
	read          time.Time // when the subject was last read in full
}

// lookReadMsg carries a reading, and the pid it was of: several can be
// in flight at once when the cursor is moving, and one that lands after
// the subject has changed again is stale and dropped.
type lookReadMsg struct {
	pid    int
	report lookReport
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
		// back to a row the cursor has left.
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
func (m lookModel) reading() (lookModel, tea.Cmd) {
	m.read = time.Now()
	pid, srv := m.pid, m.srv
	return m, tea.Batch(
		func() tea.Msg { return lookReadMsg{pid: pid, report: lookOf(pid, srv)} },
		m.tick(),
	)
}

// lookOf is the page for a pid as things stand, or the page that says
// the row has gone. A pid of nothing is a page waiting on a cursor that
// has not said where it is yet.
func lookOf(pid int, srv *server) lookReport {
	if pid == 0 {
		return lookReport{}
	}
	uid := os.Getuid()
	procs, err := readProcesses(uid)
	if err != nil {
		return lookReport{pid: pid, gone: true}
	}
	places := watch(procs, uid, placeRoots(), projectDirs(), agentStandings(procs))
	s, ok := subjectOf(pid, places, procs)
	if !ok {
		return lookReport{pid: pid, gone: true}
	}

	// What conn holds for the row's terminal, when there is a server to
	// ask. Outside one there is nothing to say of panes.
	if srv != nil {
		if panes, err := srv.panes(); err == nil {
			s.pane, s.inside = panes[s.entry.tty], true
		}
	}
	// What claude says of itself, and which conversation that is — the
	// session file names it, and the transcript is where the branch and
	// the last ask are.
	if s.entry.kind == kindAgent {
		s.sess = claudeSessions()[pid]
		if s.sess.SessionID != "" {
			c := conversation{ID: s.sess.SessionID, Dir: s.entry.cwd}
			readConvoMeta(convoPath(s.entry.cwd, s.sess.SessionID), &c)
			s.convo = c
		}
	}
	s.git = readGit(s.place.path)

	home, _ := os.UserHomeDir()
	return composeLook(s, home, time.Now())
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
