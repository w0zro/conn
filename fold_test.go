package main

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

// At rest a tree shows its head and, under it, only what can want you:
// a contact wherever it is, a fault, a row that is waiting. The rest
// folds into the row it stood under, and a shell says what it runs —
// looking through a bash -c to the command. What is kept stands under
// the nearest row kept, a level in.
func TestTheViewAtRestIsTheFold(t *testing.T) {
	projects := []project{{path: "/w", entries: []entry{
		{pid: 1, kind: kindShell, typed: "zsh", status: statusActive},
		{pid: 2, kind: kindShell, typed: "bash -c go test ./...", status: statusActive, depth: 1},
		{pid: 3, kind: kindRun, typed: "go test ./...", status: statusActive, depth: 2},
		{pid: 4, kind: kindRun, typed: "conn.test", status: statusActive, depth: 3},
		{pid: 5, kind: kindContact, typed: "claude", status: statusWorking, doing: "read tui.go", depth: 2},
		{pid: 6, kind: kindRun, typed: "node mcp.js", status: statusActive, depth: 3},
		{pid: 7, kind: kindShell, typed: "bash -c make", status: statusWaiting, depth: 3},
		{pid: 8, kind: kindEditor, typed: "vim notes.md", status: statusStopped, fault: true, depth: 1},
		{pid: 9, kind: kindRun, typed: "sleep 9", status: statusActive},
		{pid: 10, kind: kindRun, typed: "npm run dev", status: statusActive, depth: 1},
	}}, {path: "/x", note: "a note", entries: []entry{
		{pid: 20, kind: kindShell, typed: "zsh", status: statusIdle},
	}}}
	got := fold(projects)
	var rows []string
	for _, pl := range got {
		rows = append(rows, pl.path+" "+pl.note)
		for _, e := range pl.entries {
			rows = append(rows, strings.Repeat(" ", e.depth+1)+e.kind+" "+activityOf(e)+" "+e.status)
		}
	}
	want := []string{
		"/w ",
		" SHELL go test ./... ACTIVE",
		"  CONTACT read tui.go WORKING",
		"   SHELL bash -c make WAITING",
		"  EDITOR vim notes.md STOPPED",
		" RUN sleep 9 ACTIVE",
		"/x a note",
		" SHELL zsh IDLE",
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("folded:\n%s\nwant:\n%s", strings.Join(rows, "\n"), strings.Join(want, "\n"))
	}
	// The row's own command is kept under what it says: the kill
	// question and the page name the shell, not what it runs.
	if e := got[0].entries[0]; e.asTyped() != "zsh" || e.under != "go test ./..." {
		t.Errorf("the head: typed %q, under %q", e.asTyped(), e.under)
	}
	// The projects given are left as they were.
	if len(projects[0].entries) != 10 || projects[0].entries[0].under != "" {
		t.Error("the tree given was written to")
	}
}

// A service stays at rest. It is a thing to reach — a port to go to, a
// health to watch — and not a step in what its compose is doing, so a
// healthy service under a stack is on the panel without z, where the
// compose plugin between them is not.
func TestAServiceStaysAtRest(t *testing.T) {
	projects := []project{{path: "/w", entries: []entry{
		{pid: 500, kind: kindRun, command: "stack · docker compose up", typed: "stack · docker compose up", tty: "ttys040", status: statusActive},
		{pid: 501, kind: kindRun, command: "docker compose up", typed: "docker compose up", tty: "ttys040", status: statusActive, depth: 1},
		{pid: 502, kind: kindRun, command: "docker-compose compose up", typed: "docker-compose compose up", tty: "ttys040", status: statusActive, depth: 2},
		{pid: -5, kind: kindService, command: "web", typed: "web", ports: []string{"8080"}, status: statusActive, depth: 3, container: "aaa"},
		{pid: -6, kind: kindService, command: "db", typed: "db", status: "UNHEALTHY", fault: true, depth: 3, container: "bbb"},
	}}}
	var rows []string
	for _, e := range fold(projects)[0].entries {
		rows = append(rows, strings.Repeat(" ", e.depth)+e.kind+" "+e.command)
	}
	want := []string{"RUN stack · docker compose up", " SERVICE web", " SERVICE db"}
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
	projects := []project{{path: "/w", entries: []entry{
		{pid: 1, kind: kindRun, typed: "npm run dev", status: statusActive},
		{pid: 2, kind: kindRun, typed: "node /w/node_modules/.bin/vite", status: statusActive, depth: 1, ports: []string{"5174"}, sockets: []socket{{"TCP", "*:5174", "LISTEN"}, {"TCP", "127.0.0.1:5174->127.0.0.1:60322", "ESTABLISHED"}}},
		{pid: 3, kind: kindShell, typed: "zsh", status: statusActive},
		{pid: 4, kind: kindRun, typed: "npm run dev:web", status: statusActive, depth: 1},
		{pid: 5, kind: kindRun, typed: "node vite", status: statusActive, depth: 2, ports: []string{"5173"}},
		{pid: 6, kind: kindShell, typed: "bash -c make", status: statusWaiting, depth: 3},
		{pid: 7, kind: kindRun, typed: "npm run dev", status: statusActive, ports: []string{"24678"}},
		{pid: 8, kind: kindRun, typed: "node vite", status: statusActive, depth: 1, ports: []string{"5175"}},
		{pid: 9, kind: kindRun, typed: "turbo dev", status: statusActive},
		{pid: 10, kind: kindRun, typed: "next dev", status: statusActive, depth: 1, ports: []string{"3000"}},
		{pid: 11, kind: kindRun, typed: "node api.js", status: statusActive, depth: 1, ports: []string{"4000"}},
		{pid: 12, kind: kindContact, typed: "claude", status: statusWorking},
		{pid: 13, kind: kindRun, typed: "python -m http.server", status: statusActive, depth: 1, ports: []string{"8000"}},
	}}}
	var rows []string
	for _, e := range fold(projects)[0].entries {
		rows = append(rows, strings.Repeat(" ", e.depth)+e.kind+" "+activityOf(e)+portsWord(e.ports))
	}
	want := []string{
		"RUN npm run dev · :5174",
		"SHELL npm run dev:web · :5173",
		" SHELL bash -c make",
		"RUN npm run dev · :24678",
		" RUN node vite · :5175",
		"RUN turbo dev",
		" RUN next dev · :3000",
		" RUN node api.js · :4000",
		"CONTACT claude",
		" RUN python -m http.server · :8000",
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("folded:\n%s\nwant:\n%s", strings.Join(rows, "\n"), strings.Join(want, "\n"))
	}
	// The head is still its own process: the kill question and the
	// page go by its pid, and its command is what it was. It says whose
	// port it carries, and carries the sockets too, so its page says
	// what it listens on and is connected to.
	head := fold(projects)[0].entries[0]
	if head.pid != 1 || head.asTyped() != "npm run dev" || head.listener != "node /w/node_modules/.bin/vite" || len(head.sockets) != 2 {
		t.Errorf("the head: %+v", head)
	}
	s := readoutSubj()
	s.entry = head
	text := texts(drawReadout(composeReadout(s, "/Users/w0zro", processesNow), 100, 60, plain))
	for _, want := range []string{"Command ... npm run dev", "Listens ... TCP *:5174", "Connected . TCP 127.0.0.1:5174->127.0.0.1:60322"} {
		if !strings.Contains(text, want) {
			t.Errorf("the folded head's page lacks %q:\n%s", want, text)
		}
	}
	if len(projects[0].entries[0].ports) != 0 || len(projects[0].entries[0].sockets) != 0 {
		t.Error("the tree given was written to")
	}
}
