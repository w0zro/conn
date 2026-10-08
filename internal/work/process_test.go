package work

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A process born since the last reading is asked against its own life:
// a compiler spawned, worked and gone inside one gap would otherwise
// read as merely alive for the one moment it was ever seen, which is
// most of what a build is made of. Its whole life is inside the gap,
// which is what makes the two the same question.
func TestAProcessBornInTheGapIsAskedAgainstItsOwnLife(t *testing.T) {
	was := map[int]time.Duration{9: 0}
	wasAt := processesNow
	nowAt := wasAt.Add(2 * time.Second)
	fresh := []Process{
		// Spawned a third of a second ago and has had a processor for
		// nearly all of it: working.
		{PID: 20, CPU: 300 * time.Millisecond, Started: nowAt.Add(-330 * time.Millisecond)},
		// Older than the last reading, which did not have it: there is
		// no span of conn's to ask about, and its life is not one.
		{PID: 21, CPU: time.Hour, Started: wasAt.Add(-2 * time.Hour)},
		// Just spawned and has done nothing yet.
		{PID: 22, CPU: 0, Started: nowAt.Add(-10 * time.Millisecond)},
		// No start time to speak of, and not called working on the
		// strength of it.
		{PID: 23, CPU: time.Hour},
	}
	busy := CpuWorking(was, wasAt, fresh, nowAt)
	if !busy[20] {
		t.Error("a compiler burning its whole short life is not working")
	}
	for _, pid := range []int{21, 22, 23} {
		if busy[pid] {
			t.Errorf("pid %d is working: %v", pid, busy)
		}
	}
}

