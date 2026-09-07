package main

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"
)

// scanTimeout bounds every command a scan runs. lsof answers in tens of
// milliseconds on a healthy machine; the bound is for the machine with a dead
// network mount, where lsof hangs for minutes and a scan that waited would
// stand in the way of every scan after it.
const scanTimeout = 15 * time.Second

// listing runs one of the commands the scans read, bounded by timeout. What
// was written before a failure is still returned: lsof exits nonzero when any
// process refuses it, which says nothing about the ones that answered.
//
// WaitDelay is for the process the timeout's kill does not take on — an lsof
// stuck in uninterruptible disk wait cannot be killed, and the scan has to
// come back even when the process never will.
func listing(timeout time.Duration, name string, args ...string) ([]byte, error) {
	return listingIn("", timeout, name, args...)
}

// listingIn is listing run in a directory, for a command that reads the
// directory it is run in — compose, which finds its project there.
func listingIn(dir string, timeout time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.WaitDelay = 2 * time.Second
	return cmd.Output()
}

// Proc is a running process, identified by the directory it is working in.
//
// Command is the name of the program, which is what lsof knows. Argv is what
// was actually run, which lsof does not know and ps does: "npm run dev" is a
// node, and being told it is a node is no help at all.
type Proc struct {
	PID     int
	PPID    int
	Command string
	Argv    string
	Dir     string

	// Started is when the process began, as ps prints it — an opaque token
	// that, together with the pid, identifies the process the way a pid
	// alone cannot: pids are recycled, start times are not. A kill compares
	// it before signalling, so a row from an old scan cannot aim at whatever
	// inherited its number.
	Started string

	// State is the process's state as ps reports it — "S+", "R", "Z" —
	// read with the scan so a row can say when a process is stopped or a
	// zombie without being looked at.
	State string

	// Ports is what the process is accepting TCP connections on, by number,
	// lowest first. A port is worth carrying because it is the thing you
	// were about to go and look up: a dev server's row says what it is,
	// and this says where it is.
	Ports []string

	// Container is set on a row that is a container rather than a process:
	// what docker said of it. Its PID is then a number of conn's own,
	// below zero, and Ports are what it publishes on the host (docker.go).
	Container *Container
}

// ProcNode is a process together with the processes it started.
type ProcNode struct {
	Proc
	Children []*ProcNode
}

// runningProcs lists the processes visible to this user along with their
// working directories and the ports they are listening on.
//
// lsof is the only way to read another process's cwd on macOS; there is no
// /proc to walk. Processes owned by other users are reported as permission
// errors on stderr and simply do not appear, which is the behavior we want.
func runningProcs() ([]Proc, error) {
	return procsBut(os.Getpid())
}

// procsBut is runningProcs for a conn of the given pid: neither that
// process nor its children are work happening in a repository.
func procsBut(self int) ([]Proc, error) {
	// One call asks for every process's working directory and every
	// listening TCP socket together — without -a the selections are
	// unioned — which is a few milliseconds over asking for the
	// directories alone, where a second call per row would be that much
	// again for every row drawn. -nP keeps the addresses numeric: a lookup
	// per socket is what makes lsof slow.
	out, err := listing(scanTimeout, "lsof", "-nP", "-d", "cwd", "-iTCP", "-sTCP:LISTEN", "-F", "pcRfn")
	if err != nil && len(out) == 0 {
		return nil, err
	}

	// What each process was run with and when it began, in one call. Asking
	// per process is milliseconds each, which is fine for the one row being
	// inspected and far too slow for a list being redrawn.
	procs, err := parseScan(out, self, psTable())
	if err != nil {
		return nil, err
	}
	// The containers docker runs for a place, filed under the compose
	// that runs them where one is in the list.
	return attachContainers(procs, containers()), nil
}

// parseScan reads what lsof said in procsBut's format: per process, its
// pid, command and parent, then per file its descriptor and name — the
// working directory under cwd, an address under a numbered descriptor.
func parseScan(out []byte, self int, ps map[int]psInfo) ([]Proc, error) {
	var procs []Proc
	ports := map[int][]string{}
	var cur Proc
	fd := ""

	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if len(line) < 2 {
			continue
		}
		field, value := line[0], line[1:]
		switch field {
		case 'p':
			pid, err := strconv.Atoi(value)
			if err != nil {
				cur = Proc{}
				continue
			}
			cur = Proc{PID: pid}
		case 'R':
			cur.PPID, _ = strconv.Atoi(value)
		case 'c':
			cur.Command = value
		case 'f':
			fd = value
		case 'n':
			// Neither conn nor anything it started for itself is work
			// happening in a repository. Its own children — the lsof that ran
			// this scan, the git and ps behind the detail pane — inherit the
			// directory conn was started in, so without this they appear and
			// disappear in that repository's tree on every refresh.
			//
			// conn has no children worth showing: the shells it opens belong
			// to the tmux server, which is a different process and keeps its own
			// working directory well away from any project.
			if cur.PID == 0 || cur.PID == self || cur.PPID == self {
				continue
			}
			if fd != "cwd" {
				if port, ok := portOf(value); ok && !slices.Contains(ports[cur.PID], port) {
					ports[cur.PID] = append(ports[cur.PID], port)
				}
				continue
			}
			if !strings.HasPrefix(value, "/") {
				continue
			}
			cur.Dir = value
			cur.Argv = ps[cur.PID].argv
			cur.Started = ps[cur.PID].started
			cur.State = ps[cur.PID].state
			procs = append(procs, cur)
		}
	}
	for i := range procs {
		if ps := ports[procs[i].PID]; len(ps) > 0 {
			sortPorts(ps)
			procs[i].Ports = ps
		}
	}
	return procs, sc.Err()
}

