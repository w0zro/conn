package main

import (
	"syscall"

	tea "charm.land/bubbletea/v2"
)

// What the rail asks of the server, each off the loop as a command:
// a process into the slot, a shell opened at a place, the slot opened
// beside the rail, and the small ones — zoom, width, detach — through
// serverCmd.

// openSlot opens the slot beside the rail, with the hold in it.
func (m model) openBay() tea.Cmd {
	home, self := m.head.login.home, m.self
	return m.serverCmd(func() error { return m.srv.splitBay(home, self) })
}

// reviveSlot puts a hold in a slot whose pane died, in its own shape.
func (m model) reviveBay() tea.Cmd {
	home, self := m.head.login.home, m.self
	return m.serverCmd(func() error { return m.srv.reviveBay(home, self) })
}

// reach puts a process in the slot, off the loop, and hands back the
// terminal that is in the slot once it is there.
func (m model) reach(target pane, tty string) tea.Cmd {
	srv := m.srv
	return func() tea.Msg {
		if srv.show(target) != nil {
			return nil
		}
		return reachedMsg{tty}
	}
}

// openLook puts the look in the slot, off the loop. It follows the
// rail's cursor from there, so this is asked once and not again for
// every row read: focus stays on the rail, and j and k carry the page
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

// closeLook takes the page out of the slot and leaves a hold in its
// place, which is what an empty slot is.
func (m model) closeReadout() tea.Cmd {
	home, self, srv := m.head.login.home, m.self, m.srv
	return func() tea.Msg {
		if srv.hideReadout(home, self) != nil {
			return nil
		}
		return readoutMsg{on: false}
	}
}

// openShell opens a shell at a place, off the loop, and hands back what
// tmux said of it.
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

// aiProgram is the AI conn starts. Claude is the only kind conn
// starts for now, so a is its key everywhere a shell's is s.
const contactProgram = "claude"

// startAI opens an AI at a place, off the loop, the way openShell
// opens a shell there.
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
// could not, comes back as a message.
func (m model) scanProjects() tea.Cmd {
	roots := projectRoots(m.head.login.home)
	return func() tea.Msg {
		ps, err := findProjects(roots)
		if err != nil {
			return projectsMsg{err: "THE ROOTS COULD NOT BE WALKED: " + err.Error()}
		}
		return projectsMsg{projects: ps}
	}
}

// scanConvos reads a place's suspended conversations off the loop, the
// process table as it stood when the picker opened, so a leftover
// session file cannot be mistaken for one still going.
func (m model) scanSessions(dirs []string) tea.Cmd {
	projects := m.projects
	return func() tea.Msg {
		return sessionsMsg{dirs: dirs, sessions: claudeSuspended(dirs, projects)}
	}
}

// openResumed opens a shell that picks a suspended conversation back
// up, off the loop, the way startAI opens a fresh one.
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
