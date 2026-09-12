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

// lookBeat is how often the page reads again. The watch's own beat:
// the two are readings of the same table and there is no reason for one
// to be staler than the other.
const lookBeat = 2 * time.Second

type lookModel struct {
	srv           *server
	pid           int
	width, height int
	p             palette
	report        lookReport
}

type lookReadMsg struct{ report lookReport }

func runLook(srv *server, pid int, p palette) error {
	m := lookModel{srv: srv, pid: pid, p: p, report: lookReport{pid: pid}}
	_, err := tea.NewProgram(m, programOptions()...).Run()
	return err
}

func (m lookModel) Init() tea.Cmd { return m.read() }

func (m lookModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case lookReadMsg:
		m.report = msg.report
		return m, tea.Tick(lookBeat, func(time.Time) tea.Msg { return lookTickMsg{} })
	case lookTickMsg:
		return m, m.read()
	case tea.KeyPressMsg:
		if m.srv != nil {
			return m, func() tea.Msg { _ = m.srv.focusRail(); return nil }
		}
	}
	return m, nil
}

type lookTickMsg struct{}

// read gathers the subject and words it, off the loop: the process
// table, the tree the row sits in, what claude says of it, and what git
// says of its place are each a reading, and git is a process besides.
func (m lookModel) read() tea.Cmd {
	pid, srv := m.pid, m.srv
	return func() tea.Msg {
		return lookReadMsg{report: lookOf(pid, srv)}
	}
}

// lookOf is the page for a pid as things stand, or the page that says
// the row has gone.
func lookOf(pid int, srv *server) lookReport {
	uid := os.Getuid()
	procs, err := readProcesses(uid)
	if err != nil {
		return lookReport{pid: pid, gone: true}
	}
	places := watch(procs, uid, placeRoots(), agentStandings(procs))
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
