package main

import (
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/w0zro/conn/internal/room"
	"github.com/w0zro/conn/internal/work"

	"github.com/w0zro/conn/internal/station"

	"github.com/w0zro/conn/internal/tmux"

	"github.com/w0zro/conn/internal/config"

	tea "charm.land/bubbletea/v2"
)

// What the panel asks of the server, each off the loop as a command:
// a process into the bay, a shell opened at a project, the bay opened
// beside the panel, and the small ones — zoom, width, detach — through
// serverCmd.

// openBay opens the bay beside the panel, with the hold in it.
func (m model) openBay() tea.Cmd {
	home, self := m.head.Login.Home, m.self
	return m.serverCmd(func() error { return m.srv.SplitBay(home, self) })
}

// reviveBay puts a hold in a bay whose pane died, in its own shape.
func (m model) reviveBay() tea.Cmd {
	home, self := m.head.Login.Home, m.self
	return m.serverCmd(func() error { return m.srv.ReviveBay(home, self) })
}

// reach puts a process in the bay, off the loop, and passes back the
// terminal that is in the bay once it is there.
func (m model) reach(target room.Pane, tty string) tea.Cmd {
	srv := m.srv
	return func() tea.Msg {
		if srv.Show(target) != nil {
			return nil
		}
		return reachedMsg{tty}
	}
}

// openReadout puts the readout in the bay, off the loop. It follows the
// panel's cursor from there, so this is asked once and not again for
// every row read: focus stays on the panel, and j and k carry the page
// along with them.
func (m model) openReadout() tea.Cmd {
	home, self, srv := m.head.Login.Home, m.self, m.srv
	return func() tea.Msg {
		if srv.ShowReadout(home, self) != nil {
			return nil
		}
		return readoutMsg{on: true}
	}
}

// openShell opens a shell at a project, off the loop, and passes
// back what tmux said of it.
func (m model) openShell(dir string) tea.Cmd {
	srv := m.srv
	return func() tea.Msg {
		sh, err := srv.Open(dir)
		if err != nil {
			return noticeMsg{"the shell could not be opened: " + err.Error()}
		}
		return openedMsg{shell: sh}
	}
}

// raise brings one declaration up: the answer to enter on a down row,
// in the bay with the keys in it, and to u, parked, the keys left on
// the panel. replace is the pane holding the last run of it, where one
// stands.
func (m model) raise(path string, d work.Declaration, replace string, enter bool) tea.Cmd {
	srv := m.srv
	return func() tea.Msg {
		sh, err := srv.RaiseDeclared(d.At(path), d.Command, d.Name, work.MarkDeclared(path, d.Name), replace, enter)
		if err != nil {
			return nil
		}
		if !enter {
			return raisedMsg{shells: []room.Shell{sh}}
		}
		return openedMsg{shell: sh}
	}
}

// raiseAll brings up everything a project declares that is not up, in
// the order of the file, parked: the keys stay on the panel and the
// reading lists the rows as they come. The file is read fresh, off the
// loop, so what is raised is what it says now. up is passed over, and
// held is replaced, by mark.
func (m model) raiseAll(path string, up map[string]bool, held map[string]string) tea.Cmd {
	srv := m.srv
	return func() tea.Msg {
		list, err := work.ReadDeclared(path)
		if err != nil {
			return nil
		}
		var shells []room.Shell
		for _, d := range list {
			mark := work.MarkDeclared(path, d.Name)
			if up[mark] {
				continue
			}
			// A brew service is started by brew, not in a pane.
			if formula, ok := work.BrewArgs(d.Command); ok {
				_, _ = work.BrewSays(work.BrewWait, "services", "start", formula)
				continue
			}
			sh, err := srv.RaiseDeclared(d.At(path), d.Command, d.Name, mark, held[mark], false)
			if err != nil {
				continue
			}
			shells = append(shells, sh)
		}
		return raisedMsg{shells: shells}
	}
}

