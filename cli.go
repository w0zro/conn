package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/w0zro/conn/internal/shell"
	"github.com/w0zro/conn/internal/tmux"
)

// The command line is conn for whoever has no panel: an agent, a
// script, a person at a shell. It works the processes of the project
// the working directory is in, and knows a process the way an agent
// does: by the name it was started under, by the port an error named,
// or by the pid conn ls gave. Agents rebuilt that every session out of
// tmux capture-pane, ps, lsof and pkill -f, which matches by pattern
// and can end the wrong thing.
//
// A process here is a pane of conn's server, or a server listening on
// a port whose directory is in the project and that conn did not start.

// A proc is one process the command line knows.
type proc struct {
	pane    *pane  // nil for a listener conn does not hold
	Session string // the conversation it is, where it is one
	Name    string
	PID     int
	State   string
	Ports   []string
	Command string
	Dir     string
}

// A here is the machine as read once from a directory: the project it
// is in, the checkout, and the processes.
type here struct {
	home, root, tree string
	srv              *tmux.Server
	up               bool
	procs            []proc
}

// readHere reads the machine from a directory.
func readHere(home, dir string) (here, error) {
	dir = realDir(dir)
	h := here{home: home, root: repoRoot(dir), tree: checkout(dir)}
	srv, err := findServer(home)
	if err != nil {
		return h, err
	}
	h.srv = srv
	var ps []pane
	if srv.HasSession(sessionName) {
		h.up = true
		if ps, err = panes(srv); err != nil {
			return h, err
		}
	}
	h.procs = machine(ps, liveClaude(), listening(), parents())
	return h, nil
}

// realDir is a directory as the process table names it, symlinks
// resolved.
func realDir(dir string) string {
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		return real
	}
	return dir
}

// checkout is the top of the git checkout a directory is in, a
// worktree's own, or the directory.
func checkout(dir string) string {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return dir
	}
	return strings.TrimSpace(string(out))
}

// machine is the processes: conn's panes, but conn's own, with the
// ports listened on under each, and the listeners no pane holds.
func machine(ps []pane, live map[string]claudeRun, listeners map[int][]string, parent map[int]int) []proc {
	byPID := map[int]int{} // a pane's process → its index in out
	var out []proc
	for i := range ps {
		p := &ps[i]
		if p.Panel || p.Hold || p.Dead {
			continue
		}
		byPID[p.PID] = len(out)
		state, session := paneState(*p, live)
		out = append(out, proc{pane: p, Session: session, Name: paneName(*p), PID: p.PID, State: state, Command: p.Command, Dir: p.Dir})
	}
	pids := make([]int, 0, len(listeners))
	for pid := range listeners {
		pids = append(pids, pid)
	}
	sort.Ints(pids)
	var outside []int
	for _, pid := range pids {
		if i, ok := under(pid, byPID, parent); ok {
			out[i].Ports = append(out[i].Ports, listeners[pid]...)
			continue
		}
		outside = append(outside, pid)
	}
	if len(outside) > 0 {
		cwds, comms := describe(outside)
		for _, pid := range outside {
			out = append(out, proc{Name: filepath.Base(comms[pid]), PID: pid, State: "outside conn", Ports: listeners[pid], Command: comms[pid], Dir: cwds[pid]})
		}
	}
	for i := range out {
		out[i].Ports = uniq(out[i].Ports)
	}
	return out
}

// under is the pane a process runs under: its own, or its parent's,
// up the tree.
func under(pid int, panes map[int]int, parent map[int]int) (int, bool) {
	for seen := 0; pid > 1 && seen < 64; seen++ {
		if i, ok := panes[pid]; ok {
			return i, true
		}
		pid = parent[pid]
	}
	return 0, false
}

// paneName is what a pane answers to: the name conn u gave it, or what
// runs in it.
func paneName(p pane) string {
	if p.Name != "" {
		return p.Name
	}
	return p.Command
}

// paneState is what a pane is doing — how what conn u started ended,
// what a conversation says of itself, or running — and the
// conversation it is, where it is one: by conn's mark, or by the
// terminal Claude Code says it runs on.
func paneState(p pane, live map[string]claudeRun) (state, session string) {
	if p.Name != "" && p.Exit != "" {
		return "exited " + p.Exit, ""
	}
	if r, ok := live[p.Session]; ok && r.state() != "" {
		return r.state(), p.Session
	}
	for id, r := range live {
		if r.TTY != "" && r.TTY == p.TTY {
			return or(r.state(), "running"), id
		}
	}
	return "running", p.Session
}

