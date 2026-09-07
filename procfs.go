package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Linux keeps every process on a filesystem, and conn reads it there: the
// working directory is a link, the command line a file, the parent, the
// group, the state and the start time one line of stat, and the listening
// sockets a table in net/tcp joined to the process by the inode its fd
// links name. It is what lsof would read for conn, read without lsof —
// which the images conn runs in as a devcontainer do not have, and whose
// ps may be busybox's, with no start-time column to ask for. The scan is
// written against a root so a test can lay out a /proc of its own.

// procfsScan reads the processes under root that the user may read — a
// process another user owns denies access to its cwd, and is not work in
// a repository conn could reach — but for self and its children, which
// the scan leaves out.
func procfsScan(root string, self int) ([]Proc, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	listening := procfsListeners(root)
	var procs []Proc
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || !e.IsDir() || pid == self {
			continue
		}
		dir := filepath.Join(root, e.Name())
		cwd, err := os.Readlink(filepath.Join(dir, "cwd"))
		if err != nil {
			continue
		}
		stat, err := os.ReadFile(filepath.Join(dir, "stat"))
		if err != nil {
			continue
		}
		p, ok := parseProcStat(string(stat))
		if !ok || p.PPID == self {
			continue
		}
		p.Dir = cwd
		if cmd, err := os.ReadFile(filepath.Join(dir, "cmdline")); err == nil {
			p.Argv = strings.TrimSpace(strings.ReplaceAll(strings.TrimRight(string(cmd), "\x00"), "\x00", " "))
		}
		p.Ports = procfsPorts(dir, listening)
		procs = append(procs, p)
	}
	return procs, nil
}

// parseProcStat reads one process's stat line: the pid, the command in
// parentheses — which may hold spaces and parentheses of its own, so it
// ends at the last one — then the state, the parent, the group, and, as
// the twentieth field after the command, the start time in clock ticks
// since boot, which is the token that tells a pid's process from the one
// that had the number before.
func parseProcStat(line string) (Proc, bool) {
	open := strings.IndexByte(line, '(')
	close := strings.LastIndexByte(line, ')')
	if open < 0 || close < open {
		return Proc{}, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(line[:open]))
	if err != nil {
		return Proc{}, false
	}
	fields := strings.Fields(line[close+1:])
	if len(fields) < 20 {
		return Proc{}, false
	}
	ppid, _ := strconv.Atoi(fields[1])
	pgid, _ := strconv.Atoi(fields[2])
	return Proc{
		PID:     pid,
		PPID:    ppid,
		PGID:    pgid,
		Command: line[open+1 : close],
		State:   fields[0],
		Started: fields[19],
	}, true
}

// procfsListeners is every listening TCP socket under root, by the inode
// its owner's fd links name, to the port it listens on.
func procfsListeners(root string) map[string]string {
	listening := map[string]string{}
	for _, table := range []string{"tcp", "tcp6"} {
		b, err := os.ReadFile(filepath.Join(root, "net", table))
		if err != nil {
			continue
		}
		for inode, port := range parseNetTCP(string(b)) {
			listening[inode] = port
		}
	}
	return listening
}

// tcpListen is the state a listening socket is in, in net/tcp's terms.
const tcpListen = "0A"

// parseNetTCP reads a net/tcp table: after the header, one socket a
// line — its local address as hex host and port, its state, and its
// inode tenth — keeping the ones that listen.
func parseNetTCP(table string) map[string]string {
	out := map[string]string{}
	for i, line := range strings.Split(table, "\n") {
		fields := strings.Fields(line)
		if i == 0 || len(fields) < 10 || fields[3] != tcpListen {
			continue
		}
		_, hexPort, ok := strings.Cut(fields[1], ":")
		if !ok {
			continue
		}
		port, err := strconv.ParseInt(hexPort, 16, 32)
		if err != nil {
			continue
		}
		out[fields[9]] = strconv.Itoa(int(port))
	}
	return out
}

// procfsPorts is what a process listens on: its fds that are sockets,
// looked up among the listeners by inode, lowest port first.
func procfsPorts(dir string, listening map[string]string) []string {
	if len(listening) == 0 {
		return nil
	}
	fds, err := os.ReadDir(filepath.Join(dir, "fd"))
	if err != nil {
		return nil
	}
	var ports []string
	for _, fd := range fds {
		target, err := os.Readlink(filepath.Join(dir, "fd", fd.Name()))
		if err != nil {
			continue
		}
		inode, ok := strings.CutPrefix(target, "socket:[")
		if !ok {
			continue
		}
		if port, ok := listening[strings.TrimSuffix(inode, "]")]; ok && !contains(ports, port) {
			ports = append(ports, port)
		}
	}
	sortPorts(ports)
	return ports
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// procfsStarted is when each of the given processes began, as their stat
// lines say now, for a kill to compare with what the scan saw. A process
// gone from the table is absent.
func procfsStarted(root string, pids []int) map[int]string {
	table := map[int]string{}
	for _, pid := range pids {
		b, err := os.ReadFile(filepath.Join(root, strconv.Itoa(pid), "stat"))
		if err != nil {
			continue
		}
		if p, ok := parseProcStat(string(b)); ok {
			table[pid] = p.Started
		}
	}
	return table
}
