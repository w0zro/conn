package main

import (
	"strings"
	"testing"

	"github.com/w0zro/conn/internal/work"
)

// A verb waits for its object: itself again is the row, j and k the
// rows beside it with the cursor going there, and anything else lets
// it go. . does the last verb and object again where the cursor is.
func TestAVerbWaitsForItsObject(t *testing.T) {
	m := plainModel()
	m.view = viewProcesses
	m.projects = []work.Project{{Path: "/w", Entries: []work.Entry{
		{PID: 11, Kind: work.KindContact, Command: "claude"},
		{PID: 12, Kind: work.KindRun, Command: "vite"},
		{PID: 13, Kind: work.KindRun, Command: "node build.js"},
	}}}
	m = m.onRow(11, 0)

	m, _ = press(m, "x")
	if m.verb != "x" || m.kill != nil {
		t.Fatalf("x alone: verb %q, kill %+v", m.verb, m.kill)
	}
	if !strings.Contains(m.bar(), "xx") || !strings.Contains(m.bar(), "xp") {
		t.Errorf("the bar does not say what x waits for:\n%s", m.bar())
	}
	m, _ = press(m, "x")
	if m.verb != "" || m.kill == nil || !strings.Contains(m.kill.prompt, "11 · claude?") {
		t.Fatalf("xx: verb %q, kill %+v", m.verb, m.kill)
	}

	m.kill = nil
	m, _ = press(m, "x", "j")
	if m.cursor != 12 || m.kill == nil || !strings.Contains(m.kill.prompt, "12 · vite?") {
		t.Fatalf("xj: cursor %d, kill %+v", m.cursor, m.kill)
	}

	m.kill = nil
	m, _ = press(m, ".")
	if m.cursor != 13 || m.kill == nil || !strings.Contains(m.kill.prompt, "13 · node?") {
		t.Fatalf(". after xj: cursor %d, kill %+v", m.cursor, m.kill)
	}

	m.kill = nil
	m, _ = press(m, "x", "z")
	if m.verb != "" || m.kill != nil || m.full {
		t.Errorf("x then z: verb %q, kill %+v, tree %v", m.verb, m.kill, m.full)
	}
}

// xp ends the project's work and leaves the operator's company: a
// contact, a shell, an editor stay, and a row under one taken goes
// with it rather than being asked twice.
func TestTheProjectsWorkIsWhatXpEnds(t *testing.T) {
	pl := work.Project{Path: "/w", Entries: []work.Entry{
		{PID: 11, Kind: work.KindContact, Command: "claude", Status: work.StatusWorking},
		{PID: 12, Kind: work.KindRun, Command: "npm run dev", Status: work.StatusActive, Depth: 1},
		{PID: 13, Kind: work.KindRun, Command: "vite", Status: work.StatusActive, Depth: 2},
		{PID: 14, Kind: work.KindShell, Command: "zsh", Status: work.StatusIdle},
		{PID: 15, Kind: work.KindEditor, Command: "nvim", Status: work.StatusIdle},
		{PID: -16, Kind: work.KindRun, Command: "worker", Status: work.StatusDown},
		{PID: 17, Kind: work.KindService, Command: "postgres", Status: work.StatusActive},
	}}
	var pids []int
	for _, e := range projectWork(pl) {
		pids = append(pids, e.PID)
	}
	if len(pids) != 2 || pids[0] != 12 || pids[1] != 17 {
		t.Errorf("the project's work is %v, want [12 17]", pids)
	}
	m := plainModel()
	m.view = viewProcesses
	m.projects = []work.Project{pl}
	m = m.onRow(11, 0)
	m, _ = press(m, "x", "p")
	if m.kill == nil || !strings.HasPrefix(m.kill.prompt, "end 2 of its processes") {
		t.Errorf("xp: kill %+v", m.kill)
	}
}

// The station's views are behind g, the letters alone being verbs:
// gp the projects, gr the recent sessions, gl the log, gc the console.
func TestTheStationIsBehindG(t *testing.T) {
	for keys, want := range map[string]view{"gp": viewProjects, "gr": viewSessions, "gl": viewLog, "gc": viewConsole} {
		m := plainModel()
		m.view, m.inside = viewProcesses, true
		if m, _ = press(m, keys[:1], keys[1:]); m.view != want {
			t.Errorf("%s: view %d, want %d", keys, m.view, want)
		}
	}
}