func uniq(ss []string) []string {
	sort.Strings(ss)
	out := ss[:0]
	for i, s := range ss {
		if i == 0 || s != ss[i-1] {
			out = append(out, s)
		}
	}
	return out
}

// listening is the TCP ports each process listens on, from lsof.
func listening() map[int][]string {
	out, _ := exec.Command("lsof", "-nP", "-iTCP", "-sTCP:LISTEN", "-Fpn").Output()
	return parseListening(string(out))
}

// parseListening reads lsof -Fpn: a p line names a process, and each n
// line after it an address it listens on, the port after the last
// colon.
func parseListening(out string) map[int][]string {
	m := map[int][]string{}
	pid := 0
	for _, l := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(l, "p"):
			pid, _ = strconv.Atoi(l[1:])
		case strings.HasPrefix(l, "n") && pid > 0:
			if i := strings.LastIndexByte(l, ':'); i >= 0 {
				m[pid] = append(m[pid], l[i+1:])
			}
		}
	}
	return m
}

// parents is every process's parent, from ps.
func parents() map[int]int {
	out, _ := exec.Command("ps", "-axo", "pid=,ppid=").Output()
	m := map[int]int{}
	for _, l := range strings.Split(string(out), "\n") {
		f := strings.Fields(l)
		if len(f) != 2 {
			continue
		}
		pid, _ := strconv.Atoi(f[0])
		ppid, _ := strconv.Atoi(f[1])
		m[pid] = ppid
	}
	return m
}

// describe is where each process runs and what it is, from lsof.
func describe(pids []int) (cwd, comm map[int]string) {
	list := make([]string, len(pids))
	for i, p := range pids {
		list[i] = strconv.Itoa(p)
	}
	out, _ := exec.Command("lsof", "-a", "-p", strings.Join(list, ","), "-d", "cwd", "-Fpcn").Output()
	cwd, comm = map[int]string{}, map[int]string{}
	pid := 0
	for _, l := range strings.Split(string(out), "\n") {
		if l == "" {
			continue
		}
		switch l[0] {
		case 'p':
			pid, _ = strconv.Atoi(l[1:])
		case 'c':
			comm[pid] = l[1:]
		case 'n':
			cwd[pid] = l[1:]
		}
	}
	return cwd, comm
}

// mine is the processes in the project.
func (h here) mine() []proc {
	var out []proc
	for _, p := range h.procs {
		if p.Dir != "" && (within(p.Dir, h.root) || repoRoot(p.Dir) == h.root) {
			out = append(out, p)
		}
	}
	return out
}

// find is the process an address names: a :port or a pid anywhere
// conn knows, a name in the project, the caller's checkout first. No
// process, or more than one, is an error that says which.
func (h here) find(addr string) (proc, error) {
	var hits []proc
	switch {
	case strings.HasPrefix(addr, ":"):
		for _, p := range h.procs {
			for _, port := range p.Ports {
				if ":"+port == addr {
					hits = append(hits, p)
				}
			}
		}
	case isPID(addr):
		pid, _ := strconv.Atoi(addr)
		for _, p := range h.procs {
			if p.PID == pid {
				hits = append(hits, p)
			}
		}
	default:
		for _, p := range h.mine() {
			if p.Name == addr {
				hits = append(hits, p)
			}
		}
		if len(hits) > 1 {
			var here []proc
			for _, p := range hits {
				if within(p.Dir, h.tree) {
					here = append(here, p)
				}
			}
			if len(here) > 0 {
				hits = here
			}
		}
		// A run that is over gives way to one that is not.
		if len(hits) > 1 {
			var going []proc
			for _, p := range hits {
				if !strings.HasPrefix(p.State, "exited") {
					going = append(going, p)
				}
			}
			if len(going) > 0 {
				hits = going
			}
		}
	}
	switch len(hits) {
	case 0:
		where := "in " + tilde(h.root, h.home)
		if strings.HasPrefix(addr, ":") || isPID(addr) {
			where = "that conn knows"
		}
		return proc{}, fmt.Errorf("no process %s %s; conn ls lists them", addr, where)
	case 1:
		return hits[0], nil
	}
	var pids []string
	for _, p := range hits {
		pids = append(pids, strconv.Itoa(p.PID))
	}
	return proc{}, fmt.Errorf("%s names %d processes, pids %s; name one by its pid", addr, len(hits), strings.Join(pids, ", "))
}

