package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A process table on file: two terminals of work on this machine. On
// ttys004, a shell running conn. On ttys007, a shell running claude,
// which runs a node of its own and a bash it asked for, which runs a go
// test. On ttys009, a shell at its prompt, and one stopped vim. A root
// process and one with no terminal, which the watch leaves out.
var (
	watchNow  = time.Date(2026, 9, 9, 3, 0, 0, 0, time.UTC)
	testProcs = []process{
		{pid: 1, ppid: 0, uid: 0, command: "launchd", started: watchNow.Add(-5 * 24 * time.Hour)},
		{pid: 500, ppid: 1, uid: 501, command: "distnoted", state: 'S', started: watchNow.Add(-4 * 24 * time.Hour), cwd: "/"},
		{pid: 67031, ppid: 1, uid: 501, tty: "ttys004", state: 'S', command: "zsh", args: []string{"-zsh"}, started: watchNow.Add(-3 * time.Hour), cwd: "/Users/w0zro/projects/w0zro/conn"},
		{pid: 67032, ppid: 67031, uid: 501, tty: "ttys004", foreground: true, state: 'S', command: "conn", args: []string{"./conn"}, started: watchNow.Add(-90 * time.Second), cwd: "/Users/w0zro/projects/w0zro/conn"},
		{pid: 67033, ppid: 67032, uid: 501, tty: "ttys004", state: 'S', command: "tmux", args: []string{"tmux", "-S", "/Users/w0zro/.local/state/conn/sock", "attach"}, started: watchNow.Add(-89 * time.Second), cwd: "/Users/w0zro/projects/w0zro/conn"},
		{pid: 67040, ppid: 67031, uid: 501, tty: "ttys005", state: 'S', command: "zsh", args: []string{"-zsh"}, started: watchNow.Add(-90 * time.Second), cwd: "/Users/w0zro/projects/w0zro/conn"},
		{pid: 70001, ppid: 1, uid: 501, tty: "ttys007", state: 'S', command: "zsh", args: []string{"-zsh"}, started: watchNow.Add(-2 * time.Hour), cwd: "/Users/w0zro/projects/w0zro/vim.pro/conjurer"},
		{pid: 70100, ppid: 70001, uid: 501, tty: "ttys007", foreground: true, state: 'S', command: "claude", args: []string{"claude", "--resume"}, started: watchNow.Add(-47 * time.Minute), cwd: "/Users/w0zro/projects/w0zro/vim.pro/conjurer"},
		{pid: 70212, ppid: 70100, uid: 501, tty: "ttys007", state: 'S', command: "node", args: []string{"node", "/opt/claude/mcp.js"}, started: watchNow.Add(-46 * time.Minute), cwd: "/Users/w0zro/projects/w0zro/vim.pro/conjurer"},
		{pid: 70300, ppid: 70100, uid: 501, tty: "ttys007", state: 'S', command: "bash", args: []string{"bash", "-c", "go test ./..."}, started: watchNow.Add(-12 * time.Second), cwd: "/Users/w0zro/projects/w0zro/vim.pro/conjurer/internal"},
		{pid: 70301, ppid: 70300, uid: 501, tty: "ttys007", state: 'R', command: "go", args: []string{"go", "test", "./..."}, started: watchNow.Add(-11 * time.Second), cwd: "/Users/w0zro/projects/w0zro/vim.pro/conjurer/internal"},
		{pid: 80001, ppid: 1, uid: 501, tty: "ttys009", foreground: true, state: 'S', command: "zsh", args: []string{"-zsh"}, started: watchNow.Add(-26 * time.Hour), cwd: "/Users/w0zro"},
		{pid: 80002, ppid: 80001, uid: 501, tty: "ttys009", state: 'T', command: "vim", args: []string{"vim", "notes.md"}, started: watchNow.Add(-25 * time.Hour), cwd: "/Users/w0zro"},
		{pid: 90000, ppid: 1, uid: 502, tty: "ttys011", state: 'S', command: "zsh", args: []string{"-zsh"}, started: watchNow.Add(-time.Hour), cwd: "/Users/other"},
	}
	testRoots = func(dir string) string {
		for _, root := range []string{"/Users/w0zro/projects/w0zro/conn", "/Users/w0zro/projects/w0zro/vim.pro/conjurer"} {
			if dir == root || strings.HasPrefix(dir, root+"/") {
				return root
			}
		}
		return dir
	}
)

