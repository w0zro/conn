package main

import (
	"syscall"

	"github.com/w0zro/conn/internal/work"

	"github.com/w0zro/conn/internal/tmux"

	tea "charm.land/bubbletea/v2"
)

// What enter, x and u do on the row under the cursor, each said once.
// The key does what these answer, and the bar offers what these
// answer, so the two cannot come apart: a row the bar offers a key on
// is a row the key works on, and one it does not is one the key leaves
// alone.
//
// A row is a process of this machine, a declaration from the project's
// .conn, a container, or a Homebrew service. A brew service is a
// declaration too, and is asked after as a service first: brew holds
// it, not a pane.

// enterOn is what enter does on a row, and the word the bar offers it
// by; no command where it does nothing. Everything enter does opens or
// shows a pane of the server, so off it enter does nothing.
func (m model) enterOn(e work.Entry) (string, tea.Cmd) {
	if !m.inside {
		return "", nil
	}
	// A row conn already holds a pane for is gone into; that is enter
	// everywhere. A container has no pane until one is opened for it,
	// and what there is to be in front of is what it has written, so
	// the first enter opens its output and the next goes back into the
	// pane holding it; a brew service up, its log, the same way.
	switch {
	case tmux.Reachable(m.panes[e.TTY]):
		return "Open", m.reach(m.panes[e.TTY], e.TTY)
	case e.Brew != "" && e.Status == work.StatusActive:
		if cmd := m.watchBrew(e); cmd != nil {
			return "Its log", cmd
		}
	case e.Brew != "":
		return "Bring it up", m.startBrew(e.Brew)
	case e.Container != "":
		return "Its output", m.watchContainer(e)
	case e.Declared != "" && e.Status == work.StatusDown:
		// Down: brought up, and gone into. One started by hand is up,
		// and has no pane of conn's to go into.
		if path, d, ok := m.declarationOf(e); ok {
			return "Bring it up, go in", m.raise(path, d, "", true)
		}
	}
	return "", nil
}

// endOn is what x arms on a row: the question, and what a yes to it
// runs. Nil where there is nothing to end: a container already ended, a
// brew service not running, a declaration that is down.
func (m model) endOn(e work.Entry) *pendingKill {
	switch {
	// A container is stopped rather than signalled: there is no process
	// here to send anything to, and docker's stop asks it to go before
	// insisting. By its service, which is what it is called here: the
	// row's own label carries the ports it publishes, and a question
	// that reads STOP CACHE · :6390 is asking about an address.
	case e.Container != "":
		if e.Status == work.StatusEnded || e.Fault {
			return nil
		}
		name := e.Command
		if c := m.containerAt(e.PID); c != nil {
			name = c.Service
		}
		return &pendingKill{prompt: stopPrompt(e.Container, name), end: m.stopContainer(e.Container, name)}
	case e.Brew != "":
		if e.Status != work.StatusActive {
			return nil
		}
		name := declaredNameOf(e)
		if name == "" {
			name = e.Brew
		}
		return &pendingKill{prompt: brewStopPrompt(e.Brew, name), end: m.stopBrew(e.Brew, name)}
	// A declaration in a pane conn opened for it is stopped in that
	// pane. One started by hand is a process like any other, wherever it
	// runs, and is signalled as one.
	case e.Declared != "" && (e.Status == work.StatusDown || m.panes[e.TTY].Declared == e.Declared):
		return m.endDeclared(e)
	// A row that is down is nothing running, and a pid below one is a
	// pid conn made up to hold the cursor with: there is nothing to end,
	// and a signal to it would reach a process group.
	case e.Status == work.StatusDown || e.PID <= 0:
		return nil
	}
	// A shell whose rows are folded says what it runs, and x on it is x
	// on that: the command is asked to end and the shell is left at its
	// prompt, as it is when the command has a row of its own.
	if e.Kind == work.KindShell && e.Under != "" {
		if run, ok := m.runsOf(e); ok {
			return m.signalling(run, syscall.SIGTERM)
		}
	}
	return m.signalling(e, killSignal(e.Kind))
}

// signalling is a kill that signals a row's process. The question
// names the program: a contact's whole command line is the note conn
// handed it, and a question that long is not read.
func (m model) signalling(e work.Entry, sig syscall.Signal) *pendingKill {
	name := work.Program(e.AsTyped())
	return &pendingKill{prompt: killPrompt(name, e.PID, sig), end: m.killEntry(e.PID, name, sig)}
}

// endDeclared is x on a declared process's row. Down, there is nothing
// to end. Ended and holding its pane, the pane is what goes, and the
// question says close. Up, it is sent ctrl-c in its pane, the way a
// hand stops what it ran in the foreground, and the pane goes once the
// end is recorded, so the row is DOWN in the one move rather than
// ENDED for a second x.
func (m model) endDeclared(e work.Entry) *pendingKill {
	if e.TTY == "" {
		return nil
	}
	_, name, _ := work.UnmarkDeclared(e.Declared)
	id := m.panes[e.TTY].ID
	if m.panes[e.TTY].Exit != "" {
		return &pendingKill{prompt: closePrompt(id, name), end: m.closeHeld(id, name)}
	}
	return &pendingKill{prompt: interruptPrompt(id, name), end: m.interruptDeclared(id, name)}
}

// raiseOn is what u does on a row: a declared process that is down, or
// ended and holding its pane, opened anew in place of that pane; a brew
// service, started by brew. The keys stay on the panel, so the next u
// is the next row. Nil on a row that is up, or is no declaration.
func (m model) raiseOn(e work.Entry) tea.Cmd {
	if !m.inside || !rowDown(e, m.panes) {
		return nil
	}
	if e.Brew != "" {
		return m.startBrew(e.Brew)
	}
	path, d, ok := m.declarationOf(e)
	if !ok {
		return nil
	}
	return m.raise(path, d, m.panes[e.TTY].ID, false)
}

// rowDown says whether a row is a declaration that is not up, which is
// what u would bring up: down, or ended and holding its pane; a brew
// service, by brew's word.
func rowDown(e work.Entry, panes map[string]tmux.Pane) bool {
	switch {
	case e.Declared == "":
		return false
	case e.Brew != "":
		return e.Status != work.StatusActive
	case e.TTY == "":
		return e.Status == work.StatusDown
	}
	return panes[e.TTY].Exit != ""
}
