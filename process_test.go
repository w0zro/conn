package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
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

// The watch stands one process for each piece of work: conn for its
// shell, claude for everything it runs, the idle shell for itself, the
// stopped vim over its shell; the newest work first; nothing of root's,
// of another user's, or without a terminal.
func TestWatchStandsOneProcessForEachWork(t *testing.T) {
	places := watch(testProcs, 67032, 501, testRoots)
	var got []string
	for _, pl := range places {
		for _, e := range pl.entries {
			got = append(got, pl.path+" "+e.kind+" "+e.command+" "+e.status)
		}
	}
	want := []string{
		"/Users/w0zro/projects/w0zro/conn CONN conn HERE",
		"/Users/w0zro/projects/w0zro/vim.pro/conjurer AGENT claude --resume ACTIVE",
		"/Users/w0zro EDITOR vim notes.md STOPPED",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("watch:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if places[2].entries[0].fault != true || places[0].entries[0].fault {
		t.Error("the stopped vim is not a fault, or conn is")
	}
	// The go test shows once claude is gone, and the shell it left idle.
	var without []process
	for _, p := range testProcs {
		if p.pid != 70100 {
			without = append(without, p)
		}
	}
	got = got[:0]
	for _, pl := range watch(without, 67032, 501, testRoots) {
		for _, e := range pl.entries {
			got = append(got, e.kind+" "+e.command+" "+e.status)
		}
	}
	want = []string{"RUN go test ./... ACTIVE", "RUN node /opt/claude/mcp.js ACTIVE", "SHELL zsh IDLE", "CONN conn HERE", "EDITOR vim notes.md STOPPED"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("watch without claude:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if b := watch(nil, 1, 501, testRoots); len(b) != 0 {
		t.Errorf("an empty table gives %+v", b)
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
	want := process{pid: 70301, command: "go (test)", state: 'R', ppid: 70300, pgid: 70300, tty: "pts/7", foreground: true, started: boot.Add(432000 * time.Second)}
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
