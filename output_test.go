package main

import (
	"strings"
	"testing"

	"github.com/w0zro/conn/internal/draw"
	"github.com/w0zro/conn/internal/room"
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
	panes := map[string]room.Pane{"ttys001": {ID: "%4", TTY: "ttys001"}, "ttys002": {ID: "%5", TTY: "ttys002"}, "ttys003": {ID: "%6", TTY: "ttys003"}}
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
	golden(t, "output-48x30.txt", texts(drawOutput(testOutput(""), 0, 48, 30, draw.Plain)))
	golden(t, "output-found-48x30.txt", texts(drawOutput(testOutput("error"), 1, 48, 30, draw.Plain)))
	golden(t, "output-none-48x30.txt", texts(drawOutput(testOutput("nowhere"), 0, 48, 30, draw.Plain)))
	loading := composeOutput(outList{project: "/Users/w0zro/projects/web", loading: true}, "web")
	golden(t, "output-reading-48x30.txt", texts(drawOutput(loading, 0, 48, 30, draw.Plain)))
}

func TestSlashOpensTheOutputOverTheRowsProject(t *testing.T) {
	m := model{p: draw.Plain, width: 48, height: 40, view: viewProcesses, inside: true, srv: room.New(&tmux.Server{Tmux: "/nonexistent/tmux"})}
	m.projects = []work.Project{{Path: "/Users/w0zro/projects/web", Entries: []work.Entry{{PID: 41, Kind: work.KindShell, Command: "zsh", TTY: "ttys001", Status: work.StatusIdle}}}}
	m.panes = map[string]room.Pane{"ttys001": {ID: "%4", TTY: "ttys001"}}
	m.cursor = 41
	m, _ = m.key("/")
	if m.view != viewOutput || m.out.project != "/Users/w0zro/projects/web" || !m.out.loading {
		t.Fatalf("/ did not open the output view over the row's project: view %d, project %q", m.view, m.out.project)
	}
	m, _ = m.landedOutput(outMsg{project: m.out.project, panes: testOutPanes()})
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
	// The bay follows the cursor itself; the page has no subject here.
	if s := m.subject(); !s.none() {
		t.Errorf("the subject is %+v, want none", s)
	}
	// A reading for another project is not this view's.
	m, _ = m.landedOutput(outMsg{project: "/elsewhere", panes: nil})
	if len(m.out.panes) != 2 {
		t.Error("another project's panes landed on the view")
	}
	m, _ = m.key("esc")
	if m.view != viewProcesses {
		t.Error("esc did not go back")
	}
	// Opened again over the same project, the view is where it was
	// left; over another, it is a fresh line.
	m, _ = m.key("/")
	if m.out.find.text != "error" || m.out.find.at != 1 {
		t.Errorf("the view forgot its text %q and cursor %d", m.out.find.text, m.out.find.at)
	}
	m, _ = m.key("esc")
	m.projects[0].Path = "/Users/w0zro/projects/other"
	m, _ = m.key("/")
	if m.out.find.text != "" {
		t.Error("another project kept the last one's text")
	}
}

func TestEachSayingIsCountedBackFromTheEnd(t *testing.T) {
	panes := []outPane{{tty: "ttys001", id: "%4", label: "api", lines: []string{
		"aa aa end", // two sayings on one line: the row lands on the first, k 3 and 2 back
		"nothing",
		"aa once", // k 1
		"last aa", // k 0
	}}}
	hits := linesSaying("aa", panes)
	got := []int{}
	for _, h := range hits {
		got = append(got, h.k)
	}
	if len(got) != 3 || got[0] != 0 || got[1] != 1 || got[2] != 3 {
		t.Errorf("the sayings count back from the end as %v, want [0 1 3]", got)
	}
	if n := sayings("aaa", "aa", false); n != 2 {
		t.Errorf("overlapping sayings count as tmux counts them: %d, want 2", n)
	}
	if n := sayings("AA aa", "aa", true); n != 2 {
		t.Errorf("a lower-case search ignores case in the count: %d, want 2", n)
	}
}

func TestTheBayFollowsTheCursorAfterItRests(t *testing.T) {
	m := model{p: draw.Plain, width: 48, height: 40, view: viewOutput, inside: true, srv: room.New(&tmux.Server{Tmux: "/nonexistent/tmux"})}
	m.out = outList{project: "/Users/w0zro/projects/web", panes: testOutPanes()}
	m.panes = map[string]room.Pane{"ttys001": {ID: "%4", TTY: "ttys001"}, "ttys002": {ID: "%5", TTY: "ttys002"}}
	for _, k := range []string{"e", "r", "r", "o", "r"} {
		m, _ = m.key(k)
	}
	// Each letter sets a tick going; only the last one's is answered.
	gen := m.out.previews
	if gen != 5 {
		t.Fatalf("five letters set %d ticks going", gen)
	}
	m, _ = m.update(previewTickMsg{gen: gen - 1})
	if m.out.shown != (outShown{}) {
		t.Error("an earlier tick asked the bay")
	}
	m, cmd := m.update(previewTickMsg{gen: gen})
	want := outShown{tty: "ttys001", k: 0, text: "error"}
	if m.out.shown != want || m.bay.preview != "ttys001" || cmd == nil {
		t.Errorf("the tick asked for %+v with preview %q", m.out.shown, m.bay.preview)
	}
	// Resting on the same match asks nothing more.
	m.out.previews++
	m, cmd = m.update(previewTickMsg{gen: m.out.previews})
	if cmd != nil {
		t.Error("the same match was asked for again")
	}
	// A reading that finds the previewed pane in the bay does not take
	// it for where the operator was.
	m.bay.read("ttys001", room.Pane{ID: "%4", TTY: "ttys001"}, false)
	if m.bay.work == "ttys001" {
		t.Error("a previewed pane was taken for work")
	}
	// Down is the other pane, and leaving clears the preview.
	m, _ = m.key("down")
	m, _ = m.update(previewTickMsg{gen: m.out.previews})
	if m.out.shown.tty != "ttys002" || m.out.shown.k != 0 {
		t.Errorf("down previews %+v", m.out.shown)
	}
	m, _ = m.key("esc")
	if m.bay.preview != "" || m.out.shown != (outShown{}) {
		t.Error("esc left the preview standing")
	}
}

func TestTheOutputViewIsNotOpenedOutsideTheServer(t *testing.T) {
	m := model{p: draw.Plain, width: 48, height: 40, view: viewProcesses}
	m.projects = []work.Project{{Path: "/p", Entries: []work.Entry{{PID: 1, TTY: "ttys001"}}}}
	m.cursor = 1
	m, _ = m.key("/")
	if m.view != viewProcesses {
		t.Error("outside the server there is no pane to read, and / does nothing")
	}
}
