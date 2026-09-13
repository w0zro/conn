package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// conn tells the contacts it starts where they are, and the note
// travels as one word on a shell command line: a socket with an
// apostrophe in it must stay inside that word rather than break out of
// it and run as something else. What the note says has to name the
// socket, since a contact told to open a window and not told which
// server would be guessing.
func TestTheAgentIsToldWhereItIs(t *testing.T) {
	const prefix = contactProgram + " --append-system-prompt "
	for _, socket := range []string{"/Users/w0zro/.local/state/conn/tmux.sock", "/tmp/it's here/conn.sock"} {
		got := contactCommand(socket)
		if !strings.HasPrefix(got, prefix) {
			t.Fatalf("aiCommand(%q) = %q", socket, got)
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
	// Resuming a session is the same launch, carrying the id.
	if got := resumeCommand("/s/conn.sock", "abc-123"); got != contactCommand("/s/conn.sock")+" --resume abc-123" {
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
// session in it, with the given lines and modification time.
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

// A session is read for its branch and the last thing it was
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

// A contact says of itself whether it is working; having stopped, it
// stopped for you, so anything its file says other than busy is
// waiting. A file only counts against a pid the table still has status
// as a contact, and a contact with no file to read says neither.
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
	contact := func(pid int) process {
		return process{pid: pid, uid: 501, tty: "ttys001", state: 'S', command: "claude", args: []string{"claude"},
			started: processesNow.Add(-time.Hour), cwd: "/w"}
	}
	say(10, "busy")    // mid-turn
	say(11, "idle")    // a turn it finished, asking nothing
	say(12, "waiting") // stopped on something it put to you
	say(13, "busy")    // a file its process did not outlive
	say(14, "busy")    // a pid the table has, but not as a contact
	say(15, "")        // a file saying nothing of the sort
	say(17, "sulking") // a word conn has never heard
	say(18, "shell")   // a command running under it, which is work
	procs := []process{contact(10), contact(11), contact(12), contact(15), contact(16), contact(17), contact(18),
		{pid: 14, uid: 501, tty: "ttys001", state: 'S', command: "node", args: []string{"node"},
			started: processesNow.Add(-time.Hour), cwd: "/w"},
	}

	how := contactStatuses(procs)
	for _, pid := range []int{10, 18} {
		if (how[pid] != status{working: true}) {
			t.Errorf("pid %d, mid-turn or running a command, stands %+v", pid, how[pid])
		}
	}
	// Stopped on an ask is not the same as stopped with nothing
	// pending, and only the first is waiting.
	if (how[12] != status{waiting: true}) {
		t.Errorf("a contact stopped on an ask stands %+v", how[12])
	}
	if (how[11] != status{idle: true}) {
		t.Errorf("a contact whose turn is over stands %+v", how[11])
	}
	// 17 says a word conn does not know, which leaves it exactly where
	// a contact with no file at all is: nothing said of it.
	for _, pid := range []int{13, 14, 15, 16, 17} {
		if (how[pid] != status{}) {
			t.Errorf("pid %d stands %+v, and nothing should be said of it", pid, how[pid])
		}
	}

	// And the word the processes view writes for each, end to end.
	got := map[int]string{}
	for _, pl := range projectsFrom(procs, 501, func(string) string { return "/w" }, func(string) bool { return true }, how) {
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
		16: statusActive,  // a contact with nothing to say of itself
	} {
		if got[pid] != want {
			t.Errorf("the processes view writes %s for pid %d, want %s", got[pid], pid, want)
		}
	}
}

// A session a live instance is carrying is not offered, whether or
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

	projects := []project{{path: dir, entries: []entry{
		{pid: 111, kind: kindContact},
		{pid: 999, kind: kindShell}, // 999 is running, but not as a contact
	}}}

	cs := claudeSuspended([]string{dir}, projects)
	if len(cs) != 1 || cs[0].ID != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("claudeSuspended = %+v, want only the one not vouched for as live", cs)
	}
}

// A contact says when its status became what it is, and that is the
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
	contact := func(pid int) process {
		return process{pid: pid, uid: 501, tty: "ttys001", state: 'S', command: "claude", args: []string{"claude"},
			started: processesNow.Add(-time.Hour), cwd: "/w"}
	}

	how := contactStatuses([]process{contact(20), contact(21)})
	if !how[20].waiting || !how[20].since.Equal(since) {
		t.Errorf("a contact that says when it stopped stands %+v, want waiting since %v", how[20], since)
	}
	if !how[21].waiting || !how[21].since.IsZero() {
		t.Errorf("a contact that says nothing of when stands %+v, and the moment should be unsaid", how[21])
	}

	// And the entry carries it, which is what orders the round.
	for _, pl := range projectsFrom([]process{contact(20), contact(21)}, 501, func(string) string { return "/w" }, func(string) bool { return true }, how) {
		for _, e := range pl.entries {
			if e.pid == 20 && !e.since.Equal(since) {
				t.Errorf("the entry for pid 20 stands since %v, want %v", e.since, since)
			}
		}
	}
}

