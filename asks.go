package main

import (
	"github.com/w0zro/conn/internal/work/stationlog"

	tea "charm.land/bubbletea/v2"
)

// A view keeps its own rows, its cursor and what is typed into it, and
// answers the keys that move them. What a key asks past that - to leave,
// to go back, to go somewhere else - is the panel's, and a view says it
// by answering an ask, which the panel carries out.
type ask interface{ asked() }

type (
	askLeave struct{} // conn is done: ctrl+c, or q where q leaves
	askBack  struct{} // back to where the keys came from
	// askChord is a g that is nothing on its own: the first half of gg,
	// which the panel holds and gives the view back whole as "gg".
	askChord  struct{}
	askOutput struct{ project string }        // the output of a project, searched
	askLine   struct{ line stationlog.Event } // the row a line of the log is about
)

func (askLeave) asked()  {}
func (askBack) asked()   {}
func (askChord) asked()  {}
func (askOutput) asked() {}
func (askLine) asked()   {}

// answer carries out what a view's key asked of the panel; nothing for
// a key the view answered on its own.
func (m model) answer(a ask) (model, tea.Cmd) {
	switch a := a.(type) {
	case askLeave:
		return m.leave()
	case askBack:
		return m.backFrom()
	case askChord:
		m.firstG = true
	case askOutput:
		return m.openOutput(a.project, m.from)
	case askLine:
		if r, ok := m.logEntry(a.line); ok {
			m.from = ""
			return m.goTo(r)
		}
	}
	return m, nil
}
