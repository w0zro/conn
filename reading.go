package main

import (
	"maps"
	"time"

	"github.com/w0zro/conn/internal/room"
	"github.com/w0zro/conn/internal/wire"
	"github.com/w0zro/conn/internal/work"
	"github.com/w0zro/conn/internal/work/brew"
	"github.com/w0zro/conn/internal/work/claude"
	"github.com/w0zro/conn/internal/work/declared"
	"github.com/w0zro/conn/internal/work/docker"
	"github.com/w0zro/conn/internal/work/procs"

	"github.com/w0zro/conn/internal/config"

	tea "charm.land/bubbletea/v2"
)

// readProcesses reads the process table, and in the server its panes
// and the bay, and composes the processes view off them. It reads and
// does nothing else; what the reading calls for is decided when it
// comes back.
func (m model) readProcesses() tea.Cmd {
	gen, uid := m.processesGen, m.uid
	home, configured := m.head.Login.Home, m.roots.configured
	containers, brews := m.containers, m.brews
	files, full := m.declared, m.full
	was := m.trace
	var srv *room.Server
	if m.inside {
		srv = m.srv
	}
	return func() tea.Msg {
		table, err := procs.Read(uid)
		if err != nil {
			return processesMsg{err: "THE PROCESS TABLE COULD NOT BE READ: " + err.Error(), gen: gen}
		}
		// The roots as the file names them now. The operator edits the
		// file from inside conn, and the list walks it fresh; the
		// reading names projects by the same file, and takes it as it
		// now stands rather than as it stood when conn came up. A file
		// that will not parse changes nothing: the list says so, and
		// conn is not going to name projects by a guess at what it was
		// about to say.
		//
		// And which directories are projects, asked again. The answers
		// are remembered for this reading and no longer: a directory
		// becomes a project when git init runs in it, and a process
		// already there when a remembered answer said otherwise was
		// filed under the folder above for as long as conn ran. The
		// reading's rooting goes back with it, so the view names its
		// rows with the answers they were filed by, and no two readings
		// share a memory across the loop.
		if home != "" {
			if now, err := config.Roots(home); err == nil {
				configured = now
			}
		}
		rooting := rootOn(configured)
		roots, isProject := rooting.rootOf, rooting.isProject
		// How each process stands past what the table says: anything is
		// working by the processor time it spent since the last reading,
		// which is why that reading is kept, and a contact answers for
		// itself instead - working, or waiting on you.
		now, nowAt := work.CpuOf(table), time.Now()
		how := map[int]work.Standing{}
		for pid := range work.CpuWorking(was.cpu, was.at, table, nowAt) {
			how[pid] = work.Standing{Working: true}
		}
		maps.Copy(how, claude.Statuses(table))
		// The panes come first, because a pane conn opened to watch a
		// container is two things to the reading at once: the terminal
		// that container's row will stand on, and a process that must
		// not stand for itself. A docker logs beside the service it is
		// showing would be the same thing listed twice.
		var panes map[string]room.Pane
		if srv != nil {
			panes, _ = srv.Panes()
		}
		// A pane reading a service stands in for that service's
		// terminal and is kept off the view, since a docker logs listed
		// beside the service it is showing is the same thing twice. A
		// pane holding a shell inside a container is neither: it is the
		// operator's own work, it is listed, and it is filed under the
		// service it is inside rather than taking the slot the service's
		// terminal is in.
		paneOf, shellIn, watching := map[string]string{}, map[string]string{}, map[string]bool{}
		for tty, p := range panes {
			if p.Container != "" {
				paneOf[p.Container], watching[tty] = tty, true
			}
			if p.ShellIn != "" {
				shellIn[tty] = p.ShellIn
			}
		}
		// And the panel's own terminal, where nothing of the operator's
		// runs: conn does, and what conn asks.
		panelTTY := ""
		if srv != nil {
			for tty, p := range panes {
				if p.ID == srv.Panel() {
					panelTTY = tty
				}
			}
		}
		table = work.WithoutConnsOwn(table, watching, panelTTY)
		projects := work.ProjectsFrom(table, uid, roots, isProject, how)
		records := recordsOf(table, projects)
		// And what docker is holding up, which the table cannot show: a
		// container is not a process of this machine, and compose says
		// where each belongs by the directory it was started for. What
		// docker last said is already here — the feed brings it as it
		// happens — so this costs the reading nothing and waits on no
		// daemon.
		projects = docker.Attach(projects, containers, roots, paneOf, shellIn)
		// And what the projects declare should be working them, which
		// the table has no word for until it is: a stat per project,
		// and a read where a file changed.
		files = declared.Refresh(files, declared.Paths(projects, files, isProject))
		projects = declared.Attach(projects, files, declaredPanes(panes))
		// And the services brew holds up for them, as brew last said,
		// each with the sockets of the process running it, which the
		// table has and files nowhere.
		if brew.Declared(files) {
			sockets := map[int][]work.Socket{}
			for _, p := range table {
				if len(p.Sockets) > 0 {
					sockets[p.PID] = p.Sockets
				}
			}
			projects = brew.Attach(projects, files, brews, sockets, paneOf)
		}
		// And out go the projects nothing is up in, whose every row is
		// a declaration of what is not running; see worked.
		projects = worked(projects)
		// And which of them have lost the listener they had, which is a
		// fault and so is worded before the rows are dated: a row that
		// has come to say CLOSED came to say it now.
		serves := work.MarkClosed(projects, was.serves)
		msg := processesMsg{reading: reading{projects: projects, tree: projects, panes: panes, records: records, declared: files}, gen: gen,
			trace: &trace{cpu: now, at: nowAt, stood: work.SinceSeen(projects, was.stood, was.at, nowAt),
				acts: claude.Activities(projects, was.acts), serves: serves},
			rooted: &rooting}
		if !full {
			msg.projects = fold(projects)
		}
		if srv != nil {
			if in, ok, err := srv.Bay(); err == nil && !ok {
				msg.noBay = true
			} else if ok {
				msg.bay, msg.bayDead, msg.bayReadout = in.TTY, in.Dead, in.Readout
				switch {
				case in.Help:
					msg.bayDetour = toManual
				case in.Settings:
					msg.bayDetour = toSettings
				}
				msg.bayActive = in.Active
			}
		}
		return msg
	}
}

