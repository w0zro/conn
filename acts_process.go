package main

import (
	"syscall"

	tea "charm.land/bubbletea/v2"
	"github.com/w0zro/conn/internal/room"
	"github.com/w0zro/conn/internal/work"
)

// signalling is a kill that signals a row's process. The question
// names the program: a contact's whole command line is the note conn
// handed it, and a question that long is not read.
func (m model) signalling(e work.Entry, sig syscall.Signal) *pendingKill {
	name := work.Program(e.AsTyped())
	return &pendingKill{prompt: killPrompt(name, e.PID, sig), end: m.killEntry(e.PID, name, sig)}
}

// killEntry signals a process, off the loop.
func (m model) killEntry(pid int, command string, sig syscall.Signal) tea.Cmd {
	return func() tea.Msg {
		_ = signal(pid, sig)
		return killedMsg{command: command, pid: pid, sig: sig}
	}
}

// processActs are the keys on a process of this machine: nothing for
// enter past its pane, and nothing to bring up; x signals it.
type processActs struct{}

func (processActs) enter(model, work.Entry) (string, tea.Cmd)  { return "", nil }
func (processActs) raise(model, work.Entry) tea.Cmd            { return nil }
func (processActs) down(work.Entry, map[string]room.Pane) bool { return false }

// end signals the process. A row that is down is nothing running, and
// a pid below one is a pid conn made up to hold the cursor with: there
// is nothing to end, and a signal to it would reach a process group.
// A shell whose rows are folded says what it runs, and x on it is x on
// that: the command is asked to end and the shell is left at its
// prompt, as it is when the command has a row of its own.
func (processActs) end(m model, e work.Entry) *pendingKill {
	if e.Status == work.StatusDown || e.PID <= 0 {
		return nil
	}
	if e.Kind == work.KindShell && e.Under != "" {
		if run, ok := m.runsOf(e); ok {
			return m.signalling(run, syscall.SIGTERM)
		}
	}
	return m.signalling(e, killSignal(e.Kind))
}