// The ask is read off the end of the transcript: the tool use of the
// last turn that has no result yet, with what it asked for, and with
// nothing pending the last thing the contact said. A sidechain is
// somebody else's turn and is passed over.
func TestReadAskFindsWhatTheAgentIsWaitingOn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.jsonl")
	lines := []string{
		`{"type":"user","message":{"content":"do the thing"}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"Running it."},{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"rm -rf build","description":"Delete the build directory"}}]}}`,
	}
	os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	if a := readAsk(path); a.Tool != "Bash" || a.Detail != "Delete the build directory" || a.Said != "" {
		t.Errorf("pending tool use: %+v", a)
	}
	// Answered, and the turn ends on a question in prose.
	lines = append(lines,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}}`,
		`{"type":"assistant","isSidechain":true,"message":{"content":[{"type":"tool_use","id":"t9","name":"Read","input":{"file_path":"/x"}}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"Done.  Shall I\ncommit it?"}]}}`,
	)
	os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	if a := readAsk(path); a.Tool != "" || a.Said != "Done. Shall I commit it?" {
		t.Errorf("turn ended on a question: %+v", a)
	}
	// A turn's text and its tool use are separate records, and the ask
	// is found across them; a question tool carries its question.
	lines = append(lines,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"One thing to settle."}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t2","name":"AskUserQuestion","input":{"questions":[{"question":"Which one?","header":"Pick"}]}}]}}`)
	os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	if a := readAsk(path); a.String() != "AskUserQuestion · Which one?" {
		t.Errorf("a question tool: %+v", a)
	}
	// Answered, and the reading stops at the prompt that began the turn
	// rather than reaching an older turn's words.
	lines = append(lines,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t2","content":"the first"}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t3","name":"Read","input":{"file_path":"/a"}}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t3","content":"..."}]}}`)
	os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	if a := readAsk(path); a.Tool != "" || a.Said != "One thing to settle." {
		t.Errorf("everything answered: %+v", a)
	}
	if a := readAsk(filepath.Join(dir, "none.jsonl")); a != (ask{}) {
		t.Errorf("no transcript: %+v", a)
	}
}

