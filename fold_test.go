package main

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/w0zro/conn/internal/draw"
	"github.com/w0zro/conn/internal/work"
)

// At rest a tree shows its head and, under it, only what can want you:
// a contact wherever it is, a fault, a row that is waiting. The rest
// folds into the row it stood under, and a shell says what it runs —
// looking through a bash -c to the command. What is kept stands under
// the nearest row kept, a level in.
func TestTheViewAtRestIsTheFold(t *testing.T) {
	projects := []work.Project{{Path: "/w", Entries: []work.Entry{
		{PID: 1, Kind: work.KindShell, Typed: "zsh", Status: work.StatusActive},
		{PID: 2, Kind: work.KindShell, Typed: "bash -c go test ./...", Status: work.StatusActive, Depth: 1},
		{PID: 3, Kind: work.KindRun, Typed: "go test ./...", Status: work.StatusActive, Depth: 2},
		{PID: 4, Kind: work.KindRun, Typed: "conn.test", Status: work.StatusActive, Depth: 3},
		{PID: 5, Kind: work.KindContact, Typed: "claude", Status: work.StatusWorking, Doing: "read tui.go", Depth: 2},
		{PID: 6, Kind: work.KindRun, Typed: "node mcp.js", Status: work.StatusActive, Depth: 3},
		{PID: 7, Kind: work.KindShell, Typed: "bash -c make", Status: work.StatusWaiting, Depth: 3},
		{PID: 8, Kind: work.KindEditor, Typed: "vim notes.md", Status: work.StatusStopped, Fault: true, Depth: 1},
		{PID: 9, Kind: work.KindRun, Typed: "sleep 9", Status: work.StatusActive},
		{PID: 10, Kind: work.KindRun, Typed: "npm run dev", Status: work.StatusActive, Depth: 1},
	}}, {Path: "/x", Note: "a note", Entries: []work.Entry{
		{PID: 20, Kind: work.KindShell, Typed: "zsh", Status: work.StatusIdle},
	}}}
	got := fold(projects)
	var rows []string
	for _, pl := range got {
		rows = append(rows, pl.Path+" "+pl.Note)
		for _, e := range pl.Entries {
			rows = append(rows, strings.Repeat(" ", e.Depth+1)+e.Kind+" "+activityOf(e)+" "+string(e.Status))
		}
	}
	// What is kept stands in the panel's order, by kind: the contact,
	// then the shell at its prompt, then the editor, then the work — and
	// the shell running go test ranks with the work, being what it runs.
	// See byKind. The depth each row keeps is no longer an indent on the
	// panel; it is what headOf finds a pane's head by.
	want := []string{
		"/w ",
		"  CONTACT read tui.go WORKING",
		"   SHELL bash -c make WAITING",
		"  EDITOR vim notes.md STOPPED",
		" SHELL go test ./... ACTIVE",
		" RUN sleep 9 ACTIVE",
		"/x a note",
		" SHELL zsh IDLE",
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("folded:\n%s\nwant:\n%s", strings.Join(rows, "\n"), strings.Join(want, "\n"))
	}
	// The row's own command is kept under what it says: the kill
	// question and the page name the shell, not what it runs.
	if e := rowOf(got[0].Entries, 1); e.AsTyped() != "zsh" || e.Under != "go test ./..." {
		t.Errorf("the head: typed %q, under %q", e.AsTyped(), e.Under)
	}
	// The projects given are left as they were.
	if len(projects[0].Entries) != 10 || projects[0].Entries[0].Under != "" {
		t.Error("the tree given was written to")
	}
}

// A service stays at rest. It is a thing to reach — a port to go to, a
// health to watch — and not a step in what its compose is doing, so a
// healthy service under a stack is on the panel without z, where the
// compose plugin between them is not.
//
// The services stand above the stack that brought them up, services
// ranking before runs; see byKind. The stack is not their heading on
// the panel, which is a list of rows and not a tree of them — what
// holds what is on z — and the thing to reach is the service, which is
// the row the operator wants first.
func TestAServiceStaysAtRest(t *testing.T) {
	projects := []work.Project{{Path: "/w", Entries: []work.Entry{
		{PID: 500, Kind: work.KindRun, Command: "stack · docker compose up", Typed: "stack · docker compose up", TTY: "ttys040", Status: work.StatusActive},
		{PID: 501, Kind: work.KindRun, Command: "docker compose up", Typed: "docker compose up", TTY: "ttys040", Status: work.StatusActive, Depth: 1},
		{PID: 502, Kind: work.KindRun, Command: "docker-compose compose up", Typed: "docker-compose compose up", TTY: "ttys040", Status: work.StatusActive, Depth: 2},
		{PID: -5, Kind: work.KindService, Command: "web", Typed: "web", Ports: []string{"8080"}, Status: work.StatusActive, Depth: 3, Container: "aaa"},
		{PID: -6, Kind: work.KindService, Command: "db", Typed: "db", Status: "UNHEALTHY", Fault: true, Depth: 3, Container: "bbb"},
	}}}
	var rows []string
	for _, e := range fold(projects)[0].Entries {
		rows = append(rows, strings.Repeat(" ", e.Depth)+e.Kind+" "+e.Command)
	}
	want := []string{" SERVICE web", " SERVICE db", "RUN stack · docker compose up"}
	if !slices.Equal(rows, want) {
		t.Errorf("at rest:\n%s\nwant:\n%s", strings.Join(rows, "\n"), strings.Join(want, "\n"))
	}
}

