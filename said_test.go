package main

import (
	"testing"
	"time"

	"github.com/w0zro/conn/internal/tmux"
	"github.com/w0zro/conn/internal/work"
)

func TestAPaneSayingItsStacksWordIsALine(t *testing.T) {
	m := model{p: Plain, width: 48, height: 40, view: viewProcesses, inside: true, srv: &tmux.Server{Tmux: "/nonexistent/tmux"}}
	m.projects = []work.Project{{Path: "/Users/w0zro/projects/web", Entries: []work.Entry{
		{PID: 41, Kind: work.KindShell, Command: "zsh", TTY: "ttys001", Status: work.StatusIdle},
		{PID: 43, Kind: work.KindRun, Command: "go", Typed: "go run .", TTY: "ttys001", Status: work.StatusActive},
		{PID: 42, Kind: work.KindRun, Command: "node", Typed: "node server.js", TTY: "ttys002", Status: work.StatusActive},
	}}}
	m.panes = map[string]tmux.Pane{"ttys001": {ID: "%4", TTY: "ttys001"}, "ttys002": {ID: "%5", TTY: "ttys002"}}
	m.seenAny, m.log.loaded = true, true // the view has read its file, so a line lands on it as it is written
	// Two rows share a pane, and the pane is listened to once, under
	// the row the pane is.
	if ls := m.listeners(); len(ls) != 2 || ls[0].id != "%4" || ls[0].label != "zsh" || ls[1].label != "node server.js" {
		t.Fatalf("the listeners are %+v", ls)
	}
	at := time.Date(2026, 10, 4, 17, 0, 0, 0, time.Local)
	// The first tails are seen and not said, old panics and all.
	m, _ = m.heard(saidMsg{at: at, tails: map[string][]string{
		"%4": {"$ go run .", "panic: old", "$ ", ""},
		"%5": {"listening on :3000", "", ""},
	}})
	if m.log.unseen != 0 {
		t.Fatalf("the first tail counted %d", m.log.unseen)
	}
	// Then a pane says a word, in ten lines that say it once.
	m, _ = m.heard(saidMsg{at: at.Add(2 * time.Second), tails: map[string][]string{
		"%4": {"$ go run .", "panic: old", "$ go test ./...", "--- FAIL: TestA (0.00s)", "    a_test.go:9: no", "FAIL", "FAIL\tweb\t0.1s", "$ ", ""},
		"%5": {"listening on :3000", "", ""},
	}})
	if m.log.unseen != 1 {
		t.Fatalf("the FAIL counted %d lines, want 1", m.log.unseen)
	}
	// The line is the word as the stack says it, under the pane's row,
	// with the line it said.
	if len(m.log.read) != 1 {
		t.Fatalf("the log holds %d lines", len(m.log.read))
	}
	e := m.log.read[0]
	if e.Word != "FAIL" || e.Label != "zsh" || e.PID != 41 || e.Note != "--- FAIL: TestA (0.00s)" || e.Project != "/Users/w0zro/projects/web" {
		t.Errorf("the line is %+v", e)
	}
	// The same tail again says nothing new.
	m, _ = m.heard(saidMsg{at: at.Add(4 * time.Second), tails: map[string][]string{
		"%4": {"$ go run .", "panic: old", "$ go test ./...", "--- FAIL: TestA (0.00s)", "    a_test.go:9: no", "FAIL", "FAIL\tweb\t0.1s", "$ ", ""},
	}})
	if m.log.unseen != 1 {
		t.Errorf("the same tail counted again: %d", m.log.unseen)
	}
	// Outside the server there is nothing to listen to.
	if (model{}).listen() != nil {
		t.Error("a conn outside the server listens")
	}
}
