package main

import (
	"testing"

	"github.com/w0zro/conn/internal/work"
	"github.com/w0zro/conn/internal/work/declared"
)

// A contact's row goes by its session's title while it is not working,
// and by what it is doing while it is: the title is what tells one
// claude from another, and the activity is the one thing that changes.
func TestAContactsRowGoesByItsTitle(t *testing.T) {
	idle := work.Entry{PID: 1, Kind: work.KindContact, Status: work.StatusIdle, Title: "Do the thing"}
	if got := rowName(idle); got != "Do the thing" {
		t.Errorf("idle: %q", got)
	}
	waiting := work.Entry{PID: 2, Kind: work.KindContact, Status: work.StatusWaiting, Title: "Do the thing"}
	if got := rowName(waiting); got != "Do the thing" {
		t.Errorf("waiting: %q", got)
	}
	working := work.Entry{PID: 3, Kind: work.KindContact, Status: work.StatusWorking, Title: "Do the thing", Doing: "read tui.go"}
	if got := rowName(working); got != "" {
		t.Errorf("working: %q, the activity is the row's", got)
	}
	untitled := work.Entry{PID: 4, Kind: work.KindContact, Status: work.StatusIdle}
	if got := rowName(untitled); got != "" {
		t.Errorf("untitled: %q", got)
	}
	files := work.Entry{PID: 5, Kind: work.KindRun, Declared: declared.Mark("/w/app", "web")}
	if got := rowName(files); got != "web" {
		t.Errorf("declared: %q", got)
	}
}