// One process alone listening under a head folds into it: the head
// takes the port and is the serving row, worded as what was typed or
// declared, since npm run dev and the node under it that holds the
// port are one server to the operator. A shell that says what it runs
// takes the port the same way. Several ports under one head are
// several servers, and each keeps its row; a port under a contact is
// the contact's to leave alone; and what stood under the listener
// stands under the head.
func TestALoneListenerFoldsIntoItsHead(t *testing.T) {
	projects := []work.Project{{Path: "/w", Entries: []work.Entry{
		{PID: 1, Kind: work.KindRun, Typed: "npm run dev", Status: work.StatusActive},
		{PID: 2, Kind: work.KindRun, Typed: "node /w/node_modules/.bin/vite", Status: work.StatusActive, Depth: 1, Ports: []string{"5174"}, Sockets: []work.Socket{{Proto: "TCP", Addr: "*:5174", State: "LISTEN"}, {Proto: "TCP", Addr: "127.0.0.1:5174->127.0.0.1:60322", State: "ESTABLISHED"}}},
		{PID: 3, Kind: work.KindShell, Typed: "zsh", Status: work.StatusActive},
		{PID: 4, Kind: work.KindRun, Typed: "npm run dev:web", Status: work.StatusActive, Depth: 1},
		{PID: 5, Kind: work.KindRun, Typed: "node vite", Status: work.StatusActive, Depth: 2, Ports: []string{"5173"}},
		{PID: 6, Kind: work.KindShell, Typed: "bash -c make", Status: work.StatusWaiting, Depth: 3},
		{PID: 7, Kind: work.KindRun, Typed: "npm run dev", Status: work.StatusActive, Ports: []string{"24678"}},
		{PID: 8, Kind: work.KindRun, Typed: "node vite", Status: work.StatusActive, Depth: 1, Ports: []string{"5175"}},
		{PID: 9, Kind: work.KindRun, Typed: "turbo dev", Status: work.StatusActive},
		{PID: 10, Kind: work.KindRun, Typed: "next dev", Status: work.StatusActive, Depth: 1, Ports: []string{"3000"}},
		{PID: 11, Kind: work.KindRun, Typed: "node api.js", Status: work.StatusActive, Depth: 1, Ports: []string{"4000"}},
		{PID: 12, Kind: work.KindContact, Typed: "claude", Status: work.StatusWorking},
		{PID: 13, Kind: work.KindRun, Typed: "python -m http.server", Status: work.StatusActive, Depth: 1, Ports: []string{"8000"}},
	}}}
	var rows []string
	for _, e := range fold(projects)[0].Entries {
		rows = append(rows, strings.Repeat(" ", e.Depth)+e.Kind+" "+activityOf(e)+draw.PortsWord(e.Ports))
	}
	// In the panel's order, by kind: the contact, then the shell that is
	// only a shell, then the work — which the two shells standing for
	// what they run are part of. See byKind.
	want := []string{
		"CONTACT claude",
		" SHELL bash -c make",
		"RUN npm run dev · :5174",
		"SHELL npm run dev:web · :5173",
		"RUN npm run dev · :24678",
		" RUN node vite · :5175",
		"RUN turbo dev",
		" RUN next dev · :3000",
		" RUN node api.js · :4000",
		" RUN python -m http.server · :8000",
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("folded:\n%s\nwant:\n%s", strings.Join(rows, "\n"), strings.Join(want, "\n"))
	}
	// The head is still its own process: the kill question and the
	// page go by its pid, and its command is what it was. It says whose
	// port it carries, and carries the sockets too, so its page says
	// what it listens on and is connected to.
	head := rowOf(fold(projects)[0].Entries, 1)
	if head.PID != 1 || head.AsTyped() != "npm run dev" || head.Listener != "node /w/node_modules/.bin/vite" || len(head.Sockets) != 2 {
		t.Errorf("the head: %+v", head)
	}
	s := readoutSubj()
	s.entry = head
	text := texts(drawReadout(composeReadout(s, "/Users/w0zro", processesNow), 100, 60, draw.Plain))
	for _, want := range []string{"Command ... npm run dev", "Listens ... TCP *:5174 · 1 client"} {
		if !strings.Contains(text, want) {
			t.Errorf("the folded head's page lacks %q:\n%s", want, text)
		}
	}
	if len(projects[0].Entries[0].Ports) != 0 || len(projects[0].Entries[0].Sockets) != 0 {
		t.Error("the tree given was written to")
	}
}

// rowOf is a row by its pid among those a fold kept, for the tests that
// mean one row and not whichever stands first: the rows are in the
// panel's order, by kind, and not the order they were written in.
func rowOf(rows []work.Entry, pid int) work.Entry {
	for _, e := range rows {
		if e.PID == pid {
			return e
		}
	}
	return work.Entry{}
}