// The watch stands every process for its own work, nested under
// whatever runs it: claude's node and its bash, the bash's own go, a
// shell over its idle sibling, another over its stopped vim. The
// newest work anywhere in a tree brings it, and its place, to the top.
// Nothing of root's, of another user's, without a terminal, or conn's
// own — conn is the instrument and not the work, though something
// under it, however unlikely, would still root a tree of its own.
func TestWatchStandsOneProcessForEachWork(t *testing.T) {
	places := watch(testProcs, 501, testRoots, nil)
	var got []string
	for _, pl := range places {
		for _, e := range pl.entries {
			got = append(got, strings.Repeat(" ", e.depth)+pl.path+" "+e.kind+" "+e.command+" "+e.status)
		}
	}
	want := []string{
		"/Users/w0zro/projects/w0zro/vim.pro/conjurer SHELL zsh ACTIVE",
		" /Users/w0zro/projects/w0zro/vim.pro/conjurer AGENT claude --resume ACTIVE",
		"  /Users/w0zro/projects/w0zro/vim.pro/conjurer SHELL bash -c go test ./... ACTIVE",
		"   /Users/w0zro/projects/w0zro/vim.pro/conjurer RUN go test ./... ACTIVE",
		"  /Users/w0zro/projects/w0zro/vim.pro/conjurer RUN node /opt/claude/mcp.js ACTIVE",
		"/Users/w0zro/projects/w0zro/conn SHELL zsh ACTIVE",
		" /Users/w0zro/projects/w0zro/conn SHELL zsh IDLE",
		"/Users/w0zro SHELL zsh ACTIVE",
		" /Users/w0zro EDITOR vim notes.md STOPPED",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("watch:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	find := func(pid int) entry {
		for _, pl := range places {
			for _, e := range pl.entries {
				if e.pid == pid {
					return e
				}
			}
		}
		t.Fatalf("pid %d is not on the watch", pid)
		return entry{}
	}
	if e := find(80002); !e.fault {
		t.Error("the stopped vim is not a fault")
	}
	if e := find(80001); e.fault {
		t.Error("the shell over it is a fault")
	}
	// A shell running anything, however deep, is active; bare, idle.
	if e := find(70001); e.status != statusActive {
		t.Errorf("a shell running claude is %s, not active", e.status)
	}
	if e := find(67040); e.status != statusIdle {
		t.Errorf("a bare shell is %s, not idle", e.status)
	}
	// conn is in the table, at the same place as its own shell, and is
	// not a row of it — nor is the tmux client it holds, which is
	// conn's own doing and goes off the watch with it rather than
	// hanging from the shell above conn.
	for _, pl := range places {
		for _, e := range pl.entries {
			if e.kind == kindConn {
				t.Error("conn is on its own watch")
			}
			if e.pid == 67033 {
				t.Errorf("the tmux client conn holds is a row: %+v", e)
			}
		}
	}
	// The go test and the node stand on their own once claude is gone,
	// each a root of its own place's tree; the shell it left is idle.
	var without []process
	for _, p := range testProcs {
		if p.pid != 70100 {
			without = append(without, p)
		}
	}
	got = got[:0]
	for _, pl := range watch(without, 501, testRoots, nil) {
		for _, e := range pl.entries {
			got = append(got, strings.Repeat(" ", e.depth)+e.kind+" "+e.command+" "+e.status)
		}
	}
	want = []string{
		"SHELL bash -c go test ./... ACTIVE",
		" RUN go test ./... ACTIVE",
		"RUN node /opt/claude/mcp.js ACTIVE",
		"SHELL zsh IDLE",
		"SHELL zsh ACTIVE",
		" SHELL zsh IDLE",
		"SHELL zsh ACTIVE",
		" EDITOR vim notes.md STOPPED",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("watch without claude:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if b := watch(nil, 501, testRoots, nil); len(b) != 0 {
		t.Errorf("an empty table gives %+v", b)
	}
}

// The table is read a process at a time, so what comes back can be of
// two moments: a pid listed twice, or one reused in between, leaving a
// parent that is its own descendant. Either costs a row, not the
// reading — the watch comes back, without looping and without saying
// the same process twice.
func TestATornTableCostsARowNotTheReading(t *testing.T) {
	dup := []process{
		{pid: 20, ppid: 1, uid: 501, tty: "ttys001", state: 'S', command: "zsh", args: []string{"-zsh"}, started: watchNow.Add(-time.Hour), cwd: "/Users/w0zro"},
		{pid: 21, ppid: 20, uid: 501, tty: "ttys001", state: 'S', command: "go", args: []string{"go", "build"}, started: watchNow.Add(-time.Minute), cwd: "/Users/w0zro"},
		{pid: 21, ppid: 20, uid: 501, tty: "ttys001", state: 'S', command: "go", args: []string{"go", "build"}, started: watchNow.Add(-time.Minute), cwd: "/Users/w0zro"},
	}
	var pids []int
	for _, pl := range watch(dup, 501, testRoots, nil) {
		for _, e := range pl.entries {
			pids = append(pids, e.pid)
		}
	}
	if !slices.Equal(pids, []int{20, 21}) {
		t.Errorf("a pid listed twice reads as %v", pids)
	}
	cycle := []process{
		{pid: 10, ppid: 11, uid: 501, tty: "ttys001", state: 'S', command: "zsh", args: []string{"-zsh"}, started: watchNow.Add(-time.Hour), cwd: "/Users/w0zro"},
		{pid: 11, ppid: 10, uid: 501, tty: "ttys001", state: 'S', command: "bash", args: []string{"bash"}, started: watchNow.Add(-time.Minute), cwd: "/Users/w0zro"},
		// Two more under one of them, so the ordering of that one's
		// children is something that has to be worked out at all.
		{pid: 13, ppid: 10, uid: 501, tty: "ttys001", state: 'S', command: "go", args: []string{"go", "build"}, started: watchNow.Add(-time.Minute), cwd: "/Users/w0zro"},
		{pid: 14, ppid: 10, uid: 501, tty: "ttys001", state: 'S', command: "vim", args: []string{"vim"}, started: watchNow.Add(-time.Minute), cwd: "/Users/w0zro"},
		{pid: 12, ppid: 1, uid: 501, tty: "ttys002", state: 'S', command: "zsh", args: []string{"-zsh"}, started: watchNow.Add(-time.Hour), cwd: "/Users/w0zro"},
	}
	done := make(chan []place, 1)
	go func() { done <- watch(cycle, 501, testRoots, nil) }()
	select {
	case places := <-done:
		// The one process standing clear of the cycle is still read.
		var pids []int
		for _, pl := range places {
			for _, e := range pl.entries {
				pids = append(pids, e.pid)
			}
		}
		if !slices.Contains(pids, 12) {
			t.Errorf("the process outside the cycle was lost: %v", pids)
		}
		for _, pl := range places {
			seen := map[int]bool{}
			for _, e := range pl.entries {
				if seen[e.pid] {
					t.Errorf("pid %d is on the watch twice", e.pid)
				}
				seen[e.pid] = true
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the watch did not come back from a cycle in the table")
	}
}

// ps prints a processor time as minutes and seconds, the minutes
// running past sixty rather than becoming hours; hours and days show up
// on other systems, and all of them read.
func TestPsTimesAreParsed(t *testing.T) {
	for _, c := range []struct {
		in   string
		want time.Duration
		ok   bool
	}{
		{"0:00.00", 0, true},
		{"0:00.39", 390 * time.Millisecond, true},
		{"12:34.56", 12*time.Minute + 34*time.Second + 560*time.Millisecond, true},
		{"583:40.70", 583*time.Minute + 40*time.Second + 700*time.Millisecond, true},
		{"1:02:03", time.Hour + 2*time.Minute + 3*time.Second, true},
		{"2-01:00:00", 49 * time.Hour, true},
		{"nonsense", 0, false},
		{"1:2:3:4", 0, false},
	} {
		got, ok := parsePsTime(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("%q: %v %v, want %v %v", c.in, got, ok, c.want, c.ok)
		}
	}
	times := parsePsTimes("    1  63:35.26\n  333  26:21.76\n\ngarbage line here\n  334   0:00.39\n")
	if len(times) != 3 || times[1] != 63*time.Minute+35*time.Second+260*time.Millisecond || times[334] != 390*time.Millisecond {
		t.Errorf("a listing reads as %v", times)
	}
}

// Work is what a process spent between two readings, not what it has
// spent altogether: a server up for a week has plenty of the second and
// may be doing nothing at all.
func TestWorkIsWhatWasSpentSinceTheLastReading(t *testing.T) {
	was := map[int]time.Duration{
		10: 5 * time.Hour,   // up for ages, and quiet since
		11: 0,               // fresh, and busy since
		12: time.Second,     // a little, but under the share
		13: 2 * time.Second, // gone by the next reading
	}
	wasAt := watchNow
	nowAt := wasAt.Add(2 * time.Second)
	procs := []process{
		{pid: 10, cpu: 5 * time.Hour, started: wasAt.Add(-5 * 24 * time.Hour)},
		{pid: 11, cpu: time.Second, started: wasAt.Add(-time.Minute)},
		{pid: 12, cpu: time.Second + 20*time.Millisecond, started: wasAt.Add(-time.Minute)},
	}
	busy := cpuWorking(was, wasAt, procs, nowAt)
	if !busy[11] {
		t.Error("a process that spent a second of two is not working")
	}
	for _, pid := range []int{10, 12, 13} {
		if busy[pid] {
			t.Errorf("pid %d is working", pid)
		}
	}
	// A reading that came back with no time between it and the last
	// falls to each process's own life rather than dividing by nothing.
	if b := cpuWorking(was, wasAt, procs, wasAt); b[10] || b[12] {
		t.Errorf("no time passed and %v is working", b)
	}
}

// A process the last reading did not have is asked against its own
// life: a compiler spawned, worked and gone inside one gap would
// otherwise read as merely alive for the one moment it was ever seen,
// which is most of what a build is made of.
func TestAProcessFirstSeenIsAskedAgainstItsOwnLife(t *testing.T) {
	nowAt := watchNow
	fresh := []process{
		// Spawned a third of a second ago and has had a processor for
		// nearly all of it: working.
		{pid: 20, cpu: 300 * time.Millisecond, started: nowAt.Add(-330 * time.Millisecond)},
		// Up for an hour and has used a second of it: not working.
		{pid: 21, cpu: time.Second, started: nowAt.Add(-time.Hour)},
		// Just spawned and has done nothing yet.
		{pid: 22, cpu: 0, started: nowAt.Add(-10 * time.Millisecond)},
	}
	busy := cpuWorking(nil, time.Time{}, fresh, nowAt)
	if !busy[20] {
		t.Error("a compiler burning its whole short life is not working")
	}
	if busy[21] || busy[22] {
		t.Errorf("working: %v", busy)
	}
	// A process with no start time to speak of is not called working on
	// the strength of it.
	if b := cpuWorking(nil, time.Time{}, []process{{pid: 23, cpu: time.Hour}}, nowAt); b[23] {
		t.Error("a process with no start time reads as working")
	}
}

// The word for a process is working when it is doing something, which
// beats idle and is beaten by a fault: a stopped process is stopped
// whatever it spent before it was. Waiting beats working in turn — an
// agent that says both is one whose file was written between the two,
// and the thing worth saying is that it wants you — and it is no
// fault, since nothing went wrong.
func TestTheWordsRankFaultThenWaitingThenWorking(t *testing.T) {
	var (
		nothing = standing{}
		busy    = standing{working: true}
		waits   = standing{waiting: true}
	)
	shell := process{state: 'S'}
	if s, _ := statusOf(shell, kindShell, false, busy); s != statusWorking {
		t.Errorf("a bare shell doing something is %s", s)
	}
	if s, _ := statusOf(shell, kindShell, false, nothing); s != statusIdle {
		t.Errorf("a bare shell doing nothing is %s", s)
	}
	if s, _ := statusOf(process{state: 'T'}, kindRun, false, busy); s != statusStopped {
		t.Error("a stopped process that was working is not stopped")
	}
	if s, _ := statusOf(process{state: 'S'}, kindRun, true, nothing); s != statusActive {
		t.Errorf("a run doing nothing is %s", s)
	}
	agent := process{state: 'S'}
	s, fault := statusOf(agent, kindAgent, false, waits)
	if s != statusWaiting {
		t.Errorf("an agent waiting on you is %s", s)
	}
	if fault {
		t.Error("an agent waiting on you is a fault")
	}
	if s, _ := statusOf(agent, kindAgent, false, standing{working: true, waiting: true}); s != statusWaiting {
		t.Errorf("an agent that says both is %s, not waiting", s)
	}
	if s, _ := statusOf(process{state: 'T'}, kindAgent, false, waits); s != statusStopped {
		t.Error("a stopped agent is not stopped")
	}
}

// A process is known by the name of its program.
func TestKindsAndCommands(t *testing.T) {
	for _, c := range []struct {
		p       process
		kind    string
		command string
	}{
		{process{command: "zsh", args: []string{"-zsh"}}, kindShell, "zsh"},
		{process{command: "node", args: []string{"/usr/local/bin/claude", "--resume"}}, kindAgent, "claude --resume"},
		{process{command: "nvim"}, kindEditor, "nvim"},
		{process{command: "conn", args: []string{"/Users/w0zro/.local/bin/conn"}}, kindConn, "conn"},
		{process{command: "go", args: []string{"go", "test", "./..."}}, kindRun, "go test ./..."},
		{process{command: "python3.12"}, kindRun, "python3.12"},
		{process{command: "conn", args: []string{"/usr/local/bin/conn", "hold"}}, kindHold, "conn hold"},
		// A written title: the name is the first word of it, and the
		// whole of it is what the process was started as.
		{process{command: "claude", args: []string{"claude bg-spare", "--bg-spare", "/tmp/1a39b95b.claim.sock"}}, kindAgent, "claude bg-spare --bg-spare /tmp/1a39b95b.claim.sock"},
	} {
		if kind, cmd := kindOf(c.p), commandLine(c.p); kind != c.kind || cmd != c.command {
			t.Errorf("%+v: %s %q, want %s %q", c.p, kind, cmd, c.kind, c.command)
		}
	}
	if age(watchNow.Add(-3*24*time.Hour-2*time.Hour), watchNow) != "3D 02H" ||
		age(watchNow.Add(-2*time.Hour-5*time.Minute), watchNow) != "2H 05M" ||
		age(watchNow.Add(-47*time.Minute-9*time.Second), watchNow) != "47M 09S" ||
		age(watchNow.Add(-11*time.Second), watchNow) != "11S" ||
		age(time.Time{}, watchNow) != "" {
		t.Errorf("ages: %q %q %q %q", age(watchNow.Add(-3*24*time.Hour-2*time.Hour), watchNow), age(watchNow.Add(-2*time.Hour-5*time.Minute), watchNow), age(watchNow.Add(-47*time.Minute-9*time.Second), watchNow), age(watchNow.Add(-11*time.Second), watchNow))
	}
}

// placeRoots finds the .git above a directory, and answers the same the
// second time without looking.
func TestPlaceRootsFindTheRepository(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	deep := filepath.Join(repo, "a", "b")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	roots := placeRoots()
	if got := roots(deep); got != repo {
		t.Errorf("root of %s is %q", deep, got)
	}
	if got := roots(dir); got != dir {
		t.Errorf("root of a directory outside any repository is %q", got)
	}
	if err := os.RemoveAll(filepath.Join(repo, ".git")); err != nil {
		t.Fatal(err)
	}
	if got := roots(deep); got != repo {
		t.Errorf("the root was not remembered: %q", got)
	}
	if roots("") != "" {
		t.Error("no directory has a root")
	}
}

// A manifest between the work and its repository makes the place: the
// monorepo's service is one, the repository's own manifest is not, and
// outside a repository a manifest marks nothing.
func TestPlaceRootsFindTheSubProject(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	api := filepath.Join(repo, "services", "api")
	for _, d := range []string{filepath.Join(repo, ".git"), filepath.Join(api, "internal"), filepath.Join(repo, "cmd", "conn"), filepath.Join(dir, "loose")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{filepath.Join(repo, "go.mod"), filepath.Join(api, "package.json"), filepath.Join(dir, "loose", "go.mod")} {
		if err := os.WriteFile(f, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	roots := placeRoots()
	if got := roots(filepath.Join(api, "internal")); got != api {
		t.Errorf("the place under the service is %q", got)
	}
	if got := roots(api); got != api {
		t.Errorf("the service is its own place, not %q", got)
	}
	if got := roots(filepath.Join(repo, "cmd", "conn")); got != repo {
		t.Errorf("a directory with no manifest above it works at the repository, not %q", got)
	}
	if got := roots(repo); got != repo {
		t.Errorf("the repository's own manifest makes no sub-project: %q", got)
	}
	if got := roots(filepath.Join(dir, "loose")); got != filepath.Join(dir, "loose") {
		t.Errorf("outside a repository a directory stands for itself, not %q", got)
	}
}

// lsof -F pcn, as captured.
func TestLsofIsParsed(t *testing.T) {
	out, err := os.ReadFile("testdata/lsof.txt")
	if err != nil {
		t.Fatal(err)
	}
	got := parseLsof(string(out))
	if len(got) != 5 || got[67032].command != "conn" || got[67032].cwd != "/Users/w0zro/projects/w0zro/conn" || got[409].cwd != "/" {
		t.Errorf("lsof: %+v", got)
	}
	if got := parseLsof(""); len(got) != 0 {
		t.Errorf("nothing parsed as %+v", got)
	}
}

// kern.procargs2, laid out as the kernel lays it.
func TestProcargsAreParsed(t *testing.T) {
	raw := make([]byte, 4)
	binary.LittleEndian.PutUint32(raw, 3)
	raw = append(raw, "/usr/local/bin/go\x00\x00\x00\x00go\x00test\x00./...\x00HOME=/Users/w0zro\x00"...)
	if got := parseProcargs(raw); !reflect.DeepEqual(got, []string{"go", "test", "./..."}) {
		t.Errorf("procargs: %q", got)
	}
	if got := parseProcargs(raw[:3]); got != nil {
		t.Errorf("a short procargs parsed as %q", got)
	}
}

// /proc/<pid>/stat, with a command that holds a space and a parenthesis,
// and a tree of them read off a directory that stands in for /proc.
func TestProcIsParsed(t *testing.T) {
	boot := time.Date(2026, 9, 4, 0, 47, 0, 0, time.UTC)
	line := "70301 (go (test)) R 70300 70300 70001 34823 70300 4194304 1 0 0 0 5 1 0 0 20 0 8 0 43200000 100 200 300"
	p, ok := parseProcStat(line, boot, 100)
	// utime and stime are fields 14 and 15 — 5 and 1 here — and at a
	// hundred ticks a second that is sixty milliseconds on a processor.
	want := process{pid: 70301, command: "go (test)", state: 'R', ppid: 70300, pgid: 70300, tty: "pts/7", foreground: true,
		started: boot.Add(432000 * time.Second), cpu: 60 * time.Millisecond}
	if !ok || !reflect.DeepEqual(p, want) {
		t.Errorf("stat: %+v %v, want %+v", p, ok, want)
	}
	if _, ok := parseProcStat("garbage", boot, 100); ok {
		t.Error("garbage parsed")
	}
	for nr, name := range map[int]string{0: "", 34823: "pts/7", 34816: "pts/0", 35072: "pts/256", 1025: "tty1", 5 << 8: ""} {
		if got := linuxTTY(nr); got != name {
			t.Errorf("tty %d: %q, want %q", nr, got, name)
		}
	}
	if got := parseBootTime("cpu  1 2 3\nbtime " + strconv.FormatInt(boot.Unix(), 10) + "\nprocesses 5\n"); !got.Equal(boot) {
		t.Errorf("btime: %v", got)
	}

	root := t.TempDir()
	write := func(pid, name, content string) {
		dir := filepath.Join(root, pid)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("70301", "stat", line+"\n")
	write("70301", "cmdline", "go\x00test\x00./...\x00")
	if err := os.Symlink("/home/w0zro/conn", filepath.Join(root, "70301", "cwd")); err != nil {
		t.Fatal(err)
	}
	write("70302", "stat", "broken\n")
	write("notapid", "stat", line)
	procs := readProcTree(root, boot, 100)
	if len(procs) != 1 || procs[0].cwd != "/home/w0zro/conn" || !reflect.DeepEqual(procs[0].args, []string{"go", "test", "./..."}) || procs[0].uid != os.Getuid() {
		t.Errorf("proc tree: %+v", procs)
	}
}
