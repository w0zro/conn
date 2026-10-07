package main

import (
	"strings"
	"testing"
	"time"

	"github.com/w0zro/conn/internal/draw"
	"github.com/w0zro/conn/internal/work"
)

// The log view's file of record: a day's lines under TODAY, the day
// before under YESTERDAY, and older ones under their date; the lines
// written since the view was last opened in the ink; a wait and a
// fault stamped; and a line whose row is still on the panel in bold.
var logNow = time.Date(2026, 10, 3, 14, 30, 0, 0, time.Local)

func testLogEvents() []work.Event {
	conn, web := "/Users/w0zro/projects/w0zro/conn", "/Users/w0zro/projects/web"
	day := func(d int, h, m int) time.Time {
		return time.Date(2026, 10, 3-d, h, m, 0, 0, time.Local)
	}
	return []work.Event{
		{At: day(3, 9, 12), Project: web, Label: "worker", PID: 40, Word: work.StatusDown},
		{At: day(1, 18, 2), Project: conn, Label: "the station log", PID: 10, Word: work.StatusWaiting, Note: "Allow Bash: go test ./... in conn?"},
		{At: day(1, 18, 9), Project: conn, Label: "the station log", PID: 10, Word: work.StatusIdle, Note: "waited 7 min"},
		{At: day(1, 23, 58), Project: web, Label: "zsh", PID: 31, Word: work.LogGone},
		{At: day(0, 9, 4), Project: web, Label: "api", PID: 41, Word: work.StatusActive},
		{At: day(0, 11, 41), Project: web, Label: "go test ./...", PID: 42, Word: "EXIT 1", Note: "ran 2 min"},
		{At: day(0, 14, 2), Project: conn, Label: "the station log", PID: 10, Word: work.StatusWaiting, Note: "Allow Edit: log.go?"},
		{At: day(0, 14, 9), Project: conn, Label: "the station log", PID: 10, Word: work.StatusIdle, Note: "took 6 min"},
	}
}

func testLog(fresh int) logReport {
	alive := func(e work.Event) bool { return e.PID == 10 }
	return composeLog(testLogEvents(), fresh, alive, []string{"/Users/w0zro/projects"}, "/Users/w0zro", logNow)
}

func TestTheLogViewMatchesTheGolden(t *testing.T) {
	golden(t, "log-48x30.txt", texts(drawLog(testLog(2), 0, 48, 30, draw.Plain)))
	golden(t, "log-cursor-48x30.txt", texts(drawLog(testLog(0), 5, 48, 30, draw.Plain)))
	empty := composeLog(nil, 0, nil, nil, "/Users/w0zro", logNow)
	golden(t, "log-empty-48x30.txt", texts(drawLog(empty, 0, 48, 30, draw.Plain)))
}

func TestTheLogIsNewestFirstUnderItsDays(t *testing.T) {
	b := testLog(2)
	if len(b.rows) != 8 || b.rows[0].label != "the station log" || b.rows[0].word != work.StatusIdle || b.rows[7].word != work.StatusDown {
		t.Fatalf("the rows are not newest first: %+v", b.rows)
	}
	if !b.rows[0].fresh || !b.rows[1].fresh || b.rows[2].fresh {
		t.Error("the two newest lines are the new ones")
	}
	if !b.rows[0].alive || b.rows[2].alive {
		t.Error("a line is alive by its row being on the panel")
	}
	if b.rows[0].project != "w0zro/conn" || b.rows[2].project != "web" {
		t.Errorf("the project is named as the panel names a block: %q, %q", b.rows[0].project, b.rows[2].project)
	}
	days := []string{}
	for _, r := range b.rows {
		if d := logDay(r.at, b.now); len(days) == 0 || days[len(days)-1] != d {
			days = append(days, d)
		}
	}
	if strings.Join(days, " ") != "TODAY YESTERDAY 30 SEP" {
		t.Errorf("the days read %v", days)
	}
}

