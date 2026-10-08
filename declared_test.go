package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/w0zro/conn/internal/room"
	"github.com/w0zro/conn/internal/work"
	"github.com/w0zro/conn/internal/work/declared"
)

// A compose declaration that is down has the services it would bring
// up as rows under it, each down; one that is up has a down row for
// each service that has no container among its rows yet, and none for
// the ones that have.
func TestAComposeDeclarationsServicesAreRows(t *testing.T) {
	shop := "/r/shop"
	stack := declared.Declaration{Name: "stack", Command: "docker compose up"}
	files := map[string]declared.File{shop: {
		List:     []declared.Declaration{stack},
		Services: map[string][]string{"stack": {"api", "db", "web"}},
	}}
	// Down: nothing runs it, and a shell is open in the project.
	projects := []work.Project{{Path: shop, Entries: []work.Entry{
		{PID: 100, Kind: work.KindShell, Command: "zsh", Typed: "zsh", TTY: "ttys001", Status: work.StatusIdle},
	}}}
	got := declared.Attach(projects, files, nil)
	rows := func(pl work.Project) []string {
		var out []string
		for _, e := range pl.Entries {
			out = append(out, strings.Repeat(" ", e.Depth)+e.Kind+" "+e.Command+" "+e.Status)
		}
		return out
	}
	want := []string{"SHELL zsh IDLE", "RUN stack · docker compose up DOWN", " SERVICE api DOWN", " SERVICE db DOWN", " SERVICE web DOWN"}
	if !slices.Equal(rows(got[0]), want) {
		t.Errorf("down:\n%s\nwant:\n%s", strings.Join(rows(got[0]), "\n"), strings.Join(want, "\n"))
	}
	// Each service row holds the cursor by a pid of its own, and is
	// nothing to signal, stop or enter.
	if e := got[0].Entries[2]; e.PID == 0 || e.PID == got[0].Entries[1].PID || e.Declared != "" || e.Container != "" || e.Cwd != shop {
		t.Errorf("a down service row: %+v", e)
	}
	// And the fold keeps them, as rows that want bringing up.
	if kept := fold(got)[0]; len(kept.Entries) != 5 {
		t.Errorf("the fold took the down services: %d rows", len(kept.Entries))
	}

	// Up: the head runs, and docker has two of the three services under
	// it; the third is down under the head, after the rows it has.
	mark := declared.Mark(shop, "stack")
	up := []work.Project{{Path: shop, Entries: []work.Entry{
		{PID: 200, Kind: work.KindShell, Command: "sh -c docker compose up", Typed: "sh -c docker compose up", TTY: "ttys002", Status: work.StatusActive},
		{PID: 201, Kind: work.KindRun, Command: "docker compose up", Typed: "docker compose up", TTY: "ttys002", Status: work.StatusActive, Depth: 1},
		{PID: -5, Kind: work.KindService, Command: "api", Typed: "api", Ports: []string{"3000"}, Status: work.StatusActive, Depth: 2, Container: "aaa"},
		{PID: -6, Kind: work.KindService, Command: "web", Typed: "web", Ports: []string{"8080"}, Status: work.StatusActive, Depth: 2, Container: "bbb"},
		{PID: 300, Kind: work.KindShell, Command: "zsh", Typed: "zsh", TTY: "ttys003", Status: work.StatusIdle},
	}}}
	panes := map[string]room.Pane{"ttys002": {ID: "%2", TTY: "ttys002", Declared: mark}}
	got = declared.Attach(up, files, declaredPanes(panes))
	want = []string{"RUN stack · docker compose up ACTIVE", " RUN docker compose up ACTIVE", "  SERVICE api ACTIVE", "  SERVICE web ACTIVE", " SERVICE db DOWN", "SHELL zsh IDLE"}
	if !slices.Equal(rows(got[0]), want) {
		t.Errorf("up:\n%s\nwant:\n%s", strings.Join(rows(got[0]), "\n"), strings.Join(want, "\n"))
	}
	if len(up[0].Entries) != 5 {
		t.Error("the model's own rows were written to")
	}

	// By hand: the same stack typed into a shell, no pane marked. The
	// row that ran the command is the stack, and the missing service
	// is down under it, not under the shell.
	hand := []work.Project{{Path: shop, Entries: []work.Entry{
		{PID: 300, Kind: work.KindShell, Command: "zsh", Typed: "zsh", TTY: "ttys003", Status: work.StatusIdle, Cwd: shop},
		{PID: 301, Kind: work.KindRun, Command: "docker compose up", Typed: "docker compose up", TTY: "ttys003", Status: work.StatusActive, Depth: 1, Cwd: shop},
		{PID: -5, Kind: work.KindService, Command: "api", Typed: "api", Ports: []string{"3000"}, Status: work.StatusActive, Depth: 2, Container: "aaa"},
		{PID: -6, Kind: work.KindService, Command: "web", Typed: "web", Ports: []string{"8080"}, Status: work.StatusActive, Depth: 2, Container: "bbb"},
	}}}
	got = declared.Attach(hand, files, nil)
	want = []string{"SHELL zsh IDLE", " RUN stack · docker compose up ACTIVE", "  SERVICE api ACTIVE", "  SERVICE web ACTIVE", "  SERVICE db DOWN"}
	if !slices.Equal(rows(got[0]), want) {
		t.Errorf("by hand:\n%s\nwant:\n%s", strings.Join(rows(got[0]), "\n"), strings.Join(want, "\n"))
	}
	if e := got[0].Entries[1]; e.Declared != mark || e.PID != 301 {
		t.Errorf("the row started by hand: %+v", e)
	}
}
