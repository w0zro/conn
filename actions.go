package main

import (
	"syscall"

	tea "charm.land/bubbletea/v2"
)

// What the panel asks of the server, each off the loop as a command:
// a process into the bay, a shell opened at a project, the bay opened
// beside the panel, and the small ones — zoom, width, detach — through
// serverCmd.

// openBay opens the bay beside the panel, with the hold in it.
func (m model) openBay() tea.Cmd {
	home, self := m.head.login.home, m.self
	return m.serverCmd(func() error { return m.srv.splitBay(home, self) })
}

// reviveBay puts a hold in a bay whose pane died, in its own shape.
func (m model) reviveBay() tea.Cmd {
	home, self := m.head.login.home, m.self
	return m.serverCmd(func() error { return m.srv.reviveBay(home, self) })
}

// reach puts a process in the bay, off the loop, and passes back the
// terminal that is in the bay once it is there.
func (m model) reach(target pane, tty string) tea.Cmd {
	srv := m.srv
	return func() tea.Msg {
		if srv.show(target) != nil {
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
	home, self, srv := m.head.login.home, m.self, m.srv
	return func() tea.Msg {
		if srv.showReadout(home, self) != nil {
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
		sh, err := srv.open(dir)
		if err != nil {
			return nil
		}
		return openedMsg{shell: sh}
	}
}

// contactProgram is the contact conn starts. Claude is the only kind
// conn starts for now, so a is its key everywhere a shell's is s.
const contactProgram = "claude"

// startContact opens a contact at a project, off the loop, the way
// openShell opens a shell there.
func (m model) startContact(dir string) tea.Cmd {
	srv := m.srv
	return func() tea.Msg {
		sh, err := srv.openCmd(dir, contactCommand(srv.socket))
		if err != nil {
			return nil
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
	roots, cfgErr := projectRoots(m.head.login.home)
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
		return sessionsMsg{dirs: dirs, sessions: claudeSuspended(dirs, projects)}
	}
}

// openResumed opens a shell that picks a suspended session back
// up, off the loop, the way startContact opens a fresh one.
func (m model) openResumed(dir, id string) tea.Cmd {
	srv := m.srv
	return func() tea.Msg {
		sh, err := srv.openCmd(dir, resumeCommand(srv.socket, id))
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
func hasPid(projects []project, pid int) bool {
	for _, pl := range projects {
		for _, e := range pl.entries {
			if e.pid == pid {
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
func (m model) watchContainer(e entry) tea.Cmd {
	srv, dir, id := m.srv, e.cwd, e.container
	cmd := shellQuote(dockerPath) + " logs --tail 2000 --follow " + shellQuote(id) + " 2>&1; " + holdOpen
	return func() tea.Msg {
		sh, err := srv.openWatching(dir, cmd, id)
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
func (m model) shellInContainer(e entry) tea.Cmd {
	srv, dir, id := m.srv, e.cwd, e.container
	cmd := shellQuote(dockerPath) + " exec -it " + shellQuote(id) + " sh -c " +
		shellQuote(pickShell) + " 2>&1 || " + holdOpen
	return func() tea.Msg {
		sh, err := srv.openShellIn(dir, cmd, id)
		if err != nil {
			return nil
		}
		return openedMsg{shell: sh}
	}
}

// holdOpen keeps a pane standing after what it was opened for has
// finished. cat with nothing to read waits on the terminal for as long
// as the pane is there, which is exactly as long as wanted: the operator
// leaves by going somewhere else, and the pane goes when its work is
// replaced in the workspace.
//
// It is for a pane with something left to read in it — a log that ended,
// an error docker printed. A pane whose work is over and has left
// nothing behind should go, and a shell is that.
const holdOpen = "exec cat"

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
func (m model) stopContainer(id, service string) tea.Cmd {
	feed := m.dockerFeed
	return func() tea.Msg {
		_, _ = dockerSays(dockerStopWait, "stop", id)
		// The feed hears of it from docker's own events, but a stop
		// asked for here is worth asking about at once rather than
		// waiting to be told.
		feed.ask()
		return killedMsg{command: service, pid: 0}
	}
}

// openHelp puts the manual in the workspace. The page is written out of
// the binary first, so what is shown is the manual this conn was built
// with rather than whatever is installed on the machine.
func (m model) openHelp() tea.Cmd {
	home, self, srv := m.head.login.home, m.self, m.srv
	return func() tea.Msg {
		if srv.showHelp(home, self) != nil {
			return nil
		}
		return helpMsg{on: true}
	}
}