func TestLOpensTheLogAndLAgainCloses(t *testing.T) {
	m := model{p: draw.Plain, width: 48, height: 40, view: viewProcesses}
	m.log.unseen = 3
	m, _ = m.key("l")
	if m.view != viewLog {
		t.Fatal("l did not open the log")
	}
	// Opening it is seeing what the band counted: the count is cleared,
	// and the lines it counted are the new ones.
	if m.log.unseen != 0 || m.log.fresh != 3 {
		t.Errorf("the count is %d and the new lines %d after opening", m.log.unseen, m.log.fresh)
	}
	m = m.landedLog(logMsg{testLogEvents()})
	if len(m.log.read) != 8 {
		t.Fatalf("the file landed as %d lines", len(m.log.read))
	}
	m, _ = m.key("j")
	m, _ = m.key("j")
	if e, ok := m.log.logAt(); !ok || e.Word != "EXIT 1" {
		t.Errorf("two down from the newest is %v", e)
	}
	m, _ = m.key("G")
	if e, _ := m.log.logAt(); e.Word != work.StatusDown {
		t.Errorf("G is the oldest line, got %v", e)
	}
	m, _ = m.key("l")
	if m.view != viewProcesses {
		t.Error("l again did not go back")
	}
}

func TestTheLogIsKeptBesideTheSocket(t *testing.T) {
	t.Setenv("CONN_SOCKET", "/tmp/cs-x/sock")
	if got := logPath("/Users/w0zro"); got != "/tmp/cs-x/log" {
		t.Errorf("a server on its own socket logs to %q", got)
	}
	t.Setenv("CONN_SOCKET", "")
	t.Setenv("XDG_STATE_HOME", "")
	if got := logPath("/Users/w0zro"); got != "/Users/w0zro/.local/state/conn/log" {
		t.Errorf("the station logs to %q", got)
	}
}

func TestAReadingWritesTheLogAndTheBandCounts(t *testing.T) {
	began := logNow.Add(-time.Hour)
	first := []work.Project{{Path: "/Users/w0zro/projects/web", Entries: []work.Entry{
		{PID: 10, Kind: work.KindContact, Command: "claude", Title: "the station log", Status: work.StatusWorking, Started: began},
	}}}
	second := []work.Project{{Path: "/Users/w0zro/projects/web", Entries: []work.Entry{
		{PID: 10, Kind: work.KindContact, Command: "claude", Title: "the station log", Status: work.StatusWaiting, Started: began},
	}}}
	m := model{p: draw.Plain, width: 48, height: 40, view: viewProcesses}
	// The first reading is read against nothing.
	m, _ = m.logging(processesMsg{projects: first, tree: first})
	if m.log.unseen != 0 || !m.seenAny {
		t.Fatalf("the first reading counted %d", m.log.unseen)
	}
	m, _ = m.logging(processesMsg{projects: second, tree: second})
	if m.log.unseen != 1 {
		t.Fatalf("the wait counted %d", m.log.unseen)
	}
	// A reading that failed is read against nothing, and the next good
	// one is read against the last good one.
	m, _ = m.logging(processesMsg{err: "THE PROCESS TABLE COULD NOT BE READ"})
	m, _ = m.logging(processesMsg{projects: second, tree: second})
	if m.log.unseen != 1 {
		t.Errorf("a failed reading changed the count to %d", m.log.unseen)
	}
	// The label is what stays: a contact by its title, not what it is
	// doing this moment.
	if got := logLabel(work.Entry{Kind: work.KindContact, Command: "claude", Title: "the station log", Doing: "Read log.go"}); got != "the station log" {
		t.Errorf("a contact is labelled %q", got)
	}
	if got := logLabel(work.Entry{Kind: work.KindRun, Command: "go", Typed: "go test ./..."}); got != "go test ./..." {
		t.Errorf("a run is labelled %q", got)
	}
}