// A session file is named by pid and outlives its process, and a pid
// comes round again. The file says when its process began, and is
// believed only of a process that began then; a file too old to say is
// believed only if its status changed after the process began.
func TestASessionFileIsBelievedOnlyOfItsOwnProcess(t *testing.T) {
	// As read off a live machine in Pacific time: Claude writes the
	// moment in UTC, and ps said 09:43:50 for the same process.
	pacific, _ := time.LoadLocation("America/Los_Angeles")
	began := time.Date(2026, 9, 12, 9, 43, 50, 0, pacific)
	f := sessionFile{ProcStart: "Sat Sep 12 16:43:50 2026"}
	if !f.wroteBy(began) || !f.wroteBy(began.Add(time.Second)) {
		t.Error("the file's own process is not believed")
	}
	if f.wroteBy(began.Add(time.Hour)) {
		t.Error("a later process with the same pid is believed")
	}
	old := sessionFile{StatusUpdatedAt: began.Add(-time.Minute).UnixMilli()}
	if old.wroteBy(began) {
		t.Error("a status from before the process began is believed")
	}
	old.StatusUpdatedAt = began.Add(time.Minute).UnixMilli()
	if !old.wroteBy(began) {
		t.Error("a status from after the process began is not believed")
	}
	if !(sessionFile{}).wroteBy(began) || !f.wroteBy(time.Time{}) {
		t.Error("with nothing to compare, the file is not believed")
	}
}

// The activity column says a tool call as a verb and an object: a file
// by its base name, a command by itself, a question not at all.
func TestDoingWordIsAVerbAndAnObject(t *testing.T) {
	raw := func(s string) map[string]json.RawMessage {
		var m map[string]json.RawMessage
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	for _, c := range []struct{ name, input, want string }{
		{"Read", `{"file_path":"/w/conn/tui.go"}`, "read tui.go"},
		{"Edit", `{"file_path":"/w/conn/tui.go","old_string":"a"}`, "edit tui.go"},
		{"Write", `{"file_path":"/w/x.md","content":"..."}`, "write x.md"},
		{"Bash", `{"command":"go test ./...","description":"Run the suite"}`, "go test ./..."},
		{"Grep", `{"pattern":"since"}`, "grep since"},
		{"Agent", `{"description":"Find the callers","prompt":"..."}`, "agent Find the callers"},
		{"AskUserQuestion", `{"questions":[{"question":"Which?"}]}`, ""},
		{"Unheard", `{"prompt":"do it"}`, "unheard do it"},
	} {
		if got := doingWord(c.name, raw(c.input)); got != c.want {
			t.Errorf("doingWord(%s) = %q, want %q", c.name, got, c.want)
		}
	}
}

// A working contact's row says the tool it has in flight, read off its
// transcript; an idle one says nothing of the sort. The transcript is
// read again only when the file has changed.
func TestActivitiesReadWhatAWorkingContactIsDoing(t *testing.T) {
	claude := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claude)
	dir := "/w/conn"
	if err := os.MkdirAll(filepath.Join(claude, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(claude, "projects", encodePath(dir)), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, pid := range []int{10, 11} {
		body := `{"pid":` + strconv.Itoa(pid) + `,"sessionId":"s-` + strconv.Itoa(pid) + `","status":"busy","cwd":"` + dir + `"}`
		if err := os.WriteFile(filepath.Join(claude, "sessions", strconv.Itoa(pid)+".json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		lines := `{"type":"user","message":{"content":"do the thing"}}
{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"go test ./..."}}]}}
`
		if err := os.WriteFile(sessionPath(dir, "s-"+strconv.Itoa(pid)), []byte(lines), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	projects := []project{{path: dir, entries: []entry{
		{pid: 10, kind: kindContact, status: statusWorking, cwd: dir},
		{pid: 11, kind: kindContact, status: statusIdle, cwd: dir},
	}}}
	was := activities(projects, nil)
	if got := projects[0].entries[0].doing; got != "go test ./..." {
		t.Errorf("the working contact is doing %q", got)
	}
	if got := projects[0].entries[1].doing; got != "" {
		t.Errorf("the idle contact is doing %q", got)
	}
	if len(was) != 1 {
		t.Errorf("%d transcripts were held, not the working one alone", len(was))
	}
	// The same file is not read again: the word held is answered.
	for path := range was {
		seen := was[path]
		seen.word = "held"
		was[path] = seen
	}
	activities(projects, was)
	if got := projects[0].entries[0].doing; got != "held" {
		t.Errorf("an unchanged transcript was read again: %q", got)
	}
}