// interruptDeclared sends ctrl-c to a declared process's pane and,
// once the pane has recorded the end, takes the pane down, so the row
// goes straight to DOWN rather than standing ENDED for a second x: an
// end the operator asked for has nothing in it to read. A command that
// dies of the ctrl-c takes the shell with it and the pane closes on its
// own, which is the same end. The wait is bounded by closeWait; a
// process that does not answer keeps its pane, and the row goes on
// saying it is up.
func (m model) interruptDeclared(pane, command string) tea.Cmd {
	srv := m.srv
	return func() tea.Msg {
		if err := srv.Interrupt(pane); err == nil {
			deadline := time.Now().Add(closeWait)
			for time.Now().Before(deadline) {
				exit, err := srv.PaneExit(pane)
				if err != nil {
					break // the pane is gone, and so is the process
				}
				if exit != "" {
					_ = srv.ClosePane(pane)
					break
				}
				time.Sleep(closePoll)
			}
		}
		return killedMsg{command: command}
	}
}

// closeHeld takes down the pane a declared process ended in.
func (m model) closeHeld(id, name string) tea.Cmd {
	srv := m.srv
	return func() tea.Msg {
		_ = srv.ClosePane(id)
		return killedMsg{command: name}
	}
}

// declarationOf is the declaration a row stands for, from the file as
// last read.
func (m model) declarationOf(e work.Entry) (path string, d work.Declaration, ok bool) {
	path, name, ok := work.UnmarkDeclared(e.Declared)
	if !ok {
		return "", work.Declaration{}, false
	}
	for _, d := range m.declared[path].List {
		if d.Name == name {
			return path, d, true
		}
	}
	return "", work.Declaration{}, false
}

// startContact opens a contact at a project, off the loop, the way
// openShell opens a shell there.
func (m model) startContact(dir string) tea.Cmd {
	srv := m.srv
	return func() tea.Msg {
		sh, err := srv.OpenCmd(dir, work.ContactCommand(srv.Socket))
		if err != nil {
			return noticeMsg{"the contact could not be opened: " + err.Error()}
		}
		return openedMsg{shell: sh}
	}
}

// scanProjects walks the roots off the loop; what it found, or why it
// could not, comes back as a message. A config file that will not parse
// is said instead of the walk, and ahead of it: the roots being walked
// are then not the ones the operator asked for, and that is the first
// thing to know.
func (m model) scanProjects() tea.Cmd {
	roots, cfgErr := config.Roots(m.head.Login.Home)
	return func() tea.Msg {
		if cfgErr != nil {
			return projectsMsg{err: "THE CONFIG COULD NOT BE READ: " + cfgErr.Error()}
		}
		ps, err := findProjects(roots)
		if err != nil {
			return projectsMsg{err: "THE ROOTS COULD NOT BE WALKED: " + err.Error()}
		}
		return projectsMsg{projects: ps}
	}
}

// scanSessions reads a project's suspended sessions off the loop, the
// process table as it stood when the sessions view opened, so a
// leftover session file cannot be mistaken for one still going.
func (m model) scanSessions(dirs []string) tea.Cmd {
	projects := m.projects
	return func() tea.Msg {
		return sessionsMsg{dirs: dirs, sessions: work.ClaudeSuspended(dirs, projects)}
	}
}

// openResumed opens a shell that picks a suspended session back
// up, off the loop, the way startContact opens a fresh one.
func (m model) openResumed(dir, id string) tea.Cmd {
	srv := m.srv
	return func() tea.Msg {
		sh, err := srv.OpenCmd(dir, work.ResumeCommand(srv.Socket, id))
		if err != nil {
			return nil
		}
		return openedMsg{shell: sh}
	}
}

// killEntry signals a process, off the loop.
func (m model) killEntry(pid int, command string, sig syscall.Signal) tea.Cmd {
	return func() tea.Msg {
		_ = signal(pid, sig)
		return killedMsg{command: command, pid: pid, sig: sig}
	}
}

// hasPid says whether a process is among what was read.
func hasPid(projects []work.Project, pid int) bool {
	for _, pl := range projects {
		for _, e := range pl.Entries {
			if e.PID == pid {
				return true
			}
		}
	}
	return false
}

// serverCmd runs a server action off the loop. What the server did is
// on the window for the operator to see, and what it did not do has no
// row of its own to be said on.
func (m model) serverCmd(act func() error) tea.Cmd {
	return func() tea.Msg {
		_ = act()
		return nil
	}
}

