package main

import (
	"strings"
	"testing"

	"github.com/w0zro/conn/internal/tmux"
	"github.com/w0zro/conn/internal/work"
)

func testOutPanes() []outPane {
	return []outPane{
		{tty: "ttys001", id: "%4", pid: 41, label: "api", history: 3, lines: []string{
			"$ go run ./cmd/api",
			"listening on :8080",
			"GET /health 200",
			"ERROR: dial tcp 127.0.0.1:5432: connection refused",
			"GET /health 500",
			"",
			"",
		}},
		{tty: "ttys002", id: "%5", pid: 42, label: "go test ./...", history: 0, lines: []string{
			"--- FAIL: TestTheRecordPaginates (0.00s)",
			"    main_test.go:99: the first record page holds 25 rows, want 24",
			"FAIL",
			"error: exit status 1",
			"",
		}},
	}
}

func TestTheOutputIsSearchedNewestFirstByPane(t *testing.T) {
	hits := linesSaying("error", testOutPanes())
	got := []string{}
	for _, h := range hits {
		got = append(got, strings.TrimSpace(h.text))
	}
	want := []string{"ERROR: dial tcp 127.0.0.1:5432: connection refused", "error: exit status 1"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("a lower-case search ignores case and reads each pane newest first:\n%v", got)
	}
	// A capital in the text is a search as typed.
	if hits := linesSaying("ERROR", testOutPanes()); len(hits) != 1 || hits[0].pane != 0 || hits[0].at != 0 {
		t.Errorf("a search with a capital is as typed: %v", hits)
	}
	if hits := linesSaying("/health", testOutPanes()); len(hits) != 2 || hits[0].line != 4 || hits[1].line != 2 {
		t.Errorf("the newest match comes first within a pane: %v", hits)
	}
	if linesSaying("", testOutPanes()) != nil {
		t.Error("nothing typed matches nothing")
	}
	if n := written(testOutPanes()[0].lines); n != 5 {
		t.Errorf("the pane has said %d lines, want 5: the screen's blank rows are not its", n)
	}
}

func TestAPaneIsReadOnceForAllItsRows(t *testing.T) {
	projects := []work.Project{{Path: "/Users/w0zro/projects/web", Entries: []work.Entry{
		{PID: 41, Kind: work.KindShell, Command: "zsh", TTY: "ttys001"},
		{PID: 43, Kind: work.KindRun, Command: "go", Typed: "go run ./cmd/api", TTY: "ttys001"},
		{PID: 42, Kind: work.KindRun, Command: "go", Typed: "go test ./...", TTY: "ttys002"},
		{PID: 50, Kind: work.KindService, Command: "postgres", TTY: ""},
	}}, {Path: "/Users/w0zro/projects/other", Entries: []work.Entry{{PID: 60, TTY: "ttys003"}}}}
	panes := map[string]tmux.Pane{"ttys001": {ID: "%4", TTY: "ttys001"}, "ttys002": {ID: "%5", TTY: "ttys002"}, "ttys003": {ID: "%6", TTY: "ttys003"}}
	got := heldPanes(projects, panes, "/Users/w0zro/projects/web")
	if len(got) != 2 || got[0].id != "%4" || got[0].pid != 41 || got[1].id != "%5" || got[1].label != "go test ./..." {
		t.Errorf("the project's held panes are %+v", got)
	}
}

func TestAMatchIsKeptInViewOnALongLine(t *testing.T) {
	text, at := around("short line with a match in it", 18, 40)
	if text != "short line with a match in it" || at != 18 {
		t.Errorf("a line that fits is left alone: %q %d", text, at)
	}
	long := strings.Repeat("x", 60) + "match" + strings.Repeat("y", 20)
	text, at = around(long, 60, 30)
	if !strings.HasPrefix(text, "…xxxxxxxx") || text[at:at+5] != "match" {
		t.Errorf("the start is cut so the match sits a few cells in: %q %d", text, at)
	}
	text, at = around("a match early "+strings.Repeat("z", 60), 2, 30)
	if !strings.HasPrefix(text, "a match early") || at != 2 || len([]rune(text)) > 30 {
		t.Errorf("a match near the start keeps the start: %q %d", text, at)
	}
}

func testOutput(filter string) outputReport {
	l := outList{project: "/Users/w0zro/projects/web", panes: testOutPanes()}
	l.find.set(filter)
	return composeOutput(l, "web")
}

func TestTheOutputViewMatchesTheGolden(t *testing.T) {
	golden(t, "output-48x30.txt", texts(drawOutput(testOutput(""), 0, 48, 30, plain)))
	golden(t, "output-found-48x30.txt", texts(drawOutput(testOutput("error"), 1, 48, 30, plain)))
	golden(t, "output-none-48x30.txt", texts(drawOutput(testOutput("nowhere"), 0, 48, 30, plain)))
	loading := composeOutput(outList{project: "/Users/w0zro/projects/web", loading: true}, "web")
	golden(t, "output-reading-48x30.txt", texts(drawOutput(loading, 0, 48, 30, plain)))
}

func TestSlashOpensTheOutputOverTheRowsProject(t *testing.T) {
	m := model{p: plain, width: 48, height: 40, view: viewProcesses, inside: true, srv: &tmux.Server{Tmux: "/nonexistent/tmux"}}
	m.projects = []work.Project{{Path: "/Users/w0zro/projects/web", Entries: []work.Entry{{PID: 41, Kind: work.KindShell, Command: "zsh", TTY: "ttys001", Status: work.StatusIdle}}}}
	m.panes = map[string]tmux.Pane{"ttys001": {ID: "%4", TTY: "ttys001"}}
	m.cursor = 41
	m, _ = m.key("/")
	if m.view != viewOutput || m.out.project != "/Users/w0zro/projects/web" || !m.out.loading {
		t.Fatalf("/ did not open the output view over the row's project: view %d, project %q", m.view, m.out.project)
	}
	m = m.landedOutput(outMsg{project: m.out.project, panes: testOutPanes()})
	for _, k := range []string{"e", "r", "r", "o", "r"} {
		m, _ = m.key(k)
	}
	if hits := m.out.matches(); len(hits) != 2 {
		t.Fatalf("typing narrowed to %d matches", len(hits))
	}
	m, _ = m.key("down")
	if hit, pane, ok := m.out.outAt(); !ok || pane.label != "go test ./..." || hit.line != 3 {
		t.Errorf("down is the next match: %+v in %q", hit, pane.label)
	}
	// The page is about the match's row.
	if s := m.subject(); s.pid != 42 {
		t.Errorf("the subject is pid %d, want the match's pane's row", s.pid)
	}
	// A reading for another project is not this view's.
	m = m.landedOutput(outMsg{project: "/elsewhere", panes: nil})
	if len(m.out.panes) != 2 {
		t.Error("another project's panes landed on the view")
	}
	m, _ = m.key("esc")
	if m.view != viewProcesses {
		t.Error("esc did not go back")
	}
}

func TestTheOutputViewIsNotOpenedOutsideTheServer(t *testing.T) {
	m := model{p: plain, width: 48, height: 40, view: viewProcesses}
	m.projects = []work.Project{{Path: "/p", Entries: []work.Entry{{PID: 1, TTY: "ttys001"}}}}
	m.cursor = 1
	m, _ = m.key("/")
	if m.view != viewProcesses {
		t.Error("outside the server there is no pane to read, and / does nothing")
	}
}
