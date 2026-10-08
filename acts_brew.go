package main

import (
	tea "charm.land/bubbletea/v2"
	"github.com/w0zro/conn/internal/room"
	"github.com/w0zro/conn/internal/shell"
	"github.com/w0zro/conn/internal/work"
	"github.com/w0zro/conn/internal/work/brew"
)

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
	cmd := "tail -n 2000 -f " + shell.Quote(log) + " 2>&1; " + shell.HoldOpen
	return func() tea.Msg {
		sh, err := srv.OpenWatching(dir, cmd, brew.Mark(formula))
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
		_, _ = brew.Says(brew.Wait, "services", "start", formula)
		return readBrew()
	}
}

// stopBrew asks brew to stop a service, the way x on a container asks
// docker: there is no process here conn would signal itself.
func (m model) stopBrew(formula, name string) tea.Cmd {
	return func() tea.Msg {
		_, _ = brew.Says(brew.Wait, "services", "stop", formula)
		return killedMsg{command: name, pid: 0}
	}
}

// brewActs are the keys on a service brew holds up for a project that
// declares it. brew holds it, not a pane: enter on it up is its log,
// and on it down brings it up, as u does; x stops it.
type brewActs struct{}

func (brewActs) enter(m model, e work.Entry) (string, tea.Cmd) {
	if e.Status == work.StatusActive {
		if cmd := m.watchBrew(e); cmd != nil {
			return "Its log", cmd
		}
		return "", nil
	}
	return "Bring it up", m.startBrew(e.Brew)
}

func (brewActs) end(m model, e work.Entry) *pendingKill {
	if e.Status != work.StatusActive {
		return nil
	}
	name := declaredNameOf(e)
	if name == "" {
		name = e.Brew
	}
	return &pendingKill{prompt: brewStopPrompt(e.Brew, name), end: m.stopBrew(e.Brew, name)}
}

func (brewActs) raise(m model, e work.Entry) tea.Cmd { return m.startBrew(e.Brew) }

// down is brew's word: a service it does not call active.
func (brewActs) down(e work.Entry, _ map[string]room.Pane) bool {
	return e.Status != work.StatusActive
}