// processesTick is when the processes view reads again: soon while it
// waits on a shell conn opened, and at its own pace otherwise.
func (m model) processesTick() tea.Cmd {
	gen, every := m.processesGen, processesEvery
	if m.awaited.pid != 0 {
		every = processesSoon
	}
	return tea.Tick(every, func(time.Time) tea.Msg { return processesTickMsg{gen} })
}

// A trace is what a reading leaves for the next one to read against.
type trace struct {
	// The processor time every process had used, and when that was
	// taken: a process is working by what it has spent since, not by
	// what it has spent altogether.
	cpu map[int]time.Duration
	at  time.Time
	// Each row as it stood, and since when: what the next reading dates
	// a row's status against.
	stood map[int]work.Stood
	// Each contact's transcript as it was read for its activity.
	acts map[string]claude.ActivitySeen
	// Each row seen listening, and how many readings it has had nothing
	// open since: what says a listener has gone.
	serves map[int]work.ServingSeen
}

// landed is a reading come back: taken onto the model, and then what
// the view that is up does with one.
func (m model) landed(msg processesMsg) (model, tea.Cmd) {
	// The row the list's cursor was on, before the reading replaces the
	// rows it is an index into.
	wasRow, hadRow := m.atCursor()
	m = m.took(msg)
	// What changed since the last reading, to the log; see log.go. And
	// what every held pane has said since, read on the beat; see said.go.
	m, wrote := m.logging(msg)
	wrote = tea.Batch(wrote, m.listen())
	// The reading the console was waiting on: the processes view goes up
	// with its rows already in it, drawn at the panel's width, and the
	// bay opens beside a frame that is already the shape it will be.
	cmds := []tea.Cmd{wrote}
	if m.entering {
		m.entering, m.resumed, m.view = false, false, viewProcesses
		if m.inside {
			cmds = append(cmds, m.serverCmd(func() error { return m.srv.Narrow() }))
		}
	}
	switch m.view {
	case viewProcesses:
		var cmd tea.Cmd
		m, cmd = m.tended(msg)
		cmds = append(cmds, m.processesTick(), cmd)
	case viewProjects:
		// The list holds live processes too, so it is read for as long as
		// it is up: a row that says a process is there is a row enter is
		// about to go into, and one read minutes ago is a promise the
		// machine may not keep.
		cmds = append(cmds, m.processesTick())
		m.list.kept(m.projectRows(), wasRow, hadRow)
	case viewLog:
		// The log is read for as long as it is up, since it is the
		// readings that write it: a line lands on the view as the change
		// is seen, and the lines' rows are known to be there or gone.
		cmds = append(cmds, m.processesTick())
	case viewOutput:
		// The output view reads on too, and its panes with it: a process
		// that says something while the view is up is searched as it
		// says it.
		cmds = append(cmds, m.processesTick(), m.captureOutput())
	default:
		return m, tea.Batch(cmds...)
	}
	// And the readout is what the workspace holds while the keys are
	// here, about the row the cursor is on, so a view just come on, or
	// one that has just got its first row, has it without anybody
	// asking.
	m, cmd := m.keepingPage()
	return m, tea.Batch(append(cmds, cmd)...)
}

