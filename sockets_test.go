package main

import (
	"strings"
	"testing"
	"time"

	"github.com/w0zro/conn/internal/draw"
	"github.com/w0zro/conn/internal/work"
)

// The page says what a row has open: what it listens on first, then
// what it is connected to, then its unix sockets, each kind labelled
// once; a row with nothing open has no such group.
func TestThePageSaysWhatARowListensOn(t *testing.T) {
	// A run's page, which is the groups; a contact's is the sheet, and
	// a contact has nothing open to the world worth its page.
	s := readoutSubj()
	s.entry.Kind, s.entry.Command, s.entry.Typed = work.KindRun, "node vite", ""
	s.entry.Sockets = []work.Socket{
		{Proto: "TCP", Addr: "127.0.0.1:5173->127.0.0.1:60322", State: "ESTABLISHED"},
		{Proto: "TCP", Addr: "*:5173", State: "LISTEN"},
		{Proto: "UDP", Addr: "*:*", State: ""},
		{Proto: "unix", Addr: "/tmp/dev.sock", State: ""},
	}
	text := texts(drawReadout(composeReadout(s, "/Users/w0zro", processesNow), 100, 60, draw.Plain))
	for _, want := range []string{"SOCKETS", "Listens ... TCP *:5173", "Connected . TCP 127.0.0.1:5173->127.0.0.1:60322", "Unix ...... /tmp/dev.sock"} {
		if !strings.Contains(text, want) {
			t.Errorf("the page lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "*:*") {
		t.Errorf("a UDP socket bound nowhere is on the page:\n%s", text)
	}
	if strings.Index(text, "LISTENS") > strings.Index(text, "CONNECTED") {
		t.Errorf("what listens is not said first:\n%s", text)
	}
	s.entry.Sockets = nil
	if quiet := texts(drawReadout(composeReadout(s, "/Users/w0zro", processesNow), 100, 60, draw.Plain)); strings.Contains(quiet, "SOCKETS") {
		t.Errorf("a row with nothing open has a sockets group:\n%s", quiet)
	}
}

// A listener that has closed keeps its row on the panel. Its port is
// what had lifted it onto the head that runs it; with the port gone
// the row would have folded into that head and taken the fault with
// it, leaving a project whose dev server is unreachable saying nothing
// at all.
func TestAClosedListenerKeepsItsRow(t *testing.T) {
	began := time.Now().Add(-time.Hour)
	rows := func(ports []string) []work.Project {
		return []work.Project{{Path: "/w/a", Entries: []work.Entry{
			{PID: 300, Kind: work.KindShell, Command: "npm run dev", Started: began, Status: work.StatusActive, Depth: 0},
			{PID: 301, Kind: work.KindRun, Command: "node vite", Started: began, Status: work.StatusActive, Depth: 1, Ports: ports},
		}}}
	}
	was := work.MarkClosed(rows([]string{"5173"}), nil)
	for range work.ClosedAfter {
		was = work.MarkClosed(rows(nil), was)
	}
	gone := rows(nil)
	work.MarkClosed(gone, was)
	folded := fold(gone)
	if len(folded[0].Entries) != 2 {
		t.Fatalf("the fold left %d rows: %+v", len(folded[0].Entries), folded[0].Entries)
	}
	if e := folded[0].Entries[1]; e.Status != work.StatusClosed || !e.Fault {
		t.Errorf("the listener's row says %q, fault %v", e.Status, e.Fault)
	}
	// And the project's block says the fault at the end of its rule.
	b := composeProcesses(folded, nil, "", nil, func(string) bool { return true }, "/h", time.Now(), "", false, true)
	if word, stamped, _ := verdict(b.projects[0].rows); word != string(work.StatusClosed) || !stamped {
		t.Errorf("the block says %q, stamped %v", word, stamped)
	}
}
