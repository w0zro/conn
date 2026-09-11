package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// What the rail asks of the server, each off the loop as a command:
// a process into the slot, a shell opened at a place, the slot opened
// beside the rail, and the small ones — zoom, width, detach — through
// serverCmd. What goes wrong is said on the bottom row.

// openSlot opens the slot beside the rail, with the hold in it.
func (m model) openSlot() tea.Cmd {
	home, self := m.head.session.home, m.self
	return m.serverCmd(func() error { return m.srv.splitSlot(home, self) }, "")
}

// reach puts a process in the slot, off the loop, and hands back the
// terminal that is in the slot once it is there.
func (m model) reach(target pane, tty string) tea.Cmd {
	srv := m.srv
	return func() tea.Msg {
		if err := srv.show(target); err != nil {
			return noteMsg{strings.ToUpper(err.Error())}
		}
		return reachedMsg{tty}
	}
}

// openShell opens a shell at a place, off the loop, and hands back what
// tmux said of it.
func (m model) openShell(dir string) tea.Cmd {
	srv := m.srv
	return func() tea.Msg {
		sh, err := srv.open(dir)
		if err != nil {
			return noteMsg{strings.ToUpper(err.Error())}
		}
		return openedMsg{shell: sh}
	}
}

// agentCommand starts an agent. Claude is the only kind conn starts for
// now, so a is its key everywhere a shell's is s.
const agentCommand = "claude"

// openAgent opens an agent at a place, off the loop, the way openShell
// opens a shell there.
func (m model) openAgent(dir string) tea.Cmd {
	srv := m.srv
	return func() tea.Msg {
		sh, err := srv.openCmd(dir, agentCommand)
		if err != nil {
			return noteMsg{strings.ToUpper(err.Error())}
		}
		return openedMsg{shell: sh}
	}
}

// scanProjects walks the roots off the loop; what it found, or why it
// could not, comes back as a message.
func (m model) scanProjects() tea.Cmd {
	roots := projectRoots(m.head.session.home)
	return func() tea.Msg {
		ps, err := findProjects(roots)
		if err != nil {
			return projectsMsg{err: "THE ROOTS COULD NOT BE WALKED: " + err.Error()}
		}
		return projectsMsg{projects: ps}
	}
}

// hasPid says whether a process is among what was read.
func hasPid(places []place, pid int) bool {
	for _, pl := range places {
		for _, e := range pl.entries {
			if e.pid == pid {
				return true
			}
		}
	}
	return false
}

// serverCmd runs a server action off the loop; what goes wrong is said
// on the bottom row.
func (m model) serverCmd(act func() error, done string) tea.Cmd {
	return func() tea.Msg {
		if err := act(); err != nil {
			return noteMsg{strings.ToUpper(err.Error())}
		}
		return noteMsg{done}
	}
}
