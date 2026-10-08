package main

import (
	"github.com/w0zro/conn/internal/room"
	"github.com/w0zro/conn/internal/work"

	tea "charm.land/bubbletea/v2"
)

// What enter, x and u do on the row under the cursor, each said once.
// The key does what these answer, and the bar offers what these
// answer, so the two cannot come apart: a row the bar offers a key on
// is a row the key works on, and one it does not is one the key leaves
// alone.
//
// A row is a process of this machine, a declaration from the project's
// .conn, a container, or a Homebrew service, as work.Source says, and
// each source answers for its own keys: acts_process.go, acts_declared.go,
// acts_container.go, acts_brew.go. What holds for every row is said
// here, once, before the source is asked.

// The acts of a row's source: what enter does past a pane conn already
// holds, what x arms, what u brings up, and whether the row is down,
// which is when u has anything to bring up.
type acts interface {
	enter(m model, e work.Entry) (string, tea.Cmd)
	end(m model, e work.Entry) *pendingKill
	raise(m model, e work.Entry) tea.Cmd
	down(e work.Entry, panes map[string]room.Pane) bool
}

// actsOf is the acts of a row's source.
func actsOf(e work.Entry) acts {
	switch e.Source() {
	case work.FromContainer:
		return containerActs{}
	case work.FromBrew:
		return brewActs{}
	case work.FromDeclared:
		return declaredActs{}
	}
	return processActs{}
}

// enterOn is what enter does on a row, and the word the bar offers it
// by; no command where it does nothing. Everything enter does opens or
// shows a pane of the server, so off it enter does nothing. A row conn
// already holds a pane for is gone into; that is enter everywhere.
// Past that, it is the source's: a container's output, a brew
// service's log, a declaration brought up.
func (m model) enterOn(e work.Entry) (string, tea.Cmd) {
	if !m.inside {
		return "", nil
	}
	if room.Reachable(m.panes[e.TTY]) {
		return "Open", m.reach(m.panes[e.TTY], e.TTY)
	}
	return actsOf(e).enter(m, e)
}

// endOn is what x arms on a row: the question, and what a yes to it
// runs. Nil where there is nothing to end.
func (m model) endOn(e work.Entry) *pendingKill {
	return actsOf(e).end(m, e)
}

// raiseOn is what u does on a row that is down: brought up anew, and
// the keys left on the panel, so the next u is the next row. Nil on a
// row that is up, or has nothing to bring up.
func (m model) raiseOn(e work.Entry) tea.Cmd {
	if !m.inside || !rowDown(e, m.panes) {
		return nil
	}
	return actsOf(e).raise(m, e)
}

// rowDown says whether a row is down in the sense u answers: a
// declaration that is not up, or a brew service brew says is not.
func rowDown(e work.Entry, panes map[string]room.Pane) bool {
	return actsOf(e).down(e, panes)
}
