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

// reach puts a process in the bay, off the loop, and processes back the
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

// openShell opens a shell at a project, off the loop, and processes
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
