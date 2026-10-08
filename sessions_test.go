package main

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/w0zro/conn/internal/draw"
	"github.com/w0zro/conn/internal/work"
	"github.com/w0zro/conn/internal/work/claude"

	"github.com/w0zro/conn/internal/theme"

	tea "charm.land/bubbletea/v2"
)

// testSessions2 is a project's suspended sessions as claudeSuspended
// would give them: newest first, one with nothing read of it yet.
var testSessions2 = []claude.Session{
	{ID: "aaaaaaaa-0000-0000-0000-000000000001", Dir: "/Users/w0zro/projects/w0zro/conn", When: processesNow.Add(-2 * time.Hour), Branch: "main", Prompt: "fix the flaky build test"},
	{ID: "bbbbbbbb-0000-0000-0000-000000000002", Dir: "/Users/w0zro/projects/w0zro/conn", When: processesNow.Add(-3 * 24 * time.Hour), Branch: "topic/resume"},
}

func testSessions(filter string) sessionsReport {
	return composeSessions(testSessions2, "/Users/w0zro/projects/w0zro/conn", filter, "/Users/w0zro", []string{"/Users/w0zro/projects"}, processesNow, false)
}

// A session answers the filter by its branch, the last thing it
// was asked, or the directory it was had in.
func TestMatchingSessionsAnswersByBranchPromptOrDir(t *testing.T) {
	for _, c := range []struct {
		filter string
		want   int
	}{
		{"", 2},
		{"flaky", 1},
		{"topic", 1},
		{"CONN", 2},
		{"nothing at all", 0},
	} {
		if got := len(matchingSessions(testSessions2, c.filter)); got != c.want {
			t.Errorf("%q leaves %d, want %d", c.filter, got, c.want)
		}
	}
}

// The sessions view at rest, filtered, loading, and with nothing found
// are files of record.
func TestSessionsMatchesTheGolden(t *testing.T) {
	golden(t, "sessions-48x30.txt", texts(drawSessions(testSessions(""), 0, 48, 30, draw.Plain)))
	golden(t, "sessions-filtered-48x30.txt", texts(drawSessions(testSessions("flaky"), 0, 48, 30, draw.Plain)))
	loading := composeSessions(nil, "/Users/w0zro/projects/w0zro/conn", "", "/Users/w0zro", []string{"/Users/w0zro/projects"}, processesNow, true)
	golden(t, "sessions-loading-48x30.txt", texts(drawSessions(loading, 0, 48, 30, draw.Plain)))
	empty := composeSessions(nil, "/Users/w0zro/projects/w0zro/conn", "", "/Users/w0zro", []string{"/Users/w0zro/projects"}, processesNow, false)
	golden(t, "sessions-empty-48x30.txt", texts(drawSessions(empty, 0, 48, 30, draw.Plain)))
}

// The sessions view's rows hold: the project it is for, the count
// against the right, the filter on its own line, the branch and the
// age, a session with nothing read of it named by its directory
// instead, and the cursor on one row.
func TestSessionsLayOut(t *testing.T) {
	rows := drawSessions(testSessions(""), 1, 48, 30, draw.Plain)
	text := texts(rows)
	for _, s := range []string{
		"SESSIONS", "2 SUSPENDED", "FIND   ▏", "w0zro/conn",
		"main", "fix the flaky", "topic/resume", "2H 00M", "3D 00H",
	} {
		if !strings.Contains(text, s) {
			t.Errorf("the sessions view lacks %q:\n%s", s, text)
		}
	}
	if strings.Count(text, "▸") != 1 {
		t.Errorf("the cursor marks %d rows", strings.Count(text, "▸"))
	}
	for _, r := range drawSessions(testSessions(""), 0, 48, 30, draw.Colored(theme.Conn.Dark)) {
		if w := utf8.RuneCountInString(stripEscapes(r.Text)); w != 48 {
			t.Errorf("a colored row paints %d columns", w)
		}
	}
	b := testSessions("flaky")
	text = texts(drawSessions(b, 0, 48, 30, draw.Plain))
	if !strings.Contains(text, "1 OF 2") {
		t.Errorf("narrowed:\n%s", text)
	}
}

// The recent view is every project's sessions: r opens it from the
// processes view, A still opens the one project's, and the two are not
// the same view. A row is named by the project that holds where it was
// had, by its leaf, and says what the session is called, falling back
// to what it was last asked; the golden holds the drawing.
func TestTheRecentViewIsEveryProjectsSessions(t *testing.T) {
	m := plainModel()
	m.view, m.inside = viewProcesses, true
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m = next.(model)
	if m.view != viewSessions || !m.sessions.recent || cmd == nil {
		t.Fatalf("r: view %v, recent %v, cmd %v", m.view, m.sessions.recent, cmd != nil)
	}

	// An answer for the one project's view is not this view's.
	if m.sessions.landed(sessionsMsg{dirs: []string{"/w/conn"}, sessions: testSessions2}) {
		t.Error("the recent view took a project's sessions")
	}
	recent := []claude.Session{
		{ID: "cccccccc-0000-0000-0000-000000000003", Dir: "/Users/w0zro/projects/w0zro/conn/internal", When: processesNow.Add(-time.Hour), Branch: "main", Title: "Package split", Prompt: "split the packages"},
		{ID: "dddddddd-0000-0000-0000-000000000004", Dir: "/Users/w0zro/projects/rides", When: processesNow.Add(-26 * time.Hour), Prompt: "fix the map"},
	}
	if !m.sessions.landed(sessionsMsg{recent: true, sessions: recent}) {
		t.Fatal("the recent view refused its own answer")
	}
	rootOf := func(dir string) string {
		if strings.HasPrefix(dir, "/Users/w0zro/projects/w0zro/conn") {
			return "/Users/w0zro/projects/w0zro/conn"
		}
		return dir
	}
	b := m.sessions.report("/Users/w0zro", []string{"/Users/w0zro/projects"}, processesNow, rootOf)
	rows := drawSessions(b, 0, 48, 30, draw.Plain)
	golden(t, "sessions-recent-48x30.txt", texts(rows))
	text := texts(rows)
	for _, want := range []string{"RECENT", "conn          Package split", "rides         fix the map"} {
		if !strings.Contains(text, want) {
			t.Errorf("the recent view lacks %q:\n%s", want, text)
		}
	}

	// A, from the processes view, is the project's own view still.
	m = plainModel()
	m.view, m.inside = viewProcesses, true
	m.projects = []work.Project{{Path: "/w/conn", Entries: []work.Entry{{PID: 11, Kind: work.KindShell}}}}
	m.cursor = 11
	next, _ = m.Update(tea.KeyPressMsg{Code: 'A', Text: "A"})
	m = next.(model)
	if m.view != viewSessions || m.sessions.recent || m.sessions.project != "/w/conn" {
		t.Errorf("A: view %v, recent %v, project %q", m.view, m.sessions.recent, m.sessions.project)
	}
}
