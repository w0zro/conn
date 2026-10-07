package main

import (
	"time"

	"github.com/w0zro/conn/internal/console"
	"github.com/w0zro/conn/internal/station"

	tea "charm.land/bubbletea/v2"
)

// The console as the panel holds it: the station once it has been
// read, and how far the console has come on. It comes on a stage at a
// time — the header at once, from what is known before anything is
// read, then the readout once the station is read and its beat has
// passed, then the checks one by one, then the verdict.
type consoleOn struct {
	st    *station.Station // the station, once read
	stage int              // the stage the console has come on to
	due   bool             // the readout's beat has passed and it waits on the station
}

// stationRead is the station read. Where the readout's beat has already
// passed and was waiting on it, the readout comes on now.
func (m model) stationRead(st station.Station) (model, tea.Cmd) {
	m.console.st = &st
	if !m.console.due {
		return m, nil
	}
	m.console.due = false
	return m.advance()
}

// stageDue is the next stage's beat: the readout waits for the station
// to be read, and every other stage comes on.
func (m model) stageDue() (model, tea.Cmd) {
	if m.console.stage+1 == console.StageReadout && m.console.st == nil {
		m.console.due = true
		return m, nil
	}
	return m.advance()
}

// consoleKey answers a key on the console. A key skips the sequence to
// the end; a key at the end continues to the processes view.
func (m model) consoleKey(k string) (model, tea.Cmd) {
	if k == "ctrl+c" || k == "q" {
		return m.leave()
	}
	if last := console.LastStage(m.report()); m.console.stage < last {
		m.console.stage = last
		return m, nil
	}
	// Told nowhere to look, conn cannot show the processes view at all:
	// it is every project work is happening in, and there are no
	// projects. So it asks, here, where going on from the console would
	// otherwise arrive at an empty list that means three different
	// things.
	if len(m.roots.real) == 0 {
		return m.toRoots()
	}
	// The console holds until the processes view has something to show.
	// Going at once put an empty view up, filled it a tenth of a second
	// later when the table had been read, and moved it to the panel's
	// width after that — three screens to arrive at one. The console is a
	// still page and a moment more of it is not seen, where a view
	// assembling itself is.
	//
	// Only the first time. Coming back from the console the rows of the
	// last stay are still held, a couple of seconds old, and the
	// processes view goes up with them at once while the reading on its
	// way brings them up to date.
	m.processesGen++
	if len(m.projects) == 0 && m.processesErr == "" {
		m.entering = true
		return m, m.readProcesses()
	}
	m.view = viewProcesses
	if m.inside {
		return m, tea.Batch(m.readProcesses(), m.serverCmd(func() error { return m.srv.Narrow() }))
	}
	return m, m.readProcesses()
}

// report is the console's words as things stand: from the station once
// it is read, from the header's part of it before, and as the terminal
// in hand can hold them. The stages are counted off the report the
// screen will actually draw, so a console that gave up its per-root
// lines does not go on ticking through stages that have no row.
func (m model) report() console.Report {
	st := m.head
	if m.console.st != nil {
		st = *m.console.st
	}
	return console.Fitted(console.Compose(st, m.now), m.height)
}

// The time before each stage after the header: a beat for the readout
// and the verdict, less for each check.
func (m model) stageDelay(stage int) time.Duration {
	switch stage {
	case console.StageReadout, console.LastStage(m.report()):
		return 150 * time.Millisecond
	default:
		return 80 * time.Millisecond
	}
}

func (m model) nextStage() tea.Cmd {
	return tea.Tick(m.stageDelay(m.console.stage+1), func(time.Time) tea.Msg { return stageMsg{} })
}

// advance brings the next stage on and sets the one after it going.
func (m model) advance() (model, tea.Cmd) {
	last := console.LastStage(m.report())
	if m.console.stage < last {
		m.console.stage++
	}
	if m.console.stage < last {
		return m, m.nextStage()
	}
	return m, nil
}
