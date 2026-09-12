package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// conn tells the agents it starts where they are, and the note travels
// as one word on a shell command line: a socket with an apostrophe in
// it must stay inside that word rather than break out of it and run as
// something else. What the note says has to name the socket, since an
// agent told to open a window and not told which server would be
// guessing.
func TestTheAgentIsToldWhereItIs(t *testing.T) {
	const prefix = agentProgram + " --append-system-prompt "
	for _, socket := range []string{"/Users/w0zro/.local/state/conn/tmux.sock", "/tmp/it's here/conn.sock"} {
		got := agentCommand(socket)
		if !strings.HasPrefix(got, prefix) {
			t.Fatalf("agentCommand(%q) = %q", socket, got)
		}
		word := strings.TrimPrefix(got, prefix)
		if !strings.HasPrefix(word, "'") || !strings.HasSuffix(word, "'") {
			t.Errorf("the note is not one quoted word: %q", word)
		}
		note := strings.ReplaceAll(strings.TrimSuffix(strings.TrimPrefix(word, "'"), "'"), `'\''`, "'")
		if note != insideNote(socket) {
			t.Errorf("the note does not survive quoting:\n%s\nwant:\n%s", note, insideNote(socket))
		}
		for _, want := range []string{socket, "new-window", "capture-pane"} {
			if !strings.Contains(note, want) {
				t.Errorf("the note says nothing of %q:\n%s", want, note)
			}
		}
	}
	// Resuming a conversation is the same launch, carrying the id.
	if got := resumeCommand("/s/conn.sock", "abc-123"); got != agentCommand("/s/conn.sock")+" --resume abc-123" {
		t.Errorf("resumeCommand = %q", got)
	}
}

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
	if got := resumeCommand("/s/conn.sock", "abc-123"); !strings.HasPrefix(got, "claude --append-system-prompt '") || !strings.HasSuffix(got, "' --resume abc-123") {
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
// stopped for you, so anything its file says other than busy is
// waiting.
// A file only counts against a pid the table still has standing as an
// agent, and an agent with no file to read says neither.
func TestAnAgentSaysWorkingOrWaitingOfItself(t *testing.T) {
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
	say(11, "idle")    // a turn it finished, asking nothing
	say(12, "waiting") // stopped on something it put to you
	say(13, "busy")    // a file its process did not outlive
	say(14, "busy")    // a pid the table has, but not as an agent
	say(15, "")        // a file saying nothing of the sort
	say(17, "sulking") // a word conn has never heard
	say(18, "shell")   // a command running under it, which is work
	procs := []process{agent(10), agent(11), agent(12), agent(15), agent(16), agent(17), agent(18),
		{pid: 14, uid: 501, tty: "ttys001", state: 'S', command: "node", args: []string{"node"},
			started: watchNow.Add(-time.Hour), cwd: "/w"},
	}

	how := agentStandings(procs)
	for _, pid := range []int{10, 18} {
		if (how[pid] != standing{working: true}) {
			t.Errorf("pid %d, mid-turn or running a command, stands %+v", pid, how[pid])
		}
	}
	// Stopped on an ask is not the same as stopped with nothing
	// pending, and only the first is waiting.
	if (how[12] != standing{waiting: true}) {
		t.Errorf("an agent stopped on an ask stands %+v", how[12])
	}
	if (how[11] != standing{idle: true}) {
		t.Errorf("an agent whose turn is over stands %+v", how[11])
	}
	// 17 says a word conn does not know, which leaves it exactly where
	// an agent with no file at all is: nothing said of it.
	for _, pid := range []int{13, 14, 15, 16, 17} {
		if (how[pid] != standing{}) {
			t.Errorf("pid %d stands %+v, and nothing should be said of it", pid, how[pid])
		}
	}

	// And the word the watch writes for each, end to end.
	got := map[int]string{}
	for _, pl := range watch(procs, 501, func(string) string { return "/w" }, func(string) bool { return true }, how) {
		for _, e := range pl.entries {
			got[e.pid] = e.status
		}
	}
	for pid, want := range map[int]string{
		10: statusWorking, // mid-turn
		18: statusWorking, // a command running under it
		12: statusWaiting, // stopped on an ask
		11: statusIdle,    // turn over
		17: statusActive,  // a word conn does not know, so nothing is claimed
		16: statusActive,  // an agent with nothing to say of itself
	} {
		if got[pid] != want {
			t.Errorf("the watch writes %s for pid %d, want %s", got[pid], pid, want)
		}
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

// An agent says when its status became what it is, and that is the
// moment a wait is measured from. Claude writes the file on a change
// rather than on a clock, so the stamp holds still between changes and
// is the moment of the change itself. A file that says nothing of when
// leaves it unsaid rather than guessing at now.
func TestAnAgentSaysWhenItCameToStandThatWay(t *testing.T) {
	claude := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claude)
	if err := os.MkdirAll(filepath.Join(claude, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	since := time.UnixMilli(1789152626774)
	write := func(pid int, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(claude, "sessions", strconv.Itoa(pid)+".json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(20, `{"pid":20,"sessionId":"a-1","status":"waiting","statusUpdatedAt":1789152626774}`)
	write(21, `{"pid":21,"sessionId":"a-2","status":"waiting"}`) // says nothing of when
	agent := func(pid int) process {
		return process{pid: pid, uid: 501, tty: "ttys001", state: 'S', command: "claude", args: []string{"claude"},
			started: watchNow.Add(-time.Hour), cwd: "/w"}
	}

	how := agentStandings([]process{agent(20), agent(21)})
	if !how[20].waiting || !how[20].since.Equal(since) {
		t.Errorf("an agent that says when it stopped stands %+v, want waiting since %v", how[20], since)
	}
	if !how[21].waiting || !how[21].since.IsZero() {
		t.Errorf("an agent that says nothing of when stands %+v, and the moment should be unsaid", how[21])
	}

	// And the entry carries it, which is what orders the round.
	for _, pl := range watch([]process{agent(20), agent(21)}, 501, func(string) string { return "/w" }, func(string) bool { return true }, how) {
		for _, e := range pl.entries {
			if e.pid == 20 && !e.since.Equal(since) {
				t.Errorf("the entry for pid 20 stands since %v, want %v", e.since, since)
			}
		}
	}
}
