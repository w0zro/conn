package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestEncodePathDashesEveryThingThatIsNotAlnum(t *testing.T) {
	if got := encodePath("/Users/w0zro/projects/conn"); got != "-Users-w0zro-projects-conn" {
		t.Errorf("encodePath = %q", got)
	}
}

func TestIsSessionIDAcceptsOnlyHexAndDashes(t *testing.T) {
	cases := map[string]bool{
		"": false, "abc-123": true, "ABCDEF-0": true,
		"claude": false, "../../etc/passwd": false, "a b": false,
	}
	for id, want := range cases {
		if got := isSessionID(id); got != want {
			t.Errorf("isSessionID(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestResumeCommandCarriesTheID(t *testing.T) {
	if got := resumeCommand("abc-123"); got != "claude --resume abc-123" {
		t.Errorf("resumeCommand = %q", got)
	}
}

// writeTranscript makes a project's transcript directory and a
// conversation in it, with the given lines and modification time.
func writeTranscript(t *testing.T, claude, dir, id string, lines []string, when time.Time) {
	t.Helper()
	pdir := filepath.Join(claude, "projects", encodePath(dir))
	if err := os.MkdirAll(pdir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(pdir, id+".jsonl")
	body := ""
	for _, l := range lines {
		body += l + "\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
}

// A conversation is read for its branch and the last thing it was
// asked, oldest matching record losing to a newer one read first, since
// the reader goes backwards from the end.
func TestClaudeSuspendedReadsBranchAndPrompt(t *testing.T) {
	claude := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claude)
	dir := "/Users/w0zro/projects/conn"
	now := time.Now()

	writeTranscript(t, claude, dir, "11111111-1111-1111-1111-111111111111", []string{
		`{"type":"user","isMeta":false,"gitBranch":"main","message":{"content":"fix the flaky test"}}`,
		`{"type":"last-prompt","lastPrompt":"say more about  the   spacing"}`,
	}, now.Add(-time.Hour))
	writeTranscript(t, claude, dir, "22222222-2222-2222-2222-222222222222", []string{
		`{"type":"user","isMeta":false,"gitBranch":"topic","message":{"content":"draft the README"}}`,
	}, now)

	cs := claudeSuspended([]string{dir}, nil)
	if len(cs) != 2 {
		t.Fatalf("claudeSuspended found %d, want 2", len(cs))
	}
	// Newest first.
	if cs[0].ID != "22222222-2222-2222-2222-222222222222" || cs[0].Branch != "topic" || cs[0].Prompt != "draft the README" {
		t.Errorf("newest: %+v", cs[0])
	}
	// last-prompt, read first going backwards, wins over the user record
	// under it, and is flattened onto one line.
	if cs[1].ID != "11111111-1111-1111-1111-111111111111" || cs[1].Branch != "main" || cs[1].Prompt != "say more about the spacing" {
		t.Errorf("oldest: %+v", cs[1])
	}
}

// An agent says of itself whether it is working; having stopped, it
// stopped for you, so anything its file says other than busy is owed.
// A file only counts against a pid the table still has standing as an
// agent, and an agent with no file to read says neither.
func TestAnAgentSaysWorkingOrOwedOfItself(t *testing.T) {
	claude := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claude)
	if err := os.MkdirAll(filepath.Join(claude, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	say := func(pid int, status string) {
		t.Helper()
		body := `{"pid":` + strconv.Itoa(pid) + `,"sessionId":"a-1","status":"` + status + `"}`
		if err := os.WriteFile(filepath.Join(claude, "sessions", strconv.Itoa(pid)+".json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	agent := func(pid int) process {
		return process{pid: pid, uid: 501, tty: "ttys001", state: 'S', command: "claude", args: []string{"claude"},
			started: watchNow.Add(-time.Hour), cwd: "/w"}
	}
	say(10, "busy")    // mid-turn
	say(11, "idle")    // a turn it finished
	say(12, "waiting") // stopped on something it wants from you
	say(13, "busy")    // a file its process did not outlive
	say(14, "busy")    // a pid the table has, but not as an agent
	say(15, "")        // a file saying nothing of the sort
	procs := []process{agent(10), agent(11), agent(12), agent(15), agent(16),
		{pid: 14, uid: 501, tty: "ttys001", state: 'S', command: "node", args: []string{"node"},
			started: watchNow.Add(-time.Hour), cwd: "/w"},
	}

	how := agentStandings(procs)
	if !how[10].working || how[10].owed {
		t.Errorf("a busy agent stands %+v", how[10])
	}
	for _, pid := range []int{11, 12} {
		if !how[pid].owed || how[pid].working {
			t.Errorf("pid %d, having stopped, stands %+v", pid, how[pid])
		}
	}
	for _, pid := range []int{13, 14, 15, 16} {
		if (how[pid] != standing{}) {
			t.Errorf("pid %d stands %+v, and nothing should be said of it", pid, how[pid])
		}
	}

	// And the word the watch writes for each, end to end.
	got := map[int]string{}
	for _, pl := range watch(procs, 501, func(string) string { return "/w" }, how) {
		for _, e := range pl.entries {
			got[e.pid] = e.status
		}
	}
	if got[10] != statusWorking || got[11] != statusOwed || got[12] != statusOwed || got[16] != statusActive {
		t.Errorf("the watch writes %v", got)
	}
}

// A conversation a live instance is carrying is not offered, whether or
// not the session file naming it agrees: a pid the process table does
// not have running claude cannot vouch for it.
func TestClaudeSuspendedExcludesWhatIsLive(t *testing.T) {
	claude := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claude)
	dir := "/Users/w0zro/projects/conn"
	now := time.Now()

	writeTranscript(t, claude, dir, "11111111-1111-1111-1111-111111111111", []string{
		`{"type":"user","isMeta":false,"message":{"content":"still going"}}`,
	}, now)
	writeTranscript(t, claude, dir, "22222222-2222-2222-2222-222222222222", []string{
		`{"type":"user","isMeta":false,"message":{"content":"actually suspended"}}`,
	}, now)

	sessions := filepath.Join(claude, "sessions")
	if err := os.MkdirAll(sessions, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(pid int, id string) {
		body := `{"pid":` + strconv.Itoa(pid) + `,"sessionId":"` + id + `"}`
		if err := os.WriteFile(filepath.Join(sessions, strconv.Itoa(pid)+".json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A live pid vouches for its session; a stale file naming a pid
	// nothing is running as claude does not.
	write(111, "11111111-1111-1111-1111-111111111111")
	write(999, "22222222-2222-2222-2222-222222222222")

	places := []place{{path: dir, entries: []entry{
		{pid: 111, kind: kindAgent},
		{pid: 999, kind: kindShell}, // 999 is running, but not as an agent
	}}}

	cs := claudeSuspended([]string{dir}, places)
	if len(cs) != 1 || cs[0].ID != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("claudeSuspended = %+v, want only the one not vouched for as live", cs)
	}
}