// took is the reading taken onto the model: its rows, the panes and
// the bay as it found them, and what it leaves for the next reading;
// and the cursor kept on its row across the change.
func (m model) took(msg processesMsg) model {
	// The reading was made on the roots it found the file naming; the
	// model goes onto them with it, so the rows and the roots they are
	// named by are never of two files.
	if msg.rooted != nil {
		m = m.rooted(*msg.rooted)
	}
	m.reading, m.processesErr = msg.reading, msg.err
	m.bay.read(msg.bay, msg.panes[msg.bay], msg.bayReadout)
	m.detour.to = msg.bayDetour
	// Where the keys are, by the server's own word. conn is told by the
	// terminal when they leave, and knows on its own when its reaching
	// sent them away, but a reading can land between the reaching and
	// the word of it: it then saw a bay with a process in it and no
	// readout, under a panel it still believed had the keys, and put the
	// readout back over the process. The reading carries tmux's answer,
	// so a bay that has the keys is a panel that does not, whatever conn
	// has yet been told.
	if msg.bayActive {
		m.focused = false
	}
	if msg.trace != nil {
		m.trace = *msg.trace
	}
	// The shell conn opened is the cursor's once the reading has it; one
	// that never comes is given up on when the wait is out.
	if pid, over := m.awaited.found(m.projects, time.Now()); over {
		if pid != 0 {
			m.cursor = pid
		}
		m.awaited = awaited{}
	}
	// follow's job is to keep hold of the row the operator was on while
	// the rows change under it. With the manual or the settings up there
	// is no such row, and following would hand one back every couple of
	// seconds: the cursor is cleared on purpose, and stays cleared until
	// the operator moves it themselves.
	if m.detour.to == noDetour {
		m = m.onRow(m.cursor, m.cursorAt)
	}
	return m
}

// tended is the bay put right after a reading, in the processes view,
// which is the view that tends it.
func (m model) tended(msg processesMsg) (model, tea.Cmd) {
	if !m.inside {
		return m, nil
	}
	switch {
	// A home without its bay gets one; the next reading finds it.
	case msg.noBay:
		return m, m.openBay()
	// A manual or the settings left behind: each says so as it goes, and
	// this is the same answer for one that ended without saying — killed
	// from outside, or gone while the keys were in the list and nobody
	// was tending the workspace.
	case msg.bayDead && msg.bayDetour != noDetour:
		return m.leftDetour(true)
	// A bay whose pane died stays the shape it was; only what is in it is
	// replaced, so the panel never has to give up its width and take it
	// back. What replaces it is the next process conn holds, from the
	// cursor down and round again: the operator was working in the bay,
	// and the work goes on in the one nearest to hand. Only with none to
	// reach does the bay hold a placard.
	case msg.bayDead:
		if e, ok := m.nextReachable(); ok {
			return m, m.reach(m.panes[e.TTY], e.TTY)
		}
		return m, m.reviveBay()
	}
	return m, nil
}

// recordsOf is the table's record behind each row, for the page: what
// the page reads of a process that the row does not carry, kept for
// the rows alone rather than for the whole table.
func recordsOf(table []work.Process, projects []work.Project) map[int]wire.Record {
	byPid := map[int]work.Process{}
	for _, p := range table {
		byPid[p.PID] = p
	}
	out := map[int]wire.Record{}
	for _, pl := range projects {
		for _, e := range pl.Entries {
			if p, ok := byPid[e.PID]; ok {
				out[e.PID] = wire.RecordOf(p)
			}
		}
	}
	return out
}

// declaredPanes is the panes conn opened for declarations, by the
// terminal each holds, as the rows are read against them.
func declaredPanes(panes map[string]room.Pane) map[string]declared.Pane {
	out := map[string]declared.Pane{}
	for tty, p := range panes {
		if p.Declared != "" {
			out[tty] = declared.Pane{ID: p.ID, TTY: p.TTY, Declared: p.Declared, Exit: p.Exit}
		}
	}
	return out
}

// A reading is what the panel reads of the machine on its beat, and
// holds until the next: the rows as the view shows them, folded or
// whole, the projects whole, the table's record behind each row for the
// page, the server's panes by the terminal each holds, and the
// projects' .conn files as found, for the next reading to stat against.
type reading struct {
	projects []work.Project
	tree     []work.Project
	records  map[int]wire.Record
	panes    map[string]room.Pane
	declared map[string]declared.File
}