// The table is read a process at a time, so what comes back can be of
// two moments: a pid listed twice, or one reused in between, leaving a
// parent that is its own descendant. Either costs a row, not the
// reading — the processes view comes back, without looping and without
// saying the same process twice.
func TestATornTableCostsARowNotTheReading(t *testing.T) {
	dup := []Process{
		{PID: 20, PPID: 1, UID: 501, TTY: "ttys001", State: 'S', Command: "zsh", Args: []string{"-zsh"}, Started: processesNow.Add(-time.Hour), Cwd: "/Users/w0zro"},
		{PID: 21, PPID: 20, UID: 501, TTY: "ttys001", State: 'S', Command: "go", Args: []string{"go", "build"}, Started: processesNow.Add(-time.Minute), Cwd: "/Users/w0zro"},
		{PID: 21, PPID: 20, UID: 501, TTY: "ttys001", State: 'S', Command: "go", Args: []string{"go", "build"}, Started: processesNow.Add(-time.Minute), Cwd: "/Users/w0zro"},
	}
	var pids []int
	for _, pl := range ProjectsFrom(dup, 501, testRoots, testIsProject, nil) {
		for _, e := range pl.Entries {
			pids = append(pids, e.PID)
		}
	}
	if !slices.Equal(pids, []int{20, 21}) {
		t.Errorf("a pid listed twice reads as %v", pids)
	}
	cycle := []Process{
		{PID: 10, PPID: 11, UID: 501, TTY: "ttys001", State: 'S', Command: "zsh", Args: []string{"-zsh"}, Started: processesNow.Add(-time.Hour), Cwd: "/Users/w0zro"},
		{PID: 11, PPID: 10, UID: 501, TTY: "ttys001", State: 'S', Command: "bash", Args: []string{"bash"}, Started: processesNow.Add(-time.Minute), Cwd: "/Users/w0zro"},
		// Two more under one of them, so the ordering of that one's
		// children is something that has to be worked out at all.
		{PID: 13, PPID: 10, UID: 501, TTY: "ttys001", State: 'S', Command: "go", Args: []string{"go", "build"}, Started: processesNow.Add(-time.Minute), Cwd: "/Users/w0zro"},
		{PID: 14, PPID: 10, UID: 501, TTY: "ttys001", State: 'S', Command: "vim", Args: []string{"vim"}, Started: processesNow.Add(-time.Minute), Cwd: "/Users/w0zro"},
		{PID: 12, PPID: 1, UID: 501, TTY: "ttys002", State: 'S', Command: "zsh", Args: []string{"-zsh"}, Started: processesNow.Add(-time.Hour), Cwd: "/Users/w0zro"},
	}
	done := make(chan []Project, 1)
	go func() { done <- ProjectsFrom(cycle, 501, testRoots, testIsProject, nil) }()
	select {
	case projects := <-done:
		// The one process standing clear of the cycle is still read.
		var pids []int
		for _, pl := range projects {
			for _, e := range pl.Entries {
				pids = append(pids, e.PID)
			}
		}
		if !slices.Contains(pids, 12) {
			t.Errorf("the process outside the cycle was lost: %v", pids)
		}
		for _, pl := range projects {
			seen := map[int]bool{}
			for _, e := range pl.Entries {
				if seen[e.PID] {
					t.Errorf("pid %d is in the processes view twice", e.PID)
				}
				seen[e.PID] = true
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the processes view did not come back from a cycle in the table")
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
		if got := Brief(c.d); got != c.want {
			t.Errorf("brief(%v) = %q, want %q", c.d, got, c.want)
		}
	}
	if SinceWord(time.Time{}, processesNow) != "" {
		t.Error("a row with no moment got a word")
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
	procs := []Process{
		{PID: 100, PPID: 1, UID: 501, TTY: "ttys002", State: 'S', Command: "login", Args: []string{"login", "-flp", "w0zro"}, Cwd: "/Users/w0zro"},
		{PID: 101, PPID: 100, UID: 501, TTY: "ttys002", State: 'S', Command: "zsh", Args: []string{"-zsh"}, Cwd: "/Users/w0zro"},
		{PID: 102, PPID: 101, UID: 501, TTY: "ttys002", State: 'T', Command: "vim", Args: []string{"vim", "notes.md"}, Cwd: "/Users/w0zro"},
		{PID: 103, PPID: 101, UID: 501, TTY: "ttys002", State: 'S', Command: "go", Args: []string{"go", "run", "."}, Cwd: "/Users/w0zro/projects/w0zro/conn"},
		{PID: 104, PPID: 103, UID: 501, TTY: "ttys002", State: 'S', Command: "conn", Args: []string{"/tmp/go-build/conn"}, Cwd: "/Users/w0zro/projects/w0zro/conn"},
		{PID: 105, PPID: 104, UID: 501, TTY: "ttys002", State: 'S', Command: "tmux", Args: []string{"tmux", "-S", "/x/sock", "attach"}, Cwd: "/Users/w0zro/projects/w0zro/conn"},
		// The panel, inside the server, whose parent holds no terminal.
		{PID: 200, PPID: 199, UID: 501, TTY: "ttys000", State: 'S', Command: "conn", Args: []string{"/tmp/go-build/conn"}, Cwd: "/Users/w0zro"},
		// And a shell of the operator's own, in the server, which is work.
		{PID: 201, PPID: 199, UID: 501, TTY: "ttys003", State: 'S', Command: "zsh", Args: []string{"-zsh"}, Cwd: "/Users/w0zro/projects/w0zro/conn"},
	}
	var got []string
	for _, pl := range ProjectsFrom(procs, 501, testRoots, testIsProject, nil) {
		for _, e := range pl.Entries {
			got = append(got, strings.Repeat(" ", e.Depth)+e.Kind+" "+e.Command+" "+strconv.Itoa(e.PID))
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

// A process is known by the name of its program.
func TestKindsAndCommands(t *testing.T) {
	for _, c := range []struct {
		p       Process
		kind    string
		command string
	}{
		{Process{Command: "zsh", Args: []string{"-zsh"}}, KindShell, "zsh"},
		{Process{Command: "node", Args: []string{"/usr/local/bin/claude", "--resume"}}, KindContact, "claude --resume"},
		{Process{Command: "nvim"}, KindEditor, "nvim"},
		{Process{Command: "conn", Args: []string{"/Users/w0zro/.local/bin/conn"}}, KindConn, "conn"},
		{Process{Command: "go", Args: []string{"go", "test", "./..."}}, KindRun, "go test ./..."},
		{Process{Command: "python3.12"}, KindRun, "python3.12"},
		{Process{Command: "conn", Args: []string{"/usr/local/bin/conn", "hold"}}, kindHold, "conn hold"},
		// A written title: the name is the first word of it, and the
		// whole of it is what the process was started as.
		{Process{Command: "claude", Args: []string{"claude bg-spare", "--bg-spare", "/tmp/1a39b95b.claim.sock"}}, KindContact, "claude bg-spare --bg-spare /tmp/1a39b95b.claim.sock"},
		// A script handed on the line, newlines and all, is one line
		// on the row.
		{Process{Command: "python3", Args: []string{"python3", "-c", "import time\nwhile True:\n    work()\n"}}, KindRun, "python3 -c import time while True: work()"},
	} {
		if kind, cmd := KindOf(c.p), commandLine(c.p); kind != c.kind || cmd != c.command {
			t.Errorf("%+v: %s %q, want %s %q", c.p, kind, cmd, c.kind, c.command)
		}
	}
	if Age(processesNow.Add(-3*24*time.Hour-2*time.Hour), processesNow) != "3D 02H" ||
		Age(processesNow.Add(-2*time.Hour-5*time.Minute), processesNow) != "2H 05M" ||
		Age(processesNow.Add(-47*time.Minute-9*time.Second), processesNow) != "47M 09S" ||
		Age(processesNow.Add(-11*time.Second), processesNow) != "11S" ||
		Age(time.Time{}, processesNow) != "" {
		t.Errorf("ages: %q %q %q %q", Age(processesNow.Add(-3*24*time.Hour-2*time.Hour), processesNow), Age(processesNow.Add(-2*time.Hour-5*time.Minute), processesNow), Age(processesNow.Add(-47*time.Minute-9*time.Second), processesNow), Age(processesNow.Add(-11*time.Second), processesNow))
	}
}

// A name is a contact's when it means an agent and means little else.
// The word says there is a mind at the other end and that the row can
// stop and wait on you, which is more than the other kinds claim.
func TestOnlyAnAgentsNameIsAContacts(t *testing.T) {
	kind := func(args ...string) string {
		return KindOf(Process{Command: args[0], Args: args})
	}
	for _, name := range []string{"claude", "codex", "gemini", "aider", "opencode", "amp", "copilot"} {
		if got := kind(name); got != KindContact {
			t.Errorf("%s: %s, want %s", name, got, KindContact)
		}
	}
	// goose migrates a database at least as often as it agents, and
	// ollama's own processes are a server and a download.
	for _, c := range [][]string{{"goose", "up"}, {"ollama", "serve"}, {"ollama", "run", "llama3"}} {
		if got := kind(c...); got != KindRun {
			t.Errorf("%v: %s, want %s", c, got, KindRun)
		}
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
	is := ProjectDirs([]string{dir})
	if !is(repo) {
		t.Error("a repository outside the roots is no project")
	}
	if is(filepath.Join(away, "checkouts")) {
		t.Error("a folder of checkouts outside the roots is a project")
	}
	roots := RootFinder(is)
	if got := roots(filepath.Join(away, "checkouts")); got != filepath.Join(away, "checkouts") {
		t.Errorf("it works at %q rather than standing for itself", got)
	}
	if got := roots(filepath.Join(repo, "deep")); got != repo {
		t.Errorf("work in a repository outside the roots is placed at %q", got)
	}
}

// The processes view stands every process for its own work, nested
// under whatever runs it: claude's node and its bash, the bash's own
// go, a shell over its idle sibling, another over its stopped vim.
// The projects are by path, and within one everything sits where it
// started, oldest first — the trees by their own roots, a row among
// its siblings by itself. Nothing of root's, of another user's, without a
// terminal, or conn's own — conn is the instrument and not the work,
// though something under it, however unlikely, would still root a tree
// of its own.
func TestProcessesStandsOneProcessForEachWork(t *testing.T) {
	projects := ProjectsFrom(testProcs, 501, testRoots, testIsProject, nil)
	var got []string
	for _, pl := range projects {
		for _, e := range pl.Entries {
			got = append(got, strings.Repeat(" ", e.Depth)+pl.Path+" "+e.Kind+" "+e.Command+" "+string(e.Status))
		}
	}
	// Home, conn, then conjurer, by path, though conjurer's work is
	// older than conn's. conn's own project is left with the shell on
	// ttys005, ninety seconds old, because the three-hour shell on
	// ttys004 is the one conn was started from and is conn's own lineage
	// rather than work. Under claude the node it started forty-six
	// minutes ago comes before the bash it started twelve seconds ago.
	want := []string{
		"/Users/w0zro SHELL zsh ACTIVE",
		" /Users/w0zro EDITOR vim notes.md STOPPED",
		"/Users/w0zro/projects/w0zro/conn SHELL zsh IDLE",
		"/Users/w0zro/projects/w0zro/vim.pro/conjurer SHELL zsh ACTIVE",
		" /Users/w0zro/projects/w0zro/vim.pro/conjurer CONTACT claude --resume ACTIVE",
		"  /Users/w0zro/projects/w0zro/vim.pro/conjurer RUN node /opt/claude/mcp.js ACTIVE",
		"  /Users/w0zro/projects/w0zro/vim.pro/conjurer SHELL bash -c go test ./... ACTIVE",
		"   /Users/w0zro/projects/w0zro/vim.pro/conjurer RUN go test ./... ACTIVE",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("processes:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	find := func(pid int) Entry {
		for _, pl := range projects {
			for _, e := range pl.Entries {
				if e.PID == pid {
					return e
				}
			}
		}
		t.Fatalf("pid %d is not in the processes view", pid)
		return Entry{}
	}
	if e := find(80002); !e.Fault {
		t.Error("the stopped vim is not a fault")
	}
	if e := find(80001); e.Fault {
		t.Error("the shell over it is a fault")
	}
	// A shell running anything, however deep, is active; bare, idle.
	if e := find(70001); e.Status != StatusActive {
		t.Errorf("a shell running claude is %s, not active", e.Status)
	}
	if e := find(67040); e.Status != StatusIdle {
		t.Errorf("a bare shell is %s, not idle", e.Status)
	}
	// conn is in the table, at the same directory as its own shell, and is
	// not a row of it — nor is the tmux client it holds, which is conn's
	// own doing and goes off the processes view with it rather than
	// hanging from the shell above conn.
	for _, pl := range projects {
		for _, e := range pl.Entries {
			if e.Kind == KindConn {
				t.Error("conn is in its own view")
			}
			if e.PID == 67033 {
				t.Errorf("the tmux client conn holds is a row: %+v", e)
			}
		}
	}
	// The go test and the node stand on their own once claude is gone,
	// each a root of its own project's tree; the shell it left is idle.
	var without []Process
	for _, p := range testProcs {
		if p.PID != 70100 {
			without = append(without, p)
		}
	}
	got = got[:0]
	for _, pl := range ProjectsFrom(without, 501, testRoots, testIsProject, nil) {
		for _, e := range pl.Entries {
			got = append(got, strings.Repeat(" ", e.Depth)+e.Kind+" "+e.Command+" "+string(e.Status))
		}
	}
	want = []string{
		"SHELL zsh ACTIVE",
		" EDITOR vim notes.md STOPPED",
		"SHELL zsh IDLE",
		"SHELL zsh IDLE",
		"RUN node /opt/claude/mcp.js ACTIVE",
		"SHELL bash -c go test ./... ACTIVE",
		" RUN go test ./... ACTIVE",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("processes without claude:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if b := ProjectsFrom(nil, 501, testRoots, testIsProject, nil); len(b) != 0 {
		t.Errorf("an empty table gives %+v", b)
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
	roots := RootFinder(ProjectDirs(nil))
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
	roots := RootFinder(ProjectDirs([]string{dir}))

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
	if ProjectDirs([]string{dir})(dir) {
		t.Error("the root is a project of its own")
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
	row := func(pid int, status Status, started, since time.Time) Entry {
		return Entry{PID: pid, Status: status, Started: started, Since: since}
	}
	first := []Project{{Path: "/w", Entries: []Entry{
		row(1, StatusIdle, t0.Add(-time.Hour), time.Time{}),
		row(2, StatusWorking, t0.Add(-time.Hour), t0.Add(-7*time.Minute)),
	}}}
	was := SinceSeen(first, nil, time.Time{}, t0)
	if !first[0].Entries[0].Since.IsZero() {
		t.Errorf("the first reading dated a shell: %v", first[0].Entries[0].Since)
	}
	if !first[0].Entries[1].Since.Equal(t0.Add(-7 * time.Minute)) {
		t.Errorf("a contact lost its own moment: %v", first[0].Entries[1].Since)
	}

	t1 := t0.Add(2 * time.Second)
	second := []Project{{Path: "/w", Entries: []Entry{
		row(1, StatusActive, t0.Add(-time.Hour), time.Time{}), // changed
		row(2, StatusWorking, t0.Add(-time.Hour), t0.Add(-7*time.Minute)),
		row(3, StatusActive, t0.Add(time.Second), time.Time{}), // born between
	}}}
	was = SinceSeen(second, was, t0, t1)
	e := second[0].Entries
	if !e[0].Since.Equal(t1) {
		t.Errorf("a changed row is dated %v, not the reading that saw it", e[0].Since)
	}
	if !e[2].Since.Equal(t0.Add(time.Second)) {
		t.Errorf("a row born between readings is dated %v, not its birth", e[2].Since)
	}

	t2 := t1.Add(2 * time.Second)
	third := []Project{{Path: "/w", Entries: []Entry{
		row(1, StatusActive, t0.Add(-time.Hour), time.Time{}),  // as it was
		row(3, StatusActive, t1.Add(time.Second), time.Time{}), // the pid come round again
	}}}
	SinceSeen(third, was, t1, t2)
	e = third[0].Entries
	if !e[0].Since.Equal(t1) {
		t.Errorf("a row standing as it did was redated: %v", e[0].Since)
	}
	if !e[1].Since.Equal(t1.Add(time.Second)) {
		t.Errorf("a pid come round again took the old process's date: %v", e[1].Since)
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
	procs := []Process{
		// Burned eight seconds of its twelve and has been asleep since.
		{PID: 30, CPU: 8 * time.Second, Started: nowAt.Add(-12 * time.Second)},
		// Working right now, and still not said so: there is nothing
		// to say it against.
		{PID: 31, CPU: 300 * time.Millisecond, Started: nowAt.Add(-330 * time.Millisecond)},
	}
	if busy := CpuWorking(nil, time.Time{}, procs, nowAt); len(busy) != 0 {
		t.Errorf("with no reading behind it the processes view calls %v working", busy)
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
	procs := []Process{
		{PID: 1, PPID: 0, UID: 0, Command: "launchd", Started: processesNow.Add(-5 * 24 * time.Hour)},
		{PID: 300, PPID: 1, UID: 501, TTY: "ttys004", State: 'S', Command: "zsh", Args: []string{"-zsh"}, Started: processesNow.Add(-3 * time.Hour), Cwd: conn},
		{PID: 310, PPID: 300, UID: 501, TTY: "ttys004", Foreground: true, State: 'S', Command: "claude", Args: []string{"claude"}, Started: processesNow.Add(-47 * time.Minute), Cwd: conn},
		{PID: 320, PPID: 310, UID: 501, State: 'S', Command: "python3", Args: []string{"python3", "-m", "http.server", "8000"}, Started: processesNow.Add(-30 * time.Second), Cwd: conn + "/docs"},
		{PID: 330, PPID: 1, UID: 501, State: 'S', Command: "python3", Args: []string{"python3", "-m", "http.server", "8137"}, Started: processesNow.Add(-26 * time.Hour), Cwd: conn + "/docs"},
		{PID: 400, PPID: 1, UID: 501, TTY: "ttys009", State: 'S', Command: "zsh", Args: []string{"-zsh"}, Started: processesNow.Add(-2 * time.Hour), Cwd: "/Users/w0zro"},
		{PID: 410, PPID: 1, UID: 501, State: 'S', Command: "weatherd", Args: []string{"weatherd"}, Started: processesNow.Add(-5 * time.Hour), Cwd: "/Users/w0zro/Library/Containers/com.apple.weather.widget/Data"},
		{PID: 500, PPID: 1, UID: 501, State: 'S', Command: "tmux", Args: []string{"tmux", "-S", "/Users/w0zro/.local/state/conn/tmux.sock", "new-session"}, Started: processesNow.Add(-90 * time.Second), Cwd: conn},
		{PID: 510, PPID: 500, UID: 501, TTY: "ttys003", State: 'S', Command: "conn", Args: []string{"conn"}, Started: processesNow.Add(-89 * time.Second), Cwd: conn},
		{PID: 600, PPID: 1, UID: 502, State: 'S', Command: "python3", Args: []string{"python3", "-m", "http.server", "9999"}, Started: processesNow.Add(-time.Hour), Cwd: conn + "/docs"},
	}
	projects := ProjectsFrom(procs, 501, testRoots, testIsProject, nil)
	var got []string
	for _, pl := range projects {
		for _, e := range pl.Entries {
			got = append(got, strings.Repeat(" ", e.Depth)+pl.Path+" "+e.Kind+" "+e.Command+" "+string(e.Status))
		}
	}
	// Home stands first by path, though its shell began after all of
	// conn's work; at conn the server that outlived its shell is the
	// oldest thing there and stands first.
	want := []string{
		"/Users/w0zro SHELL zsh IDLE",
		conn + " RUN python3 -m http.server 8137 ACTIVE",
		conn + " SHELL zsh ACTIVE",
		" " + conn + " CONTACT claude ACTIVE",
		"  " + conn + " RUN python3 -m http.server 8000 ACTIVE",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("processes:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	on := map[int]bool{}
	for _, pl := range projects {
		for _, e := range pl.Entries {
			on[e.PID] = true
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

// The list holds still. Work appearing in a project moves nothing that
// was already there — not the row it hangs under, not that row's
// siblings, not the projects among themselves: it goes on the end of
// where it belongs and everything else keeps its order. A project the
// view has never had goes where its path sorts, which is the one place
// it will always be. It was the newest start anywhere in a subtree that
// ordered all three, so a command a contact ran re-sorted the
// processes view out from under whoever was reading it.
func TestTheProcessesViewHoldsItsOrder(t *testing.T) {
	rows := func(procs []Process) []int {
		var out []int
		for _, pl := range ProjectsFrom(procs, 501, testRoots, testIsProject, nil) {
			for _, e := range pl.Entries {
				out = append(out, e.PID)
			}
		}
		return out
	}
	before := rows(testProcs)

	// A command under the contact, a shell of its own in the oldest
	// project, and a tree in a project the processes view has never had:
	// each is newer than everything on the list.
	grown := append(append([]Process{}, testProcs...),
		Process{PID: 70999, PPID: 70100, UID: 501, TTY: "ttys007", State: 'R', Command: "rg", Args: []string{"rg", "conn"},
			Started: processesNow.Add(-time.Second), Cwd: "/Users/w0zro/projects/w0zro/vim.pro/conjurer"},
		Process{PID: 80999, PPID: 1, UID: 501, TTY: "ttys012", State: 'S', Command: "zsh", Args: []string{"-zsh"},
			Started: processesNow.Add(-2 * time.Second), Cwd: "/Users/w0zro"},
		Process{PID: 90999, PPID: 1, UID: 501, TTY: "ttys013", State: 'S', Command: "zsh", Args: []string{"-zsh"},
			Started: processesNow.Add(-3 * time.Second), Cwd: "/private/tmp/scratch"},
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
	// last in the project it is in, and the new project where its path
	// sorts — /private after /Users, whatever began when.
	if last := after[len(after)-1]; last != 90999 {
		t.Errorf("a project the processes view has never had is not where its path sorts: last row is %d", last)
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

// The row's command is what was typed: the argument conn adds when it
// raises a contact is left off, in either spelling, and the readout's
// line keeps it. The kill's question names the program alone.
func TestTheRowSaysWhatWasTyped(t *testing.T) {
	raised := Process{Command: "claude", Args: []string{"claude", "--append-system-prompt", "You are running inside conn", "--resume", "abc"}}
	if got := typedLine(raised); got != "claude" {
		t.Errorf("typedLine = %q", got)
	}
	if got := commandLine(raised); got != "claude --append-system-prompt You are running inside conn --resume abc" {
		t.Errorf("commandLine = %q", got)
	}
	joined := Process{Command: "claude", Args: []string{"claude", "--append-system-prompt=note", "--resume=abc", "--model", "opus"}}
	if got := typedLine(joined); got != "claude --model opus" {
		t.Errorf("typedLine with the values joined = %q", got)
	}
	if got := Program("go test ./..."); got != "go" {
		t.Errorf("program = %q", got)
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
		nothing = Standing{}
		busy    = Standing{Working: true}
		waits   = Standing{Waiting: true}
	)
	shell := Process{State: 'S'}
	if s, _ := statusOf(shell, KindShell, false, busy); s != StatusWorking {
		t.Errorf("a bare shell doing something is %s", s)
	}
	if s, _ := statusOf(shell, KindShell, false, nothing); s != StatusIdle {
		t.Errorf("a bare shell doing nothing is %s", s)
	}
	if s, _ := statusOf(Process{State: 'T'}, KindRun, false, busy); s != StatusStopped {
		t.Error("a stopped process that was working is not stopped")
	}
	if s, _ := statusOf(Process{State: 'S'}, KindRun, true, nothing); s != StatusActive {
		t.Errorf("a run doing nothing is %s", s)
	}
	contact := Process{State: 'S'}
	s, fault := statusOf(contact, KindContact, false, waits)
	if s != StatusWaiting {
		t.Errorf("a contact waiting on you is %s", s)
	}
	if fault {
		t.Error("a contact waiting on you is a fault")
	}
	// A contact stopped with its turn over holds nothing up, and reads the
	// way anything else at rest does rather than asking for you.
	if s, _ := statusOf(contact, KindContact, false, Standing{Idle: true}); s != StatusIdle {
		t.Errorf("a contact with its turn over is %s, not idle", s)
	}
	if s, _ := statusOf(contact, KindContact, false, Standing{Working: true, Waiting: true}); s != StatusWaiting {
		t.Errorf("a contact that says both is %s, not waiting", s)
	}
	if s, _ := statusOf(Process{State: 'T'}, KindContact, false, waits); s != StatusStopped {
		t.Error("a stopped contact is not stopped")
	}
}

// The waiting are answered longest held up first. A contact that cannot
// say when it stopped is waiting all the same, but it cannot claim a
// turn ahead of one that can prove it waited longer, so it goes last;
// two that stopped at the same moment go by pid, so the ring is the
// same ring on every reading.
func TestWaitingRoundIsLongestHeldUpFirst(t *testing.T) {
	at := func(s int) time.Time { return processesNow.Add(time.Duration(-s) * time.Second) }
	projects := []Project{
		{Path: "/a", Entries: []Entry{
			{PID: 1, Status: StatusWorking, Since: at(900)},
			{PID: 2, Status: StatusWaiting, Since: at(60)},
			{PID: 3, Status: StatusIdle, Since: at(900)},
		}},
		{Path: "/b", Entries: []Entry{
			{PID: 4, Status: StatusWaiting}, // says nothing of when
			{PID: 5, Status: StatusWaiting, Since: at(600)},
			{PID: 7, Status: StatusWaiting, Since: at(300)},
			{PID: 6, Status: StatusWaiting, Since: at(300)},
		}},
	}
	var got []int
	for _, e := range WaitingRound(projects) {
		got = append(got, e.PID)
	}
	want := []int{5, 6, 7, 2, 4} // 600s, then the two at 300s by pid, then 60s, then the one that cannot say
	if !slices.Equal(got, want) {
		t.Errorf("the waiting round is %v, want %v", got, want)
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
	procs := []Process{
		{PID: 10, CPU: 5 * time.Hour, Started: wasAt.Add(-5 * 24 * time.Hour)},
		{PID: 11, CPU: time.Second, Started: wasAt.Add(-time.Minute)},
		{PID: 12, CPU: time.Second + 20*time.Millisecond, Started: wasAt.Add(-time.Minute)},
	}
	busy := CpuWorking(was, wasAt, procs, nowAt)
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
	if b := CpuWorking(was, wasAt, procs, wasAt); b[10] || b[12] {
		t.Errorf("no time passed and %v is working", b)
	}
}

// A process table on file: two terminals of work on this machine. On
// ttys004, a shell running conn. On ttys007, a shell running claude,
// which runs a node of its own and a bash it asked for, which runs a go
// test. On ttys009, a shell at its prompt, and one stopped vim. A root
// process, and one with no terminal working at / — no project, so
// nothing adopts it and the processes view leaves it out.
var (
	processesNow = time.Date(2026, 9, 9, 3, 0, 0, 0, time.UTC)
	testProcs    = []Process{
		{PID: 1, PPID: 0, UID: 0, Command: "launchd", Started: processesNow.Add(-5 * 24 * time.Hour)},
		{PID: 500, PPID: 1, UID: 501, Command: "distnoted", State: 'S', Started: processesNow.Add(-4 * 24 * time.Hour), Cwd: "/"},
		{PID: 67031, PPID: 1, UID: 501, TTY: "ttys004", State: 'S', Command: "zsh", Args: []string{"-zsh"}, Started: processesNow.Add(-3 * time.Hour), Cwd: "/Users/w0zro/projects/w0zro/conn"},
		{PID: 67032, PPID: 67031, UID: 501, TTY: "ttys004", Foreground: true, State: 'S', Command: "conn", Args: []string{"./conn"}, Started: processesNow.Add(-90 * time.Second), Cwd: "/Users/w0zro/projects/w0zro/conn"},
		{PID: 67033, PPID: 67032, UID: 501, TTY: "ttys004", State: 'S', Command: "tmux", Args: []string{"tmux", "-S", "/Users/w0zro/.local/state/conn/sock", "attach"}, Started: processesNow.Add(-89 * time.Second), Cwd: "/Users/w0zro/projects/w0zro/conn"},
		{PID: 67040, PPID: 67031, UID: 501, TTY: "ttys005", State: 'S', Command: "zsh", Args: []string{"-zsh"}, Started: processesNow.Add(-90 * time.Second), Cwd: "/Users/w0zro/projects/w0zro/conn"},
		{PID: 70001, PPID: 1, UID: 501, TTY: "ttys007", State: 'S', Command: "zsh", Args: []string{"-zsh"}, Started: processesNow.Add(-2 * time.Hour), Cwd: "/Users/w0zro/projects/w0zro/vim.pro/conjurer"},
		{PID: 70100, PPID: 70001, UID: 501, TTY: "ttys007", Foreground: true, State: 'S', Command: "claude", Args: []string{"claude", "--resume"}, Started: processesNow.Add(-47 * time.Minute), Cwd: "/Users/w0zro/projects/w0zro/vim.pro/conjurer"},
		{PID: 70212, PPID: 70100, UID: 501, TTY: "ttys007", State: 'S', Command: "node", Args: []string{"node", "/opt/claude/mcp.js"}, Started: processesNow.Add(-46 * time.Minute), Cwd: "/Users/w0zro/projects/w0zro/vim.pro/conjurer"},
		{PID: 70300, PPID: 70100, UID: 501, TTY: "ttys007", State: 'S', Command: "bash", Args: []string{"bash", "-c", "go test ./..."}, Started: processesNow.Add(-12 * time.Second), Cwd: "/Users/w0zro/projects/w0zro/vim.pro/conjurer/internal"},
		{PID: 70301, PPID: 70300, UID: 501, TTY: "ttys007", State: 'R', Command: "go", Args: []string{"go", "test", "./..."}, Started: processesNow.Add(-11 * time.Second), Cwd: "/Users/w0zro/projects/w0zro/vim.pro/conjurer/internal"},
		{PID: 80001, PPID: 1, UID: 501, TTY: "ttys009", Foreground: true, State: 'S', Command: "zsh", Args: []string{"-zsh"}, Started: processesNow.Add(-26 * time.Hour), Cwd: "/Users/w0zro"},
		{PID: 80002, PPID: 80001, UID: 501, TTY: "ttys009", State: 'T', Command: "vim", Args: []string{"vim", "notes.md"}, Started: processesNow.Add(-25 * time.Hour), Cwd: "/Users/w0zro"},
		{PID: 90000, PPID: 1, UID: 502, TTY: "ttys011", State: 'S', Command: "zsh", Args: []string{"-zsh"}, Started: processesNow.Add(-time.Hour), Cwd: "/Users/other"},
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
)
