package main

import (
	"net"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestUnderMatchesRepoAndNested(t *testing.T) {
	for _, tc := range []struct {
		dir, path string
		want      bool
	}{
		{"/p/repo", "/p/repo", true},
		{"/p/repo/src/deep", "/p/repo", true},
		{"/p/repo-other", "/p/repo", false}, // prefix, but a different repo
		{"/p", "/p/repo", false},
		{"/other", "/p/repo", false},
	} {
		if got := under(tc.dir, tc.path); got != tc.want {
			t.Errorf("under(%q, %q) = %v, want %v", tc.dir, tc.path, got, tc.want)
		}
	}
}

func TestRunningProcsFindsThisTest(t *testing.T) {
	procs, err := runningProcs()
	if err != nil {
		t.Skipf("lsof unavailable: %v", err)
	}
	if len(procs) == 0 {
		t.Fatal("no processes found")
	}

	// The test binary runs in this package's directory, so that directory must
	// show up — and this process itself must not, since conn excludes its own.
	cwd, _ := os.Getwd()
	var sawCwd bool
	for _, p := range procs {
		if p.PID == os.Getpid() {
			t.Errorf("runningProcs included our own pid %d", p.PID)
		}
		if p.Dir == cwd {
			sawCwd = true
		}
	}
	if !sawCwd {
		t.Errorf("no process reported %q as its cwd", cwd)
	}
}

func TestTheScanReadsDirectoriesAndPortsFromOneListing(t *testing.T) {
	// lsof lists every process's working directory and its listening
	// sockets in one go, the files in whatever order it keeps them: the
	// directory is the file under cwd, a port is the end of an address
	// under any other descriptor — the host may hold colons of its own —
	// and a port held on two addresses is one port.
	out := []byte("p100\nR1\ncnode\nfcwd\nn/p/app\nf20\nn*:5173\nf21\nn[::1]:5173\nf22\nn127.0.0.1:24678\n" +
		"p200\nR1\ncmongod\nf9\nn127.0.0.1:27017\nfcwd\nn/opt/db\n" +
		"p300\nR1\nczsh\nfcwd\nn/p/app\n" +
		"p400\nR100\ncsh\nfcwd\nn/p/app\n")
	procs, err := parseScan(out, 100, map[int]psInfo{200: {argv: "mongod --config x"}})
	if err != nil {
		t.Fatal(err)
	}
	want := map[int]string{200: "27017", 300: ""}
	for _, p := range procs {
		if p.PID == 100 || p.PID == 400 {
			t.Errorf("the scan reported %+v, which is conn or a child of it", p)
			continue
		}
		if got := strings.Join(p.Ports, ","); got != want[p.PID] {
			t.Errorf("ports of %d = %q, want %q", p.PID, got, want[p.PID])
		}
		delete(want, p.PID)
	}
	for pid := range want {
		t.Errorf("the scan did not report %d", pid)
	}

	// Ports before the directory, and the directory of a process that
	// listens: the order of the files does not decide either.
	out = []byte("p500\nR1\ncnode\nf20\nn*:8080\nf21\nn*:80\nfcwd\nn/p/web\n")
	procs, _ = parseScan(out, -1, nil)
	if len(procs) != 1 || procs[0].Dir != "/p/web" || strings.Join(procs[0].Ports, ",") != "80,8080" {
		t.Errorf("procs = %+v, want one in /p/web on 80 and 8080", procs)
	}
}

func TestTheScanFindsAListener(t *testing.T) {
	c := exec.Command("python3", "-m", "http.server", "8932", "--bind", "127.0.0.1")
	c.Dir = "/tmp"
	if err := c.Start(); err != nil {
		t.Skip(err)
	}
	defer func() { _ = c.Process.Kill() }()

	// Until the port answers, not a hopeful sleep: a loaded runner can
	// outwait any number chosen in advance.
	deadline := time.Now().Add(10 * time.Second)
	for {
		conn, err := net.Dial("tcp", "127.0.0.1:8932")
		if err == nil {
			_ = conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the listener never came up")
		}
		time.Sleep(100 * time.Millisecond)
	}

	// The listener is this test's child, which the scan for this process
	// would leave out; scanned as some other conn would see it.
	procs, err := procsBut(-1)
	if err != nil {
		t.Skipf("lsof unavailable: %v", err)
	}
	for _, p := range procs {
		if p.PID == c.Process.Pid {
			if !slices.Contains(p.Ports, "8932") {
				t.Errorf("ports = %v, want the port the server is on", p.Ports)
			}
			return
		}
	}
	t.Error("the scan did not report the listener at all")
}

func TestProcForestNestsChildren(t *testing.T) {
	roots := procForest([]Proc{
		{PID: 10, PPID: 1, Command: "zsh"},
		{PID: 20, PPID: 10, Command: "claude"},
		{PID: 30, PPID: 20, Command: "go"},
		{PID: 40, PPID: 10, Command: "vim"},
	})

	if len(roots) != 1 || roots[0].PID != 10 {
		t.Fatalf("roots = %v, want a single root pid 10", pids(roots))
	}
	if got := pids(roots[0].Children); len(got) != 2 || got[0] != 20 || got[1] != 40 {
		t.Errorf("children of 10 = %v, want [20 40] in name order", got)
	}
	if got := pids(roots[0].Children[0].Children); len(got) != 1 || got[0] != 30 {
		t.Errorf("children of 20 = %v, want [30]", got)
	}
}

func TestSiblingsOrderByNameSoARestartHoldsItsSlot(t *testing.T) {
	// pid 95 is a Node restarted long after its siblings; name order keeps it
	// in the N slot instead of dropping it to the bottom, case aside. The two
	// zsh rows fall back to pid order, which for same-named siblings is the
	// natural reading: oldest first.
	roots := procForest([]Proc{
		{PID: 10, PPID: 1, Command: "zsh"},
		{PID: 21, PPID: 10, Command: "vim"},
		{PID: 95, PPID: 10, Command: "Node"},
		{PID: 30, PPID: 10, Command: "go"},
		{PID: 40, PPID: 10, Command: "zsh"},
		{PID: 22, PPID: 10, Command: "zsh"},
	})

	want := []int{30, 95, 21, 22, 40}
	got := pids(roots[0].Children)
	if len(got) != len(want) {
		t.Fatalf("children of 10 = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("children of 10 = %v, want %v: name order, ties by pid", got, want)
		}
	}
}

func TestProcForestRootsProcessesWhoseParentIsAbsent(t *testing.T) {
	// The parent runs outside the repo, so it is not in the set.
	roots := procForest([]Proc{
		{PID: 20, PPID: 999, Command: "claude"},
		{PID: 21, PPID: 998, Command: "vim"},
	})
	if got := pids(roots); len(got) != 2 {
		t.Errorf("roots = %v, want both processes rooted", got)
	}
}

func TestProcForestSurvivesCycles(t *testing.T) {
	done := make(chan []*ProcNode, 1)
	go func() {
		done <- procForest([]Proc{
			{PID: 10, PPID: 20}, {PID: 20, PPID: 10},
		})
	}()
	select {
	case roots := <-done:
		if len(roots) == 0 {
			t.Error("a cycle should still yield a root rather than nothing")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("procForest hung on a parent cycle")
	}
}

func TestProcForestIgnoresSelfParent(t *testing.T) {
	roots := procForest([]Proc{{PID: 10, PPID: 10}})
	if len(roots) != 1 || len(roots[0].Children) != 0 {
		t.Error("a process parented to itself should be a childless root")
	}
}

func pids(ns []*ProcNode) []int {
	out := make([]int, len(ns))
	for i, n := range ns {
		out[i] = n.PID
	}
	return out
}

func pidOfSelf() int { return os.Getpid() }

func writeFile(path, body string) error { return os.WriteFile(path, []byte(body), 0o644) }

func TestTheScanDoesNotReportItself(t *testing.T) {
	// conn's own children — the lsof running the scan, the git and ps behind
	// the detail pane — inherit its working directory, so without filtering
	// they flicker through the tree of whatever repo conn was started in.
	procs, err := runningProcs()
	if err != nil {
		t.Skipf("lsof unavailable: %v", err)
	}

	self := os.Getpid()
	for _, p := range procs {
		if p.PPID == self {
			t.Errorf("the scan reported a child of conn's own: %+v", p)
		}
		if p.PID == self {
			t.Errorf("the scan reported conn itself: %+v", p)
		}
	}
}

func TestAFreshShellDoesNotSortBetweenTwoAgents(t *testing.T) {
	// Every shell conn holds is a zsh underneath. Sorted by raw command they
	// are three ties settled by pid — claude, zsh, claude — while the rows
	// wear the names of what runs inside. The order must follow the names.
	agent := func(pid int) *ProcNode {
		return &ProcNode{
			Proc: Proc{PID: pid, Command: "zsh"},
			Children: []*ProcNode{
				{Proc: Proc{PID: pid + 1, Command: "claude", Argv: "claude"}},
			},
		}
	}
	ns := []*ProcNode{agent(100), {Proc: Proc{PID: 200, Command: "zsh"}}, agent(300)}

	sortNodes(ns)

	got := []int{ns[0].PID, ns[1].PID, ns[2].PID}
	want := []int{100, 300, 200} // the agents together, the shell after
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v: the shell sat between the agents", got, want)
		}
	}
}

func TestThePsTableReadsStateStartAndCommandLine(t *testing.T) {
	// The state is one field before lstart's five; the command line is
	// everything after, its own spacing kept.
	table := parsePS("  123 S+   Fri Aug  9 10:00:00 2026 npm run   dev\n 45 Z Sat Sep  4 01:02:03 2026 (node)\nbad line\n")
	if got := table[123]; got.state != "S+" || got.started != "Fri Aug 9 10:00:00 2026" || got.argv != "npm run   dev" {
		t.Errorf("123 = %+v, want the state, the start and the command line apart", got)
	}
	if got := table[45]; got.state != "Z" || got.argv != "(node)" {
		t.Errorf("45 = %+v, want a zombie", got)
	}
	if len(table) != 2 {
		t.Errorf("table has %d entries, want the two that parsed", len(table))
	}
}