// inProject says whether a process is the caller's to change: a verb
// that ends or starts a process reaches the project it is run in and
// no further.
func (h here) inProject(p proc) bool {
	return p.Dir != "" && (within(p.Dir, h.root) || repoRoot(p.Dir) == h.root)
}

func isPID(s string) bool {
	n, err := strconv.Atoi(s)
	return err == nil && n > 0
}

// The commands.

func cliFail(cmd string, err error) int {
	fmt.Fprintf(os.Stderr, "conn %s: %v\n", cmd, err)
	return 1
}

func cliUsage(use string) int {
	fmt.Fprintf(os.Stderr, "usage: %s\n", use)
	return 2
}

// runLs writes the processes of the project: the name each answers to,
// its pid, what it is doing, the ports it listens on, and what runs.
func runLs(w io.Writer, home, dir string, args []string) int {
	if len(args) != 0 {
		return cliUsage("conn ls")
	}
	h, err := readHere(home, dir)
	if err != nil {
		return cliFail("ls", err)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, tilde(h.root, home))
	for _, p := range h.mine() {
		command := p.Command
		if p.pane != nil && p.pane.Name != "" {
			command, _ = h.srv.Display(p.pane.ID, "#{"+runMark+"}")
		}
		at := ""
		if p.Dir != h.root {
			at = "  (" + tilde(p.Dir, home) + ")"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s%s\n", p.Name, p.PID, p.State, or(strings.Join(p.Ports, ","), "-"), flatten(command), at)
	}
	if err := tw.Flush(); err != nil {
		return cliFail("ls", err)
	}
	return 0
}

// lLines is how many lines conn l writes when not told.
const lLines = 100

// runL writes what a process has said: the end of its pane.
func runL(w io.Writer, home, dir string, args []string) int {
	const use = "conn l NAME|:PORT|PID [-n N]"
	n := lLines
	var words []string
	for i := 0; i < len(args); i++ {
		if args[i] == "-n" && i+1 < len(args) {
			v, err := strconv.Atoi(args[i+1])
			if err != nil || v < 0 {
				return cliUsage(use)
			}
			n, i = v, i+1
			continue
		}
		words = append(words, args[i])
	}
	if len(words) != 1 {
		return cliUsage(use)
	}
	h, err := readHere(home, dir)
	if err != nil {
		return cliFail("l", err)
	}
	p, err := h.find(words[0])
	if err != nil {
		return cliFail("l", err)
	}
	if p.pane == nil {
		return cliFail("l", fmt.Errorf("%s runs outside conn, so what it said went wherever whatever started it sent it", p.Name))
	}
	lines, err := said(h.srv, p.pane.ID)
	if err != nil {
		return cliFail("l", err)
	}
	if n > 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	for _, l := range lines {
		if _, err := fmt.Fprintln(w, l); err != nil {
			return cliFail("l", err)
		}
	}
	return 0
}

// said is every line a pane holds, its history and its screen, wrapped
// lines joined, less the blank rows below the last line written.
func said(srv *tmux.Server, id string) ([]string, error) {
	out, err := srv.Run("capture-pane", "-p", "-J", "-S", "-", "-E", "-", "-t", id)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines, nil
}

// A name conn u takes: a word with no space, colon or slash in it, not
// a number, so it never reads as a port or a pid.
var nameWord = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// runU starts a command under a name, in a window of conn's own at the
// working directory. -w waits for it to end, writes what it said, and
// exits with its status.
func runU(w io.Writer, home, dir string, args []string) int {
	const use = "conn u NAME [-w] -- COMMAND..."
	var command []string
	for i, a := range args {
		if a == "--" {
			args, command = args[:i], args[i+1:]
			break
		}
	}
	wait := false
	var words []string
	for _, a := range args {
		if a == "-w" {
			wait = true
			continue
		}
		words = append(words, a)
	}
	if len(words) != 1 || len(command) == 0 || !nameWord.MatchString(words[0]) || isPID(words[0]) {
		return cliUsage(use)
	}
	name := words[0]
	h, err := readHere(home, dir)
	if err != nil {
		return cliFail("u", err)
	}
	if !h.up {
		return cliFail("u", errors.New("conn's server is not up; run conn first"))
	}
	replace := ""
	if p, err := h.find(name); err == nil && p.pane != nil {
		if !strings.HasPrefix(p.State, "exited") {
			return cliFail("u", fmt.Errorf("%s is up, pid %d; conn r %s starts it again", name, p.PID, name))
		}
		replace = p.pane.ID
	}
	id, err := startNamed(h.srv, name, shellLine(command), realDir(dir))
	if err != nil {
		return cliFail("u", err)
	}
	if replace != "" {
		_, _ = h.srv.Do(tmux.Kill(replace))
	}
	if !wait {
		_, _ = fmt.Fprintf(w, "%s is up in pane %s; conn l %s reads it\n", name, id, name)
		return 0
	}
	exit, err := waitExit(h.srv, id)
	if err != nil {
		return cliFail("u", err)
	}
	lines, _ := said(h.srv, id)
	// The line the pane writes when what it ran ends is conn's, not
	// the command's.
	if len(lines) > 0 && strings.HasPrefix(lines[len(lines)-1], "["+name+" exited") {
		lines = lines[:len(lines)-1]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	for _, l := range lines {
		_, _ = fmt.Fprintln(w, l)
	}
	code, _ := strconv.Atoi(exit)
	return code
}

// shellLine is what follows -- as a line for the shell: one word is a
// line as written, pipes and all, the way an agent writes one; several
// are a command and its arguments, each quoted where it needs it.
func shellLine(words []string) string {
	if len(words) == 1 {
		return words[0]
	}
	quoted := make([]string, len(words))
	for i, w := range words {
		quoted[i] = w
		if !plainWord.MatchString(w) {
			quoted[i] = shell.Quote(w)
		}
	}
	return strings.Join(quoted, " ")
}

var plainWord = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

// startNamed starts a command under a name in a window of its own, out
// of sight, and marks it. When the command ends the pane says how,
// sets the mark that says it, and stays, so what it said can be read.
func startNamed(srv *tmux.Server, name, command, dir string) (string, error) {
	// The command runs in a subshell, so an exit in it ends the command
	// and not the line that records how it ended; and ctrl-c is caught,
	// not ignored, by that line, so it reaches the command, which takes
	// it as it would anywhere, and leaves the line to record it.
	line := "trap : INT\n(\n" + command + "\n)\n" +
		"conn_status=$?\n" +
		shell.Quote(srv.Tmux) + " -S " + shell.Quote(srv.Socket) + " set-option -p -t \"$TMUX_PANE\" " + exitMark + " \"$conn_status\"\n" +
		"printf '\\n[%s exited %s]\\n' " + shell.Quote(name) + " \"$conn_status\"\n" +
		shell.HoldOpen
	out, err := srv.Run("new-window", "-d", "-t", sessionName+":", "-n", name, "-c", dir, "-P", "-F", "#{pane_id}", line)
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(out)
	_, err = srv.Do(tmux.SetPane(id, nameMark, name), tmux.SetPane(id, runMark, command), tmux.SetPane(id, dirMark, dir))
	return id, err
}

// runPoll is how often a wait looks for the end of what it waits on.
const runPoll = 200 * time.Millisecond

// waitExit waits for what a pane runs to end, and answers the status
// it set. A pane that went without setting one was ended from outside.
func waitExit(srv *tmux.Server, id string) (string, error) {
	for {
		exit, gone := paneExit(srv, id)
		switch {
		case gone:
			return "", errors.New("its pane was closed before it said how it ended")
		case exit != "":
			return exit, nil
		}
		time.Sleep(runPoll)
	}
}

// paneExit is how what a pane runs ended, blank while it runs, and
// whether the pane is gone. The pane list is read, since tmux asked of
// a pane that is gone answers blank and no error.
func paneExit(srv *tmux.Server, id string) (exit string, gone bool) {
	ps, err := panes(srv)
	if err != nil {
		return "", true
	}
	for _, p := range ps {
		if p.ID == id {
			return p.Exit, false
		}
	}
	return "", true
}

// actOn finds the process an acting verb's address names, in the
// project it is run in.
func actOn(cmd, home, dir string, args []string) (proc, here, int) {
	if len(args) != 1 {
		return proc{}, here{}, cliUsage("conn " + cmd + " NAME|:PORT|PID")
	}
	h, err := readHere(home, dir)
	if err != nil {
		return proc{}, here{}, cliFail(cmd, err)
	}
	p, err := h.find(args[0])
	if err != nil {
		return proc{}, here{}, cliFail(cmd, err)
	}
	if !h.inProject(p) {
		return proc{}, here{}, cliFail(cmd, fmt.Errorf("%s runs in %s, not in %s; conn %s reaches only the project it is run in", args[0], tilde(p.Dir, home), tilde(h.root, home), cmd))
	}
	return p, h, -1
}

// runX ends the process an address names.
func runX(w io.Writer, home, dir string, args []string) int {
	p, h, code := actOn("x", home, dir, args)
	if code >= 0 {
		return code
	}
	how, err := endProc(h.srv, p)
	if err != nil {
		return cliFail("x", err)
	}
	_, _ = fmt.Fprintf(w, "%s ended: %s\n", p.Name, how)
	return 0
}

// closeWait is how long ctrl-c is given to end what a pane runs.
const closeWait = 5 * time.Second

// endProc ends a process and says how. What conn u started and is
// running is sent ctrl-c, as a hand would, and its pane closed once it
// has ended; any other pane is closed, a conversation's included, which
// keeps its transcript to be picked back up. A listener outside conn is
// sent SIGTERM.
func endProc(srv *tmux.Server, p proc) (string, error) {
	switch {
	case p.pane == nil:
		if err := syscall.Kill(p.PID, syscall.SIGTERM); err != nil {
			return "", fmt.Errorf("%s: %v", p.Name, err)
		}
		return "kill -TERM " + strconv.Itoa(p.PID), nil
	case p.pane.Name == "" || p.pane.Exit != "":
		return "kill-pane " + p.pane.ID, srv.ClosePane(p.pane.ID)
	}
	if err := srv.Interrupt(p.pane.ID); err != nil {
		return "", err
	}
	for deadline := time.Now().Add(closeWait); time.Now().Before(deadline); time.Sleep(runPoll) {
		switch exit, gone := paneExit(srv, p.pane.ID); {
		case gone:
			return "send-keys C-c", nil
		case exit != "":
			return "send-keys C-c", srv.ClosePane(p.pane.ID)
		}
	}
	return "kill-pane " + p.pane.ID + ", " + closeWait.String() + " after ctrl-c did not end it", srv.ClosePane(p.pane.ID)
}

// runR starts a process again.
func runR(w io.Writer, home, dir string, args []string) int {
	p, h, code := actOn("r", home, dir, args)
	if code >= 0 {
		return code
	}
	self, err := os.Executable()
	if err != nil {
		return cliFail("r", err)
	}
	id, err := restartProc(h.srv, connCommand(self), p)
	if err != nil {
		return cliFail("r", err)
	}
	_, _ = fmt.Fprintf(w, "%s restarted in pane %s\n", p.Name, id)
	return 0
}

// restartProc starts a process again, as it was started, and answers
// its pane. What conn u started is ended and started anew; a
// conversation is picked back up in its pane; any other pane runs again
// what it was opened with. A process outside conn was started nobody
// knows how. conn is how agents run this build.
func restartProc(srv *tmux.Server, conn string, p proc) (string, error) {
	switch {
	case p.pane == nil:
		return "", fmt.Errorf("%s was started outside conn, and conn does not know how; x ends it, and u starts a command in conn", p.Name)
	case p.pane.Name != "":
		command, _ := srv.Display(p.pane.ID, "#{"+runMark+"}")
		at, _ := srv.Display(p.pane.ID, "#{"+dirMark+"}")
		if command == "" {
			return "", fmt.Errorf("%s does not say what it was started with", p.Name)
		}
		if _, err := endProc(srv, p); err != nil {
			return "", err
		}
		return startNamed(srv, p.pane.Name, command, or(at, p.Dir))
	case p.Session != "":
		_, err := srv.Do(tmux.Respawn(p.pane.ID, resumeCommand(conn, p.Session)), tmux.SetPane(p.pane.ID, sessionMark, p.Session))
		return p.pane.ID, err
	}
	_, err := srv.Do(tmux.Cmd{"respawn-pane", "-k", "-t", p.pane.ID})
	return p.pane.ID, err
}
