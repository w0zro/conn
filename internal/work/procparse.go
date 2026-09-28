package work

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// The parsers behind the process table, pure over what the platform
// processes over, and tested on every platform against captures.

// parseLsof reads lsof -F pcn: for each process, a p line with its pid,
// a c line with its command, and an n line with the path of the file
// asked for, which here is its working directory.
func parseLsof(out string) map[int]Process {
	procs := map[int]Process{}
	var cur Process
	flush := func() {
		if cur.PID > 0 {
			procs[cur.PID] = cur
		}
	}
	for _, l := range strings.Split(out, "\n") {
		if l == "" {
			continue
		}
		switch l[0] {
		case 'p':
			flush()
			pid, _ := strconv.Atoi(l[1:])
			cur = Process{PID: pid}
		case 'c':
			cur.Command = l[1:]
		case 'n':
			cur.Cwd = l[1:]
		}
	}
	flush()
	return procs
}

// parseProcargs reads what sysctl kern.procargs2 returns for a process:
// the count of arguments, the path it was executed as, padding, then the
// arguments, each ended by NUL, with the environment after them.
func parseProcargs(raw []byte) []string {
	if len(raw) < 4 {
		return nil
	}
	argc := int(binary.LittleEndian.Uint32(raw[:4]))
	rest := raw[4:]
	if i := strings.IndexByte(string(rest), 0); i >= 0 {
		rest = rest[i:]
	}
	for len(rest) > 0 && rest[0] == 0 {
		rest = rest[1:]
	}
	var args []string
	for len(args) < argc && len(rest) > 0 {
		i := strings.IndexByte(string(rest), 0)
		if i < 0 {
			args = append(args, string(rest))
			break
		}
		args = append(args, string(rest[:i]))
		rest = rest[i+1:]
	}
	return args
}

// parseProcStat reads /proc/<pid>/stat: the pid, the command in
// parentheses, which may hold spaces and parentheses of its own, then
// the fields by position — state, ppid, pgrp, session, tty_nr, tpgid,
// and at the twenty-second the start, in ticks since boot.
func parseProcStat(line string, boot time.Time, hz int) (Process, bool) {
	open, closeParen := strings.IndexByte(line, '('), strings.LastIndexByte(line, ')')
	if open < 0 || closeParen < open {
		return Process{}, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(line[:open]))
	if err != nil {
		return Process{}, false
	}
	f := strings.Fields(line[closeParen+1:])
	if len(f) < 20 {
		return Process{}, false
	}
	p := Process{PID: pid, Command: line[open+1 : closeParen], State: f[0][0]}
	p.PPID, _ = strconv.Atoi(f[1])
	p.PGID, _ = strconv.Atoi(f[2])
	ttyNr, _ := strconv.Atoi(f[4])
	tpgid, _ := strconv.Atoi(f[5])
	p.TTY = linuxTTY(ttyNr)
	p.Foreground = tpgid == p.PGID
	if ticks, err := strconv.ParseInt(f[19], 10, 64); err == nil && hz > 0 && !boot.IsZero() {
		p.Started = boot.Add(time.Duration(ticks) * time.Second / time.Duration(hz))
	}
	// utime and stime, fields 14 and 15, which are f[11] and f[12] here:
	// what the process has spent on a processor, in and out of the
	// kernel, all of it since it started.
	if hz > 0 {
		user, uerr := strconv.ParseInt(f[11], 10, 64)
		sys, serr := strconv.ParseInt(f[12], 10, 64)
		if uerr == nil && serr == nil {
			p.CPU = time.Duration(user+sys) * time.Second / time.Duration(hz)
		}
	}
	return p, true
}

// parsePsTimes reads what `ps -axo pid=,time=` printed: a pid and the
// processor time it has used, one process to a line.
func parsePsTimes(out string) map[int]time.Duration {
	times := map[int]time.Duration{}
	for line := range strings.SplitSeq(out, "\n") {
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		if d, ok := parsePsTime(f[1]); ok {
			times[pid] = d
		}
	}
	return times
}

// parsePsTime reads one of ps's elapsed times: seconds at the end,
// minutes and then hours before it, and days off the front behind a
// dash. The minutes can run past sixty on macOS, where nothing larger
// is printed, so no field is held to its usual range.
func parsePsTime(s string) (time.Duration, bool) {
	var total time.Duration
	if days, rest, ok := strings.Cut(s, "-"); ok {
		n, err := strconv.ParseFloat(days, 64)
		if err != nil {
			return 0, false
		}
		total += time.Duration(n * float64(24*time.Hour))
		s = rest
	}
	parts := strings.Split(s, ":")
	if len(parts) > 3 {
		return 0, false
	}
	// From the right: seconds, minutes, hours.
	unit := []time.Duration{time.Second, time.Minute, time.Hour}
	for i := range parts {
		n, err := strconv.ParseFloat(parts[len(parts)-1-i], 64)
		if err != nil {
			return 0, false
		}
		total += time.Duration(n * float64(unit[i]))
	}
	return total, true
}

// linuxTTY names the terminal behind a tty_nr: a pseudo-terminal under
// pts, a console under tty, or nothing.
func linuxTTY(nr int) string {
	if nr == 0 {
		return ""
	}
	major := (nr >> 8) & 0xfff
	minor := (nr & 0xff) | ((nr >> 12) & 0xfff00)
	switch {
	case major >= 136 && major <= 143:
		return "pts/" + strconv.Itoa(minor+(major-136)*256)
	case major == 4:
		return "tty" + strconv.Itoa(minor)
	default:
		return ""
	}
}

// readProcTree reads the process table off a proc file system at root:
// every numbered directory's stat, cmdline and cwd, and the owner of the
// directory for the uid. A process that goes away between the listing
// and the reading is left out.
func readProcTree(root string, boot time.Time, hz int) []Process {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	tables := procSocketTables(root)
	var procs []Process
	for _, e := range entries {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		dir := filepath.Join(root, e.Name())
		stat, err := os.ReadFile(filepath.Join(dir, "stat"))
		if err != nil {
			continue
		}
		p, ok := parseProcStat(strings.TrimSpace(string(stat)), boot, hz)
		if !ok {
			continue
		}
		p.UID = ownerOf(dir)
		if cmd, err := os.ReadFile(filepath.Join(dir, "cmdline")); err == nil && len(cmd) > 0 {
			p.Args = strings.Split(strings.TrimRight(string(cmd), "\x00"), "\x00")
		}
		p.Cwd, _ = os.Readlink(filepath.Join(dir, "cwd"))
		p.Sockets = fdSockets(dir, tables)
		procs = append(procs, p)
	}
	return procs
}

// ownerOf is the uid that owns a file, or -1.
func ownerOf(path string) int {
	fi, err := os.Stat(path)
	if err != nil {
		return -1
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return int(st.Uid)
	}
	return -1
}

// parseBootTime reads btime out of /proc/stat.
func parseBootTime(stat string) time.Time {
	for _, l := range strings.Split(stat, "\n") {
		if v, ok := strings.CutPrefix(l, "btime "); ok {
			if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
				return time.Unix(n, 0)
			}
		}
	}
	return time.Time{}
}
