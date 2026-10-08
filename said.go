package main

import (
	"time"

	"github.com/w0zro/conn/internal/work"
	"github.com/w0zro/conn/internal/work/stationlog"

	tea "charm.land/bubbletea/v2"
)

// The station listens. On every reading the tail of every pane conn
// holds is read again, and what a pane has said since the last tail
// is read for its stack's words for something gone wrong: a panic, a
// test's FAIL, a traceback, an uncaught error, a port already in use;
// see work.Said. A pane that said one is a line in the log, the word
// as the stack says it and the line it said under it, and the band
// counts it with the rest. The row goes on saying what the machine
// says of it, ACTIVE, since the process is still running; the log is
// where what it said is kept.
//
// A pane's first tail is seen and not said: what a pane had printed
// before conn was listening is not news. One line a pane a reading: a
// traceback is ten lines that say it once.

// tailLines is how much of a pane is read on a beat: a burst longer
// than this between two readings is read from its top, and every line
// of it is new.
const tailLines = 200

// A saidMsg is the tails as read on a beat, by pane id.
type saidMsg struct {
	tails map[string][]string
	at    time.Time
}

// A listener is a pane conn holds, as the log would name what it says.
type listener struct {
	id, tty, project, label string
	pid                     int
}

// listeners is every pane conn holds for a row, each once, with the
// row the log names it by.
func (m model) listeners() []listener {
	var out []listener
	seen := map[string]bool{}
	for _, pl := range m.projects {
		for _, e := range pl.Entries {
			p, ok := m.panes[e.TTY]
			if !ok || p.ID == "" || seen[e.TTY] {
				continue
			}
			seen[e.TTY] = true
			out = append(out, listener{id: p.ID, tty: e.TTY, project: pl.Path, label: logLabel(e), pid: e.PID})
		}
	}
	return out
}

// listen reads the tails, off the loop, for heard.
func (m model) listen() tea.Cmd {
	if !m.inside || m.srv == nil {
		return nil
	}
	srv, ls := m.srv, m.listeners()
	if len(ls) == 0 {
		return nil
	}
	return func() tea.Msg {
		tails := map[string][]string{}
		for _, l := range ls {
			if lines, err := srv.Tail(l.id, tailLines); err == nil {
				tails[l.id] = lines
			}
		}
		return saidMsg{tails: tails, at: time.Now()}
	}
}

// heard takes the tails as read: what each pane said since its last
// tail, read for its stack's words, and logged; and the tails kept for
// the next beat. A pane no longer held is forgotten with its tail.
func (m model) heard(msg saidMsg) (model, tea.Cmd) {
	var events []stationlog.Event
	kept := map[string][]string{}
	for _, l := range m.listeners() {
		lines, ok := msg.tails[l.id]
		if !ok {
			if was, ok := m.tails[l.id]; ok {
				kept[l.id] = was
			}
			continue
		}
		kept[l.id] = lines
		for _, line := range work.NewLines(m.tails[l.id], lines) {
			if word, ok := work.SaidWord(line); ok {
				events = append(events, stationlog.Event{At: msg.at, Project: l.project, Label: l.label, PID: l.pid, Word: word, Note: line})
				break
			}
		}
	}
	m.tails = kept
	return m.logged(events)
}
