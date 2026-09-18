package main

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// testSessions2 is a project's suspended sessions as claudeSuspended
// would give them: newest first, one with nothing read of it yet.
var testSessions2 = []session{
	{ID: "aaaaaaaa-0000-0000-0000-000000000001", Dir: "/Users/w0zro/projects/w0zro/conn", When: processesNow.Add(-2 * time.Hour), Branch: "main", Prompt: "fix the flaky build test"},
	{ID: "bbbbbbbb-0000-0000-0000-000000000002", Dir: "/Users/w0zro/projects/w0zro/conn", When: processesNow.Add(-3 * 24 * time.Hour), Branch: "topic/resume"},
}

func testSessions(filter string) sessionsReport {
	return composeSessions(testSessions2, "/Users/w0zro/projects/w0zro/conn", filter, "/Users/w0zro", processesNow, false)
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
	golden(t, "sessions-48x30.txt", texts(drawSessions(testSessions(""), 0, 48, 30, plain)))
	golden(t, "sessions-filtered-48x30.txt", texts(drawSessions(testSessions("flaky"), 0, 48, 30, plain)))
	loading := composeSessions(nil, "/Users/w0zro/projects/w0zro/conn", "", "/Users/w0zro", processesNow, true)
	golden(t, "sessions-loading-48x30.txt", texts(drawSessions(loading, 0, 48, 30, plain)))
	empty := composeSessions(nil, "/Users/w0zro/projects/w0zro/conn", "", "/Users/w0zro", processesNow, false)
	golden(t, "sessions-empty-48x30.txt", texts(drawSessions(empty, 0, 48, 30, plain)))
}

// The sessions view's rows hold: the project it is for, the count
// against the right, the filter on its own line, the branch and the
// age, a session with nothing read of it named by its directory
// instead, and the cursor on one row.
func TestSessionsLayOut(t *testing.T) {
	rows := drawSessions(testSessions(""), 1, 48, 30, plain)
	text := texts(rows)
	for _, s := range []string{
		"SESSIONS", "2 SUSPENDED", "FIND   ▏", "~/projects/w0zro/conn",
		"main", "fix the flaky", "topic/resume", "2H 00M", "3D 00H",
	} {
		if !strings.Contains(text, s) {
			t.Errorf("the sessions view lacks %q:\n%s", s, text)
		}
	}
	if strings.Count(text, "▸") != 1 {
		t.Errorf("the cursor marks %d rows", strings.Count(text, "▸"))
	}
	for _, r := range drawSessions(testSessions(""), 0, 48, 30, colored()) {
		if w := utf8.RuneCountInString(stripEscapes(r.text)); w != 48 {
			t.Errorf("a colored row paints %d columns", w)
		}
	}
	b := testSessions("flaky")
	text = texts(drawSessions(b, 0, 48, 30, plain))
	if !strings.Contains(text, "1 OF 2") {
		t.Errorf("narrowed:\n%s", text)
	}
}