// watchContainer puts a container's output in the workspace: docker's own
// record of what it wrote, followed while it runs.
//
// A container has no shell to enter and no terminal to take over. What it
// has is stdout, which docker keeps whether the container is running or
// long dead — the worker's last words survive it, and those are the ones
// worth having. So the pane conn opens is a reader, and the row it
// belongs to is reached and left like any other from there.
//
// The reading is held open after the log ends. docker logs --follow
// blocks only while there is something to follow: on a container that
// has stopped it prints what there is and returns at once, and the pane
// would be gone before it could be put in the workspace — which is what
// happened the first time this was tried. A container that dies while
// you are watching it ends the same way. So the log is followed and then
// the pane waits, and the last words stay up to be read.
func (m model) watchContainer(e work.Entry) tea.Cmd {
	srv, dir, id := m.srv, e.Cwd, e.Container
	cmd := tmux.ShellQuote(work.DockerPath) + " logs --tail 2000 --follow " + tmux.ShellQuote(id) + " 2>&1; " + tmux.HoldOpen
	return func() tea.Msg {
		sh, err := srv.OpenWatching(dir, cmd, id)
		if err != nil {
			return nil
		}
		return openedMsg{shell: sh}
	}
}

// shellInContainer opens a shell inside a container, which is what s
// means on a row that is one: a shell here, where here is the container
// rather than the directory it was started for. bash is preferred and sh
// is the fallback, since the smaller images carry only the one.
//
// A container that is not running has no shell to give, and docker says
// so in a line. The pane is held open for that line the way it is held
// for a log that has ended: an error that flashes past is an error
// nobody read.
//
// It is held on the failure only. A log ends and there is still the log
// to read, but a shell you typed exit in has nothing left to show, and
// holding that pane open left you sitting in a cat that echoed what you
// typed and looked for all the world like a shell that had hung.
func (m model) shellInContainer(e work.Entry) tea.Cmd {
	srv, dir, id := m.srv, e.Cwd, e.Container
	cmd := tmux.ShellQuote(work.DockerPath) + " exec -it " + tmux.ShellQuote(id) + " sh -c " +
		tmux.ShellQuote(pickShell) + " 2>&1 || " + tmux.HoldOpen
	return func() tea.Msg {
		sh, err := srv.OpenShellIn(dir, cmd, id)
		if err != nil {
			return nil
		}
		return openedMsg{shell: sh}
	}
}

// openClient is S on a row that is a program conn knows: a session with
// the server by its own client, psql for postgres. In a container the
// client runs inside it by docker exec, as the user the image was
// given. On this machine it connects to the port the row listens on,
// and is found on the path, or under the formula's own prefix where
// brew keeps it out of the path, as it does postgresql. The pane holds
// on a failure so what went wrong can be read, as a shell in a
// container does; a session ended by the operator takes its pane with
// it, as a shell does.
func (m model) openClient(e work.Entry, p *knownProgram, dir string) tea.Cmd {
	srv := m.srv
	if dir == "" {
		dir = e.Cwd
	}
	if e.Container != "" {
		id := e.Container
		return func() tea.Msg {
			user := containerEnv(id, p.userEnv)
			if user == "" {
				user = p.user
			}
			cmd := tmux.ShellQuote(work.DockerPath) + " exec -it " + tmux.ShellQuote(id) + " " + p.inContainer(user) + " 2>&1 || " + tmux.HoldOpen
			sh, err := srv.OpenShellIn(dir, cmd, id)
			if err != nil {
				return nil
			}
			return openedMsg{shell: sh}
		}
	}
	if len(e.Ports) == 0 {
		return nil
	}
	port, formula := e.Ports[0], e.Brew
	return func() tea.Msg {
		client := station.LookPath(p.client)
		if client == "" && formula != "" {
			if out, err := work.BrewSays(work.BrewWait, "--prefix", formula); err == nil {
				if c := filepath.Join(strings.TrimSpace(string(out)), "bin", p.client); station.LookPath(c) != "" {
					client = c
				}
			}
		}
		if client == "" {
			return noticeMsg{p.client + " was not found on the path"}
		}
		cmd := tmux.ShellQuote(client) + " " + p.args(port) + " 2>&1 || " + tmux.HoldOpen
		sh, err := srv.OpenCmd(dir, cmd)
		if err != nil {
			return noticeMsg{"the session could not be opened: " + err.Error()}
		}
		return openedMsg{shell: sh}
	}
}

