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
// process, and one with no terminal working at / — no project, so
// nothing adopts it and the processes view leaves it out.
var (
	processesNow = time.Date(2026, 9, 9, 3, 0, 0, 0, time.UTC)
	testProcs    = []process{
		{pid: 1, ppid: 0, uid: 0, command: "launchd", started: processesNow.Add(-5 * 24 * time.Hour)},
		{pid: 500, ppid: 1, uid: 501, command: "distnoted", state: 'S', started: processesNow.Add(-4 * 24 * time.Hour), cwd: "/"},
		{pid: 67031, ppid: 1, uid: 501, tty: "ttys004", state: 'S', command: "zsh", args: []string{"-zsh"}, started: processesNow.Add(-3 * time.Hour), cwd: "/Users/w0zro/projects/w0zro/conn"},
		{pid: 67032, ppid: 67031, uid: 501, tty: "ttys004", foreground: true, state: 'S', command: "conn", args: []string{"./conn"}, started: processesNow.Add(-90 * time.Second), cwd: "/Users/w0zro/projects/w0zro/conn"},
		{pid: 67033, ppid: 67032, uid: 501, tty: "ttys004", state: 'S', command: "tmux", args: []string{"tmux", "-S", "/Users/w0zro/.local/state/conn/sock", "attach"}, started: processesNow.Add(-89 * time.Second), cwd: "/Users/w0zro/projects/w0zro/conn"},
		{pid: 67040, ppid: 67031, uid: 501, tty: "ttys005", state: 'S', command: "zsh", args: []string{"-zsh"}, started: processesNow.Add(-90 * time.Second), cwd: "/Users/w0zro/projects/w0zro/conn"},
		{pid: 70001, ppid: 1, uid: 501, tty: "ttys007", state: 'S', command: "zsh", args: []string{"-zsh"}, started: processesNow.Add(-2 * time.Hour), cwd: "/Users/w0zro/projects/w0zro/vim.pro/conjurer"},
		{pid: 70100, ppid: 70001, uid: 501, tty: "ttys007", foreground: true, state: 'S', command: "claude", args: []string{"claude", "--resume"}, started: processesNow.Add(-47 * time.Minute), cwd: "/Users/w0zro/projects/w0zro/vim.pro/conjurer"},
		{pid: 70212, ppid: 70100, uid: 501, tty: "ttys007", state: 'S', command: "node", args: []string{"node", "/opt/claude/mcp.js"}, started: processesNow.Add(-46 * time.Minute), cwd: "/Users/w0zro/projects/w0zro/vim.pro/conjurer"},
		{pid: 70300, ppid: 70100, uid: 501, tty: "ttys007", state: 'S', command: "bash", args: []string{"bash", "-c", "go test ./..."}, started: processesNow.Add(-12 * time.Second), cwd: "/Users/w0zro/projects/w0zro/vim.pro/conjurer/internal"},
		{pid: 70301, ppid: 70300, uid: 501, tty: "ttys007", state: 'R', command: "go", args: []string{"go", "test", "./..."}, started: processesNow.Add(-11 * time.Second), cwd: "/Users/w0zro/projects/w0zro/vim.pro/conjurer/internal"},
		{pid: 80001, ppid: 1, uid: 501, tty: "ttys009", foreground: true, state: 'S', command: "zsh", args: []string{"-zsh"}, started: processesNow.Add(-26 * time.Hour), cwd: "/Users/w0zro"},
		{pid: 80002, ppid: 80001, uid: 501, tty: "ttys009", state: 'T', command: "vim", args: []string{"vim", "notes.md"}, started: processesNow.Add(-25 * time.Hour), cwd: "/Users/w0zro"},
		{pid: 90000, ppid: 1, uid: 502, tty: "ttys011", state: 'S', command: "zsh", args: []string{"-zsh"}, started: processesNow.Add(-time.Hour), cwd: "/Users/other"},
	}
	testRoots = func(dir string) string {
		for _, root := range []string{"/Users/w0zro/projects/w0zro/conn", "/Users/w0zro/projects/w0zro/vim.pro/conjurer"} {
			if dir == root || strings.HasPrefix(dir, root+"/") {
				return root
			}
		}
		return dir
	}
	// The two repositories are projects; home is a directory work
	// happens in and nothing more, which is what keeps it from
	// adopting the machine.
	testIsProject = func(dir string) bool {
		return dir == "/Users/w0zro/projects/w0zro/conn" || dir == "/Users/w0zro/projects/w0zro/vim.pro/conjurer"
	}
	// Where the checkouts are kept, which the processes view names its
	// projects against.
	testProjRoots = []string{"/Users/w0zro/projects"}
)

