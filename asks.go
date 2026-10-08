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
	askChord    struct{}
	askOutput   struct{ project string }        // the output of a project, searched
	askLine     struct{ line stationlog.Event } // the row a line of the log is about
	askTakeRoot struct{ root string }           // a root chosen, to write and work from
	askResume   struct{ dir, id string }        // a suspended session to continue
	askPreview  struct{}                        // the match under the cursor, shown in the bay
	askLand     struct {                        // a match's pane, gone into on the match
		hit  outMatch
		pane outPane
	}
)

func (askLeave) asked()    {}
func (askBack) asked()     {}
func (askChord) asked()    {}
func (askOutput) asked()   {}
func (askLine) asked()     {}
func (askTakeRoot) asked() {}
func (askResume) asked()   {}
func (askPreview) asked()  {}
func (askLand) asked()     {}

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
	case askTakeRoot:
		return m.takeRoot(a.root)
	case askResume:
		if m.inside {
			var cmd tea.Cmd
			m, cmd = m.toProcesses()
			return m, tea.Batch(cmd, m.openResumed(a.dir, a.id))
		}
	case askPreview:
		return m.previewing()
	case askLand:
		return m.land(a.hit, a.pane)
	}
	return m, nil
}