// containerEnv is one variable of a container's environment, as docker
// inspect reports it, or nothing.
func containerEnv(id, name string) string {
	if name == "" || work.DockerPath == "" {
		return ""
	}
	out, err := work.DockerSays(work.DockerWait, "inspect", "-f", "{{range .Config.Env}}{{println .}}{{end}}", id)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if v, ok := strings.CutPrefix(line, name+"="); ok {
			return v
		}
	}
	return ""
}

// pickShell is run inside the container to choose its shell. bash is
// tested for rather than tried, because exec replaces the shell and a
// failed exec ends it: exec bash || exec sh never reaches the fallback,
// and on an image with no bash — which is most of the small ones — it
// exits 127 rather than giving you the sh that was there all along.
const pickShell = "command -v bash >/dev/null 2>&1 && exec bash || exec sh"

// stopContainer asks docker to let a container go, off the loop. It is
// docker's stop and not a signal: there is no process on this machine to
// send one to, and docker asks the container to end and waits before
// insisting, which is what a service expects of a shutdown.
// watchBrew opens a pane following a brew service's log, marked as the
// service's, so the pane is the row's terminal and the next enter goes
// back into it. A service with no log to follow opens nothing.
func (m model) watchBrew(e work.Entry) tea.Cmd {
	svc := m.brewAt(e.Brew)
	if svc == nil || svc.Log == "" {
		return nil
	}
	srv, dir, formula, log := m.srv, e.Cwd, e.Brew, svc.Log
	cmd := "tail -n 2000 -f " + tmux.ShellQuote(log) + " 2>&1; " + tmux.HoldOpen
	return func() tea.Msg {
		sh, err := srv.OpenWatching(dir, cmd, work.BrewMark(formula))
		if err != nil {
			return nil
		}
		return openedMsg{shell: sh}
	}
}

// startBrew asks brew to start a service, and asks it how things
// stand once it has answered, so the row is ACTIVE when the start is,
// rather than on the next beat.
func (m model) startBrew(formula string) tea.Cmd {
	return func() tea.Msg {
		_, _ = work.BrewSays(work.BrewWait, "services", "start", formula)
		return work.ReadBrew()
	}
}

// stopBrew asks brew to stop a service, the way x on a container asks
// docker: there is no process here conn would signal itself.
func (m model) stopBrew(formula, name string) tea.Cmd {
	return func() tea.Msg {
		_, _ = work.BrewSays(work.BrewWait, "services", "stop", formula)
		return killedMsg{command: name, pid: 0}
	}
}

func (m model) stopContainer(id, service string) tea.Cmd {
	feed := m.dockerFeed
	return func() tea.Msg {
		_, _ = work.DockerSays(work.DockerStopWait, "stop", id)
		// The feed hears of it from docker's own events, but a stop
		// asked for here is worth asking about at once rather than
		// waiting to be told.
		feed.Ask()
		return killedMsg{command: service, pid: 0}
	}
}

// openHelp puts the manual in the workspace. The page is written out of
// the binary first, so what is shown is the manual this conn was built
// with rather than whatever is installed on the machine.
func (m model) openHelp() tea.Cmd {
	home, self, srv := m.head.Login.Home, m.self, m.srv
	return func() tea.Msg {
		if srv.ShowHelp(home, self) != nil {
			return nil
		}
		return detourMsg{toManual}
	}
}

// openTheSettings puts the settings in the workspace, the way the
// manual goes there.
func (m model) openTheSettings() tea.Cmd {
	home, self, srv := m.head.Login.Home, m.self, m.srv
	return func() tea.Msg {
		if srv.ShowSettings(home, self) != nil {
			return nil
		}
		return detourMsg{toSettings}
	}
}
