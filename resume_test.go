package main

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// testConvos is a place's suspended conversations as claudeSuspended
// would give them: newest first, one with nothing read of it yet.
var testConvos = []conversation{
	{ID: "aaaaaaaa-0000-0000-0000-000000000001", Dir: "/Users/w0zro/projects/w0zro/conn", When: watchNow.Add(-2 * time.Hour), Branch: "main", Prompt: "fix the flaky build test"},
	{ID: "bbbbbbbb-0000-0000-0000-000000000002", Dir: "/Users/w0zro/projects/w0zro/conn", When: watchNow.Add(-3 * 24 * time.Hour), Branch: "topic/resume"},
}

func testResume(filter string) resumeReport {
	return composeResume(testConvos, "/Users/w0zro/projects/w0zro/conn", filter, "/Users/w0zro", watchNow, false)
}

// A conversation answers the filter by its branch, the last thing it
// was asked, or the directory it was had in.
func TestMatchingConvosAnswersByBranchPromptOrDir(t *testing.T) {
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
		if got := len(matchingConvos(testConvos, c.filter)); got != c.want {
			t.Errorf("%q leaves %d, want %d", c.filter, got, c.want)
		}
	}
}

// The picker at rest, filtered, loading, and with nothing found are
// files of record.
func TestResumeMatchesTheGolden(t *testing.T) {
	golden(t, "resume-48x30.txt", texts(drawResume(testResume(""), 0, 48, 30, plain)))
	golden(t, "resume-filtered-48x30.txt", texts(drawResume(testResume("flaky"), 0, 48, 30, plain)))
	loading := composeResume(nil, "/Users/w0zro/projects/w0zro/conn", "", "/Users/w0zro", watchNow, true)
	golden(t, "resume-loading-48x30.txt", texts(drawResume(loading, 0, 48, 30, plain)))
	empty := composeResume(nil, "/Users/w0zro/projects/w0zro/conn", "", "/Users/w0zro", watchNow, false)
	golden(t, "resume-empty-48x30.txt", texts(drawResume(empty, 0, 48, 30, plain)))
}

// The picker's rows hold: the place it is for, the count against the
// right, the filter on its own line, the branch and the age, a
// conversation with nothing read of it named by its directory instead,
// and the cursor on one row.
func TestResumeLayOut(t *testing.T) {
	rows := drawResume(testResume(""), 1, 48, 30, plain)
	text := texts(rows)
	for _, s := range []string{
		"RESUME", "2 SUSPENDED", "FIND  ▏", "~/projects/w0zro/conn",
		"main", "fix the flaky", "topic/resume", "2H 00M", "3D 00H",
	} {
		if !strings.Contains(text, s) {
			t.Errorf("the picker lacks %q:\n%s", s, text)
		}
	}
	if strings.Count(text, "▸") != 1 {
		t.Errorf("the cursor marks %d rows", strings.Count(text, "▸"))
	}
	for _, r := range drawResume(testResume(""), 0, 48, 30, colored()) {
		if w := utf8.RuneCountInString(stripEscapes(r.text)); w != 48 {
			t.Errorf("a colored row paints %d columns", w)
		}
	}
	b := testResume("flaky")
	text = texts(drawResume(b, 0, 48, 30, plain))
	if !strings.Contains(text, "1 OF 2") {
		t.Errorf("narrowed:\n%s", text)
	}
}