// The processes view stands every process for its own work, nested
// under whatever runs it: claude's node and its bash, the bash's own
// go, a shell over its idle sibling, another over its stopped vim.
// Everything sits where it started, oldest first — the projects by the
// work that began there, the trees by their own roots, a row among its
// siblings by itself. Nothing of root's, of another user's, without a
// terminal, or conn's own — conn is the instrument and not the work,
// though something under it, however unlikely, would still root a tree
// of its own.
func TestProcessesStandsOneProcessForEachWork(t *testing.T) {
	projects := projectsFrom(testProcs, 501, testRoots, testIsProject, nil)
	var got []string
	for _, pl := range projects {
		for _, e := range pl.entries {
			got = append(got, strings.Repeat(" ", e.depth)+pl.path+" "+e.kind+" "+e.command+" "+e.status)
		}
	}
	// Home's shell is a day old and conjurer's two hours; conn's own
	// project is left with the shell on ttys005, ninety seconds old,
	// because the three-hour shell on ttys004 is the one conn was
	// started from and is conn's own lineage rather than work. Under
	// claude the node it started forty-six minutes ago comes before the
	// bash it started twelve seconds ago.
	want := []string{
		"/Users/w0zro SHELL zsh ACTIVE",
		" /Users/w0zro EDITOR vim notes.md STOPPED",
		"/Users/w0zro/projects/w0zro/vim.pro/conjurer SHELL zsh ACTIVE",
		" /Users/w0zro/projects/w0zro/vim.pro/conjurer CONTACT claude --resume ACTIVE",
		"  /Users/w0zro/projects/w0zro/vim.pro/conjurer RUN node /opt/claude/mcp.js ACTIVE",
		"  /Users/w0zro/projects/w0zro/vim.pro/conjurer SHELL bash -c go test ./... ACTIVE",
		"   /Users/w0zro/projects/w0zro/vim.pro/conjurer RUN go test ./... ACTIVE",
		"/Users/w0zro/projects/w0zro/conn SHELL zsh IDLE",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("processes:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	find := func(pid int) entry {
		for _, pl := range projects {
			for _, e := range pl.entries {
				if e.pid == pid {
					return e
				}
			}
		}
		t.Fatalf("pid %d is not in the processes view", pid)
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
	// conn is in the table, at the same directory as its own shell, and is
	// not a row of it — nor is the tmux client it holds, which is conn's
	// own doing and goes off the processes view with it rather than
	// hanging from the shell above conn.
	for _, pl := range projects {
		for _, e := range pl.entries {
			if e.kind == kindConn {
				t.Error("conn is in its own view")
			}
			if e.pid == 67033 {
				t.Errorf("the tmux client conn holds is a row: %+v", e)
			}
		}
	}
	// The go test and the node stand on their own once claude is gone,
	// each a root of its own project's tree; the shell it left is idle.
	var without []process
	for _, p := range testProcs {
		if p.pid != 70100 {
			without = append(without, p)
		}
	}
	got = got[:0]
	for _, pl := range projectsFrom(without, 501, testRoots, testIsProject, nil) {
		for _, e := range pl.entries {
			got = append(got, strings.Repeat(" ", e.depth)+e.kind+" "+e.command+" "+e.status)
		}
	}
	want = []string{
		"SHELL zsh ACTIVE",
		" EDITOR vim notes.md STOPPED",
		"SHELL zsh IDLE",
		"RUN node /opt/claude/mcp.js ACTIVE",
		"SHELL bash -c go test ./... ACTIVE",
		" RUN go test ./... ACTIVE",
		"SHELL zsh IDLE",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("processes without claude:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if b := projectsFrom(nil, 501, testRoots, testIsProject, nil); len(b) != 0 {
		t.Errorf("an empty table gives %+v", b)
	}
}

// Work with no terminal is still work. A server the contact started and
// one that outlived the shell that started it are both of the project,
// and both stand as rows: the first under the contact that runs it, the
// second rooting a tree of its own, since nothing of yours runs it any
// more. What is merely on the machine stays off — a daemon working in
// its own container under home, where a shell happens to sit, is not in
// a project and is nobody's work. Neither is what conn is held in: the
// tmux server has no terminal and works in the repository like anything
// else there, and is off the processes view with conn.
func TestTheProcessesViewAdoptsWorkWithNoTerminal(t *testing.T) {
	const conn = "/Users/w0zro/projects/w0zro/conn"
	procs := []process{
		{pid: 1, ppid: 0, uid: 0, command: "launchd", started: processesNow.Add(-5 * 24 * time.Hour)},
		{pid: 300, ppid: 1, uid: 501, tty: "ttys004", state: 'S', command: "zsh", args: []string{"-zsh"}, started: processesNow.Add(-3 * time.Hour), cwd: conn},
		{pid: 310, ppid: 300, uid: 501, tty: "ttys004", foreground: true, state: 'S', command: "claude", args: []string{"claude"}, started: processesNow.Add(-47 * time.Minute), cwd: conn},
		{pid: 320, ppid: 310, uid: 501, state: 'S', command: "python3", args: []string{"python3", "-m", "http.server", "8000"}, started: processesNow.Add(-30 * time.Second), cwd: conn + "/docs"},
		{pid: 330, ppid: 1, uid: 501, state: 'S', command: "python3", args: []string{"python3", "-m", "http.server", "8137"}, started: processesNow.Add(-26 * time.Hour), cwd: conn + "/docs"},
		{pid: 400, ppid: 1, uid: 501, tty: "ttys009", state: 'S', command: "zsh", args: []string{"-zsh"}, started: processesNow.Add(-2 * time.Hour), cwd: "/Users/w0zro"},
		{pid: 410, ppid: 1, uid: 501, state: 'S', command: "weatherd", args: []string{"weatherd"}, started: processesNow.Add(-5 * time.Hour), cwd: "/Users/w0zro/Library/Containers/com.apple.weather.widget/Data"},
		{pid: 500, ppid: 1, uid: 501, state: 'S', command: "tmux", args: []string{"tmux", "-S", "/Users/w0zro/.local/state/conn/tmux.sock", "new-session"}, started: processesNow.Add(-90 * time.Second), cwd: conn},
		{pid: 510, ppid: 500, uid: 501, tty: "ttys003", state: 'S', command: "conn", args: []string{"conn"}, started: processesNow.Add(-89 * time.Second), cwd: conn},
		{pid: 600, ppid: 1, uid: 502, state: 'S', command: "python3", args: []string{"python3", "-m", "http.server", "9999"}, started: processesNow.Add(-time.Hour), cwd: conn + "/docs"},
	}
	projects := projectsFrom(procs, 501, testRoots, testIsProject, nil)
	var got []string
	for _, pl := range projects {
		for _, e := range pl.entries {
			got = append(got, strings.Repeat(" ", e.depth)+pl.path+" "+e.kind+" "+e.command+" "+e.status)
		}
	}
	// The server that outlived its shell is the oldest thing at the
	// project and stands first; home's shell began after all of it.
	want := []string{
		conn + " RUN python3 -m http.server 8137 ACTIVE",
		conn + " SHELL zsh ACTIVE",
		" " + conn + " CONTACT claude ACTIVE",
		"  " + conn + " RUN python3 -m http.server 8000 ACTIVE",
		"/Users/w0zro SHELL zsh IDLE",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("processes:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	on := map[int]bool{}
	for _, pl := range projects {
		for _, e := range pl.entries {
			on[e.pid] = true
		}
	}
	for _, c := range []struct {
		pid int
		why string
	}{
		{410, "a daemon under home, where no project is"},
		{500, "the tmux server conn is held in"},
		{510, "conn itself"},
		{600, "another user's server in the project"},
	} {
		if on[c.pid] {
			t.Errorf("%s is a row", c.why)
		}
	}
}

// The table is read a process at a time, so what comes back can be of
// two moments: a pid listed twice, or one reused in between, leaving a
// parent that is its own descendant. Either costs a row, not the
// reading — the processes view comes back, without looping and without
// saying the same process twice.
func TestATornTableCostsARowNotTheReading(t *testing.T) {
	dup := []process{
		{pid: 20, ppid: 1, uid: 501, tty: "ttys001", state: 'S', command: "zsh", args: []string{"-zsh"}, started: processesNow.Add(-time.Hour), cwd: "/Users/w0zro"},
		{pid: 21, ppid: 20, uid: 501, tty: "ttys001", state: 'S', command: "go", args: []string{"go", "build"}, started: processesNow.Add(-time.Minute), cwd: "/Users/w0zro"},
		{pid: 21, ppid: 20, uid: 501, tty: "ttys001", state: 'S', command: "go", args: []string{"go", "build"}, started: processesNow.Add(-time.Minute), cwd: "/Users/w0zro"},
	}
	var pids []int
	for _, pl := range projectsFrom(dup, 501, testRoots, testIsProject, nil) {
		for _, e := range pl.entries {
			pids = append(pids, e.pid)
		}
	}
	if !slices.Equal(pids, []int{20, 21}) {
		t.Errorf("a pid listed twice reads as %v", pids)
	}
	cycle := []process{
		{pid: 10, ppid: 11, uid: 501, tty: "ttys001", state: 'S', command: "zsh", args: []string{"-zsh"}, started: processesNow.Add(-time.Hour), cwd: "/Users/w0zro"},
		{pid: 11, ppid: 10, uid: 501, tty: "ttys001", state: 'S', command: "bash", args: []string{"bash"}, started: processesNow.Add(-time.Minute), cwd: "/Users/w0zro"},
		// Two more under one of them, so the ordering of that one's
		// children is something that has to be worked out at all.
		{pid: 13, ppid: 10, uid: 501, tty: "ttys001", state: 'S', command: "go", args: []string{"go", "build"}, started: processesNow.Add(-time.Minute), cwd: "/Users/w0zro"},
		{pid: 14, ppid: 10, uid: 501, tty: "ttys001", state: 'S', command: "vim", args: []string{"vim"}, started: processesNow.Add(-time.Minute), cwd: "/Users/w0zro"},
		{pid: 12, ppid: 1, uid: 501, tty: "ttys002", state: 'S', command: "zsh", args: []string{"-zsh"}, started: processesNow.Add(-time.Hour), cwd: "/Users/w0zro"},
	}
	done := make(chan []project, 1)
	go func() { done <- projectsFrom(cycle, 501, testRoots, testIsProject, nil) }()
	select {
	case projects := <-done:
		// The one process standing clear of the cycle is still read.
		var pids []int
		for _, pl := range projects {
			for _, e := range pl.entries {
				pids = append(pids, e.pid)
			}
		}
		if !slices.Contains(pids, 12) {
			t.Errorf("the process outside the cycle was lost: %v", pids)
		}
		for _, pl := range projects {
			seen := map[int]bool{}
			for _, e := range pl.entries {
				if seen[e.pid] {
					t.Errorf("pid %d is in the processes view twice", e.pid)
				}
				seen[e.pid] = true
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the processes view did not come back from a cycle in the table")
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
	wasAt := processesNow
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
	// A reading that came back with no time between it and the last has
	// nothing to divide by, and says nothing of anything it had before.
	if b := cpuWorking(was, wasAt, procs, wasAt); b[10] || b[12] {
		t.Errorf("no time passed and %v is working", b)
	}
}

// A process born since the last reading is asked against its own life:
// a compiler spawned, worked and gone inside one gap would otherwise
// read as merely alive for the one moment it was ever seen, which is
// most of what a build is made of. Its whole life is inside the gap,
// which is what makes the two the same question.
func TestAProcessBornInTheGapIsAskedAgainstItsOwnLife(t *testing.T) {
	was := map[int]time.Duration{9: 0}
	wasAt := processesNow
	nowAt := wasAt.Add(2 * time.Second)
	fresh := []process{
		// Spawned a third of a second ago and has had a processor for
		// nearly all of it: working.
		{pid: 20, cpu: 300 * time.Millisecond, started: nowAt.Add(-330 * time.Millisecond)},
		// Older than the last reading, which did not have it: there is
		// no span of conn's to ask about, and its life is not one.
		{pid: 21, cpu: time.Hour, started: wasAt.Add(-2 * time.Hour)},
		// Just spawned and has done nothing yet.
		{pid: 22, cpu: 0, started: nowAt.Add(-10 * time.Millisecond)},
		// No start time to speak of, and not called working on the
		// strength of it.
		{pid: 23, cpu: time.Hour},
	}
	busy := cpuWorking(was, wasAt, fresh, nowAt)
	if !busy[20] {
		t.Error("a compiler burning its whole short life is not working")
	}
	for _, pid := range []int{21, 22, 23} {
		if busy[pid] {
			t.Errorf("pid %d is working: %v", pid, busy)
		}
	}
}

// The first reading has nothing behind it to measure against, and says
// nothing is working rather than dividing what a process has spent by a
// life conn was not watching. conn comes up on a machine already
// running: a dev server that compiled for two seconds and has idled
// ever since read as working for the first beat and went quiet on the
// second, which is a reading that lies at the moment the processes view
// is read hardest.
func TestTheFirstReadingCallsNothingWorking(t *testing.T) {
	nowAt := processesNow
	procs := []process{
		// Burned eight seconds of its twelve and has been asleep since.
		{pid: 30, cpu: 8 * time.Second, started: nowAt.Add(-12 * time.Second)},
		// Working right now, and still not said so: there is nothing
		// to say it against.
		{pid: 31, cpu: 300 * time.Millisecond, started: nowAt.Add(-330 * time.Millisecond)},
	}
	if busy := cpuWorking(nil, time.Time{}, procs, nowAt); len(busy) != 0 {
		t.Errorf("with no reading behind it the processes view calls %v working", busy)
	}
}

// The word for a process is working when it is doing something, which
// beats idle and is beaten by a fault: a stopped process is stopped
// whatever it spent before it was. Waiting beats working in turn — a
// contact that says both is one whose file was written between the two,
// and the thing worth saying is that it wants you — and it is no fault,
// since nothing went wrong.
func TestTheWordsRankFaultThenWaitingThenWorking(t *testing.T) {
	var (
		nothing = status{}
		busy    = status{working: true}
		waits   = status{waiting: true}
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
	contact := process{state: 'S'}
	s, fault := statusOf(contact, kindContact, false, waits)
	if s != statusWaiting {
		t.Errorf("a contact waiting on you is %s", s)
	}
	if fault {
		t.Error("a contact waiting on you is a fault")
	}
	// A contact stopped with its turn over holds nothing up, and reads the
	// way anything else at rest does rather than asking for you.
	if s, _ := statusOf(contact, kindContact, false, status{idle: true}); s != statusIdle {
		t.Errorf("a contact with its turn over is %s, not idle", s)
	}
	if s, _ := statusOf(contact, kindContact, false, status{working: true, waiting: true}); s != statusWaiting {
		t.Errorf("a contact that says both is %s, not waiting", s)
	}
	if s, _ := statusOf(process{state: 'T'}, kindContact, false, waits); s != statusStopped {
		t.Error("a stopped contact is not stopped")
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
		{process{command: "node", args: []string{"/usr/local/bin/claude", "--resume"}}, kindContact, "claude --resume"},
		{process{command: "nvim"}, kindEditor, "nvim"},
		{process{command: "conn", args: []string{"/Users/w0zro/.local/bin/conn"}}, kindConn, "conn"},
		{process{command: "go", args: []string{"go", "test", "./..."}}, kindRun, "go test ./..."},
		{process{command: "python3.12"}, kindRun, "python3.12"},
		{process{command: "conn", args: []string{"/usr/local/bin/conn", "hold"}}, kindHold, "conn hold"},
		// A written title: the name is the first word of it, and the
		// whole of it is what the process was started as.
		{process{command: "claude", args: []string{"claude bg-spare", "--bg-spare", "/tmp/1a39b95b.claim.sock"}}, kindContact, "claude bg-spare --bg-spare /tmp/1a39b95b.claim.sock"},
	} {
		if kind, cmd := kindOf(c.p), commandLine(c.p); kind != c.kind || cmd != c.command {
			t.Errorf("%+v: %s %q, want %s %q", c.p, kind, cmd, c.kind, c.command)
		}
	}
	if age(processesNow.Add(-3*24*time.Hour-2*time.Hour), processesNow) != "3D 02H" ||
		age(processesNow.Add(-2*time.Hour-5*time.Minute), processesNow) != "2H 05M" ||
		age(processesNow.Add(-47*time.Minute-9*time.Second), processesNow) != "47M 09S" ||
		age(processesNow.Add(-11*time.Second), processesNow) != "11S" ||
		age(time.Time{}, processesNow) != "" {
		t.Errorf("ages: %q %q %q %q", age(processesNow.Add(-3*24*time.Hour-2*time.Hour), processesNow), age(processesNow.Add(-2*time.Hour-5*time.Minute), processesNow), age(processesNow.Add(-47*time.Minute-9*time.Second), processesNow), age(processesNow.Add(-11*time.Second), processesNow))
	}
}

// rootFinder finds the repository above a directory, and answers the
// same the second time without looking.
func TestRootFinderFindsTheRepository(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	deep := filepath.Join(repo, "a", "b")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	roots := rootFinder(projectDirs(nil))
	if got := roots(deep); got != repo {
		t.Errorf("root of %s is %q", deep, got)
	}
	if got := roots(dir); got != dir {
		t.Errorf("root of a directory outside any project is %q", got)
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

// The processes view sorts by project, and a project is a repository or
// the folder under conn's roots that holds one. Everything below a
// project is in it, whatever it carries: docs in conn, a service with a
// manifest of its own in the monorepo it is part of.
func TestRootFinderSortsByProject(t *testing.T) {
	dir := t.TempDir()
	group := filepath.Join(dir, "w0zro")
	repo := filepath.Join(group, "conn")
	docs := filepath.Join(repo, "docs")
	api := filepath.Join(repo, "services", "api")
	notes := filepath.Join(group, "notes")
	for _, d := range []string{filepath.Join(repo, ".git"), docs, filepath.Join(api, "internal"), notes} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{filepath.Join(repo, "go.mod"), filepath.Join(docs, "package.json"), filepath.Join(api, "package.json")} {
		if err := os.WriteFile(f, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	roots := rootFinder(projectDirs([]string{dir}))

	if got := roots(repo); got != repo {
		t.Errorf("the repository is its own project, not %q", got)
	}
	if got := roots(docs); got != repo {
		t.Errorf("docs works at %q, not in the repository it is part of", got)
	}
	if got := roots(filepath.Join(api, "internal")); got != repo {
		t.Errorf("a service with a manifest of its own works at %q, not in its repository", got)
	}
	// Beside the checkouts rather than inside one: the folder that holds
	// them is the project, since that is what the work there is about.
	if got := roots(notes); got != group {
		t.Errorf("a directory beside the checkouts works at %q, not at the folder holding them", got)
	}
	if got := roots(group); got != group {
		t.Errorf("the folder holding the checkouts is its own project, not %q", got)
	}
	// A root is where the checkouts are kept, not a project — even with
	// one sitting directly in it — so it stands for itself, and a shell
	// there takes in nothing working under the other checkouts.
	if err := os.MkdirAll(filepath.Join(dir, "loose", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := roots(dir); got != dir {
		t.Errorf("the root itself works at %q", got)
	}
	if projectDirs([]string{dir})(dir) {
		t.Error("the root is a project of its own")
	}
}

// The folder rule is held in by conn's roots. Anywhere else a folder
// with a checkout in it is just a folder — otherwise a home directory
// with one repository under it would be a project, and a shell sitting
// there would take in every daemon on the machine.
func TestOnlyTheRootsHoldProjectsOfFolders(t *testing.T) {
	dir := t.TempDir()
	away := t.TempDir()
	repo := filepath.Join(away, "checkouts", "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	is := projectDirs([]string{dir})
	if !is(repo) {
		t.Error("a repository outside the roots is no project")
	}
	if is(filepath.Join(away, "checkouts")) {
		t.Error("a folder of checkouts outside the roots is a project")
	}
	roots := rootFinder(is)
	if got := roots(filepath.Join(away, "checkouts")); got != filepath.Join(away, "checkouts") {
		t.Errorf("it works at %q rather than standing for itself", got)
	}
	if got := roots(filepath.Join(repo, "deep")); got != repo {
		t.Errorf("work in a repository outside the roots is placed at %q", got)
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

// The waiting are answered longest held up first. A contact that cannot
// say when it stopped is waiting all the same, but it cannot claim a
// turn ahead of one that can prove it waited longer, so it goes last;
// two that stopped at the same moment go by pid, so the ring is the
// same ring on every reading.
func TestWaitingRoundIsLongestHeldUpFirst(t *testing.T) {
	at := func(s int) time.Time { return processesNow.Add(time.Duration(-s) * time.Second) }
	projects := []project{
		{path: "/a", entries: []entry{
			{pid: 1, status: statusWorking, since: at(900)},
			{pid: 2, status: statusWaiting, since: at(60)},
			{pid: 3, status: statusIdle, since: at(900)},
		}},
		{path: "/b", entries: []entry{
			{pid: 4, status: statusWaiting}, // says nothing of when
			{pid: 5, status: statusWaiting, since: at(600)},
			{pid: 7, status: statusWaiting, since: at(300)},
			{pid: 6, status: statusWaiting, since: at(300)},
		}},
	}
	var got []int
	for _, e := range waitingRound(projects) {
		got = append(got, e.pid)
	}
	want := []int{5, 6, 7, 2, 4} // 600s, then the two at 300s by pid, then 60s, then the one that cannot say
	if !slices.Equal(got, want) {
		t.Errorf("the waiting round is %v, want %v", got, want)
	}
}

// The list holds still. Work appearing anywhere moves nothing that was
// already there — not the row it hangs under, not that row's siblings,
// not the project: it goes on the end of where it belongs and
// everything above keeps its spot. It was the newest start anywhere in
// a subtree that ordered all three, so a command a contact ran
// re-sorted the processes view out from under whoever was reading it.
func TestTheProcessesViewHoldsItsOrder(t *testing.T) {
	rows := func(procs []process) []int {
		var out []int
		for _, pl := range projectsFrom(procs, 501, testRoots, testIsProject, nil) {
			for _, e := range pl.entries {
				out = append(out, e.pid)
			}
		}
		return out
	}
	before := rows(testProcs)

	// A command under the contact, a shell of its own in the oldest
	// project, and a tree in a project the processes view has never had:
	// each is newer than everything on the list.
	grown := append(append([]process{}, testProcs...),
		process{pid: 70999, ppid: 70100, uid: 501, tty: "ttys007", state: 'R', command: "rg", args: []string{"rg", "conn"},
			started: processesNow.Add(-time.Second), cwd: "/Users/w0zro/projects/w0zro/vim.pro/conjurer"},
		process{pid: 80999, ppid: 1, uid: 501, tty: "ttys012", state: 'S', command: "zsh", args: []string{"-zsh"},
			started: processesNow.Add(-2 * time.Second), cwd: "/Users/w0zro"},
		process{pid: 90999, ppid: 1, uid: 501, tty: "ttys013", state: 'S', command: "zsh", args: []string{"-zsh"},
			started: processesNow.Add(-3 * time.Second), cwd: "/private/tmp/scratch"},
	)
	after := rows(grown)

	// Every row that was there is still there, in the order it was in.
	var kept []int
	was := map[int]bool{}
	for _, pid := range before {
		was[pid] = true
	}
	for _, pid := range after {
		if was[pid] {
			kept = append(kept, pid)
		}
	}
	if !reflect.DeepEqual(kept, before) {
		t.Errorf("the rows that were there moved:\n%v\nwere:\n%v", kept, before)
	}
	// And the new work is on the end of where it belongs: the command
	// under the contact last among what the contact runs, the new shell
	// last in the project it is in, the new project last of all.
	if last := after[len(after)-1]; last != 90999 {
		t.Errorf("a project the processes view has never had stands before the others: last row is %d", last)
	}
	at := func(pid int) int {
		for i, p := range after {
			if p == pid {
				return i
			}
		}
		t.Fatalf("pid %d is not in the processes view", pid)
		return -1
	}
	if at(70999) < at(70301) {
		t.Error("the contact's newest command stands before the ones it started earlier")
	}
	if at(80999) < at(80002) {
		t.Error("a shell opened just now stands before what was already in its project")
	}
}

// The since column says its span in one unit, the largest that
// applies.
func TestBriefIsOneUnit(t *testing.T) {
	for _, c := range []struct {
		d    time.Duration
		want string
	}{
		{3*24*time.Hour + 2*time.Hour, "3D"},
		{2*time.Hour + 59*time.Minute, "2H"},
		{47*time.Minute + 9*time.Second, "47M"},
		{24 * time.Second, "24S"},
		{0, "0S"},
	} {
		if got := brief(c.d); got != c.want {
			t.Errorf("brief(%v) = %q, want %q", c.d, got, c.want)
		}
	}
	if sinceWord(time.Time{}, processesNow) != "" {
		t.Error("a row with no moment got a word")
	}
}

// A row is dated by conn's own eye where it does not date itself: the
// first reading dates nothing, a row whose status changed is dated at
// the reading that saw it change, one born between readings is dated
// from its birth, one standing as it did keeps its date, and a contact
// keeps the moment it says itself. A pid come round again is a new
// process, not the old one's status carried on.
func TestSinceSeenDatesARowByItsOwnEye(t *testing.T) {
	t0 := processesNow
	row := func(pid int, status string, started, since time.Time) entry {
		return entry{pid: pid, status: status, started: started, since: since}
	}
	first := []project{{path: "/w", entries: []entry{
		row(1, statusIdle, t0.Add(-time.Hour), time.Time{}),
		row(2, statusWorking, t0.Add(-time.Hour), t0.Add(-7*time.Minute)),
	}}}
	was := sinceSeen(first, nil, time.Time{}, t0)
	if !first[0].entries[0].since.IsZero() {
		t.Errorf("the first reading dated a shell: %v", first[0].entries[0].since)
	}
	if !first[0].entries[1].since.Equal(t0.Add(-7 * time.Minute)) {
		t.Errorf("a contact lost its own moment: %v", first[0].entries[1].since)
	}

	t1 := t0.Add(2 * time.Second)
	second := []project{{path: "/w", entries: []entry{
		row(1, statusActive, t0.Add(-time.Hour), time.Time{}), // changed
		row(2, statusWorking, t0.Add(-time.Hour), t0.Add(-7*time.Minute)),
		row(3, statusActive, t0.Add(time.Second), time.Time{}), // born between
	}}}
	was = sinceSeen(second, was, t0, t1)
	e := second[0].entries
	if !e[0].since.Equal(t1) {
		t.Errorf("a changed row is dated %v, not the reading that saw it", e[0].since)
	}
	if !e[2].since.Equal(t0.Add(time.Second)) {
		t.Errorf("a row born between readings is dated %v, not its birth", e[2].since)
	}

	t2 := t1.Add(2 * time.Second)
	third := []project{{path: "/w", entries: []entry{
		row(1, statusActive, t0.Add(-time.Hour), time.Time{}),  // as it was
		row(3, statusActive, t1.Add(time.Second), time.Time{}), // the pid come round again
	}}}
	sinceSeen(third, was, t1, t2)
	e = third[0].entries
	if !e[0].since.Equal(t1) {
		t.Errorf("a row standing as it did was redated: %v", e[0].since)
	}
	if !e[1].since.Equal(t1.Add(time.Second)) {
		t.Errorf("a pid come round again took the old process's date: %v", e[1].since)
	}
}

// The row's command is what was typed: the argument conn adds when it
// raises a contact is left off, in either spelling, and the readout's
// line keeps it. The kill's question names the program alone.
func TestTheRowSaysWhatWasTyped(t *testing.T) {
	raised := process{command: "claude", args: []string{"claude", "--append-system-prompt", "You are running inside conn", "--resume", "abc"}}
	if got := typedLine(raised); got != "claude --resume abc" {
		t.Errorf("typedLine = %q", got)
	}
	if got := commandLine(raised); got != "claude --append-system-prompt You are running inside conn --resume abc" {
		t.Errorf("commandLine = %q", got)
	}
	joined := process{command: "claude", args: []string{"claude", "--append-system-prompt=note"}}
	if got := typedLine(joined); got != "claude" {
		t.Errorf("typedLine with the value joined = %q", got)
	}
	if got := program("go test ./..."); got != "go" {
		t.Errorf("program = %q", got)
	}
}

// A name is a contact's when it means an agent and means little else.
// The word says there is a mind at the other end and that the row can
// stop and wait on you, which is more than the other kinds claim.
func TestOnlyAnAgentsNameIsAContacts(t *testing.T) {
	kind := func(args ...string) string {
		return kindOf(process{command: args[0], args: args})
	}
	for _, name := range []string{"claude", "codex", "gemini", "aider", "opencode", "amp", "copilot"} {
		if got := kind(name); got != kindContact {
			t.Errorf("%s: %s, want %s", name, got, kindContact)
		}
	}
	// goose migrates a database at least as often as it agents, and
	// ollama's own processes are a server and a download.
	for _, c := range [][]string{{"goose", "up"}, {"ollama", "serve"}, {"ollama", "run", "llama3"}} {
		if got := kind(c...); got != kindRun {
			t.Errorf("%v: %s, want %s", c, got, kindRun)
		}
	}
}

// conn is not standing on a row of its own. It takes the terminal
// over, so the shell it was started from is blocked behind it for as
// long as it runs, in whatever directory it happened to be in, which
// for most people is home: a shell doing nothing, at a project nobody
// is working in, which cannot be gone to because going to it is what
// conn already is. Whatever stands between that shell and conn goes
// with it — a go run in development, a wrapper script, a login shell.
func TestConnIsNotStandingOnARowOfItsOwn(t *testing.T) {
	// The lineage a development conn actually has: login, the shell,
	// go run, the binary it built, and the client conn holds. Beside it,
	// a job suspended in that shell before conn was started, which is
	// work and is waiting for somebody.
	procs := []process{
		{pid: 100, ppid: 1, uid: 501, tty: "ttys002", state: 'S', command: "login", args: []string{"login", "-flp", "w0zro"}, cwd: "/Users/w0zro"},
		{pid: 101, ppid: 100, uid: 501, tty: "ttys002", state: 'S', command: "zsh", args: []string{"-zsh"}, cwd: "/Users/w0zro"},
		{pid: 102, ppid: 101, uid: 501, tty: "ttys002", state: 'T', command: "vim", args: []string{"vim", "notes.md"}, cwd: "/Users/w0zro"},
		{pid: 103, ppid: 101, uid: 501, tty: "ttys002", state: 'S', command: "go", args: []string{"go", "run", "."}, cwd: "/Users/w0zro/projects/w0zro/conn"},
		{pid: 104, ppid: 103, uid: 501, tty: "ttys002", state: 'S', command: "conn", args: []string{"/tmp/go-build/conn"}, cwd: "/Users/w0zro/projects/w0zro/conn"},
		{pid: 105, ppid: 104, uid: 501, tty: "ttys002", state: 'S', command: "tmux", args: []string{"tmux", "-S", "/x/sock", "attach"}, cwd: "/Users/w0zro/projects/w0zro/conn"},
		// The panel, inside the server, whose parent holds no terminal.
		{pid: 200, ppid: 199, uid: 501, tty: "ttys000", state: 'S', command: "conn", args: []string{"/tmp/go-build/conn"}, cwd: "/Users/w0zro"},
		// And a shell of the operator's own, in the server, which is work.
		{pid: 201, ppid: 199, uid: 501, tty: "ttys003", state: 'S', command: "zsh", args: []string{"-zsh"}, cwd: "/Users/w0zro/projects/w0zro/conn"},
	}
	var got []string
	for _, pl := range projectsFrom(procs, 501, testRoots, testIsProject, nil) {
		for _, e := range pl.entries {
			got = append(got, strings.Repeat(" ", e.depth)+e.kind+" "+e.command+" "+strconv.Itoa(e.pid))
		}
	}
	// The login, the shell, the go run, both conns and the client are
	// all gone. What is left is the suspended editor and the shell in
	// the server. The editor roots itself, its shell having gone.
	want := []string{
		"EDITOR vim notes.md 102",
		"SHELL zsh 201",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rows:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