// portOf is the port in a socket's address as lsof names it. The address is
// host:port, and the host may itself contain colons when it is an IPv6
// address.
func portOf(addr string) (string, bool) {
	i := strings.LastIndex(addr, ":")
	if i < 0 || i == len(addr)-1 {
		return "", false
	}
	return addr[i+1:], true
}

// psInfo is what ps says about one process: its state, when it began, and
// what it was run with.
type psInfo struct {
	state   string
	started string
	argv    string
}

// psTable is every process's state, start time and command line, keyed by
// pid. A failure leaves it empty; a process without an entry falls back to
// its name and goes without the start-time check.
func psTable() map[int]psInfo {
	out, err := listing(scanTimeout, "ps", "-axo", "pid=,stat=,lstart=,command=")
	if err != nil {
		return nil
	}
	return parsePS(string(out))
}

// parsePS reads psTable's columns: the pid, the state, the five fields of
// lstart, and the command line after them.
func parsePS(out string) map[int]psInfo {
	lines := strings.Split(out, "\n")
	table := make(map[int]psInfo, len(lines))
	for _, line := range lines {
		pid, rest := cutField(line)
		n, err := strconv.Atoi(pid)
		if err != nil {
			continue
		}
		state, rest := cutField(rest)
		// lstart is five fields — "Fri Aug 29 10:00:00 2026" — and the
		// command line is everything after them, its own spacing kept. The
		// fields are rejoined rather than sliced out whole, so a padded
		// single-digit day reads the same here as anywhere else ps prints it.
		fields := make([]string, 5)
		for i := range fields {
			fields[i], rest = cutField(rest)
		}
		table[n] = psInfo{state: state, started: strings.Join(fields, " "), argv: strings.TrimSpace(rest)}
	}
	return table
}

// cutField takes the next space-separated field, leaving the rest.
func cutField(s string) (string, string) {
	s = strings.TrimLeft(s, " \t")
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		return s[:i], s[i:]
	}
	return s, ""
}

// procForest arranges processes into parent/child trees. A process whose parent
// is not in the set becomes a root, so a repo's trees start at the outermost
// process actually working in it rather than at some ancestor outside it.
func procForest(procs []Proc) []*ProcNode {
	byPID := make(map[int]*ProcNode, len(procs))
	for _, p := range procs {
		byPID[p.PID] = &ProcNode{Proc: p}
	}

	var roots []*ProcNode
	for _, p := range procs {
		n := byPID[p.PID]
		parent, ok := byPID[p.PPID]
		if ok && parent != n && !descends(parent, n, byPID) {
			parent.Children = append(parent.Children, n)
			continue
		}
		roots = append(roots, n)
	}

	sortNodes(roots)
	return roots
}

// descends reports whether a is inside b's subtree, which would make attaching
// b under a a cycle. Process trees are acyclic in practice; this keeps a
// surprising ps table from hanging the renderer.
func descends(a, b *ProcNode, byPID map[int]*ProcNode) bool {
	for cur, seen := a, 0; cur != nil && seen < len(byPID); seen++ {
		if cur == b {
			return true
		}
		cur = byPID[cur.PPID]
	}
	return false
}

// sortNodes orders each level by the name its row will wear, ties broken by
// PID. The raw command is the wrong key: every shell conn holds is a zsh
// underneath, whatever runs inside it, and sorting on that put a fresh shell
// between two agents — three zsh ties, settled by pid. A name keeps its slot
// when the process behind it is restarted under a new pid, which is when PID
// order would move the row; identically-named siblings fall back to creation
// order, the one place it is the natural reading.
func sortNodes(ns []*ProcNode) {
	slices.SortFunc(ns, func(a, b *ProcNode) int {
		return cmp.Or(cmp.Compare(sortName(a), sortName(b)), cmp.Compare(a.PID, b.PID))
	})
	for _, n := range ns {
		sortNodes(n.Children)
	}
}

// sortName is the name a node's row answers to: the navigator collapses a
// chain with nothing to choose between and names it for the first non-shell
// in it, so the sort walks the same chain the same way.
func sortName(n *ProcNode) string {
	return strings.ToLower(commandOf(nameOf(runFrom(n, 1024))))
}

// runFrom is the run a node heads: the node, then each step down while
// there is nothing to choose between — one child, and that child a step of
// how the command runs rather than a thing of its own. A container is a
// thing of its own: compose running one service is still compose and the
// service, and a service's row folded into the compose's would read as the
// service gone. The bound is for a process table that says a process
// started itself.
func runFrom(n *ProcNode, bound int) []*ProcNode {
	run := []*ProcNode{n}
	for i := 0; len(n.Children) == 1 && n.Children[0].Container == nil && i < bound; i++ {
		n = n.Children[0]
		run = append(run, n)
	}
	return run
}

// indexNodes files a tree by pid, so a process can be reached from anywhere
// that knows only its number.
func indexNodes(n *ProcNode, into map[int]*ProcNode) {
	into[n.PID] = n
	for _, c := range n.Children {
		indexNodes(c, into)
	}
}

// sortPorts orders ports by number, so 80 comes before 8080.
func sortPorts(ports []string) {
	slices.SortFunc(ports, comparePort)
}

func comparePort(a, b string) int {
	x, errA := strconv.Atoi(a)
	y, errB := strconv.Atoi(b)
	if errA != nil || errB != nil {
		return cmp.Compare(a, b)
	}
	return cmp.Compare(x, y)
}

// under reports whether dir is path itself or nested inside it.
func under(dir, path string) bool {
	return dir == path || strings.HasPrefix(dir, strings.TrimSuffix(path, "/")+"/")
}
