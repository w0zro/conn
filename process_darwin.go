package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// readProcesses reads the process table: sysctl for every process's
// parent, group, owner, terminal, state and start; lsof for the working
// directory and name of each of the user's, which sysctl does not have;
// and kern.procargs2 for what each of the user's was started as.
func readProcesses(uid int) ([]process, error) {
	kinfo, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, err
	}
	// A listing that did not come back is a reading that failed, and is
	// said so rather than read as a table in which nothing has a
	// directory: every project would be NO PROJECT and the processes view
	// would be wholly wrong while looking wholly true.
	out, err := listing("lsof", "-nP", "-u", strconv.Itoa(uid), "-a", "-d", "cwd", "-F", "pcn")
	if err != nil {
		return nil, fmt.Errorf("lsof: %w", err)
	}
	dirs := parseLsof(out)
	if len(dirs) == 0 {
		return nil, errors.New("lsof answered for no process")
	}
	// The processor time each has used. kinfo_proc carries no such
	// thing conn can rely on, so ps is asked, the way lsof is asked for
	// the working directories.
	out, err = listing("ps", "-axo", "pid=,time=")
	if err != nil {
		return nil, fmt.Errorf("ps: %w", err)
	}
	cpu := parsePsTimes(out)
	// And what each has open to the world: lsof again, for the internet
	// sockets and for the unix ones, which it will not list in one
	// breath with the working directories. A listing that fails here
	// costs the reading the sockets and nothing else: a row is a row
	// without them.
	sockets := map[int][]socket{}
	if out, err := listing("lsof", "-nP", "-u", strconv.Itoa(uid), "-a", "-i", "-F", "pcnPT"); err == nil {
		sockets = parseSockets(out)
	}
	if out, err := listing("lsof", "-nP", "-u", strconv.Itoa(uid), "-a", "-U", "-F", "pcn"); err == nil {
		for pid, held := range parseUnixSockets(out) {
			sockets[pid] = append(sockets[pid], held...)
		}
	}
	ttys := ttyNames()
	procs := make([]process, 0, len(kinfo))
	for _, k := range kinfo {
		p := process{
			pid:     int(k.Proc.P_pid),
			ppid:    int(k.Eproc.Ppid),
			pgid:    int(k.Eproc.Pgid),
			uid:     int(k.Eproc.Ucred.Uid),
			started: time.Unix(k.Proc.P_starttime.Sec, int64(k.Proc.P_starttime.Usec)*1000),
			state:   darwinState(k.Proc.P_stat),
		}
		if uint32(k.Eproc.Tdev) != 0xffffffff {
			p.tty = ttys[uint32(k.Eproc.Tdev)]
			p.foreground = k.Eproc.Tpgid == k.Eproc.Pgid
		}
		if d, ok := dirs[p.pid]; ok {
			p.cwd, p.command = d.cwd, d.command
		}
		// lsof lists nothing for a process that has ended and not been
		// collected, there being no directory left to list, and its
		// arguments are gone with it; the kernel still has its name,
		// and a row that says sleep and ENDED is a row that can be
		// read, where a row with no name was not.
		if p.command == "" {
			p.command = unix.ByteSliceToString(k.Proc.P_comm[:])
		}
		p.cpu = cpu[p.pid]
		p.sockets = sockets[p.pid]
		if p.uid == uid {
			if raw, err := unix.SysctlRaw("kern.procargs2", p.pid); err == nil {
				p.args = parseProcargs(raw)
			}
		}
		procs = append(procs, p)
	}
	return procs, nil
}

// darwinState is the kernel's process state as a letter: SIDL, SRUN,
// SSLEEP, SSTOP, SZOMB.
func darwinState(stat int8) byte {
	switch stat {
	case 1:
		return 'I'
	case 2:
		return 'R'
	case 3:
		return 'S'
	case 4:
		return 'T'
	case 5:
		return 'Z'
	default:
		return '?'
	}
}

// ttyNames is every terminal device under /dev by its device number.
func ttyNames() map[uint32]string {
	names := map[uint32]string{}
	entries, _ := os.ReadDir("/dev")
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "tty") {
			continue
		}
		fi, err := os.Stat("/dev/" + e.Name())
		if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
			continue
		}
		if st, ok := fi.Sys().(*syscall.Stat_t); ok {
			names[uint32(st.Rdev)] = e.Name()
		}
	}
	return names
}

// readTools is what conn needs on this platform past the kernel: tmux,
// to hold the work, and lsof, for the working directories.
func readTools() []tool {
	return []tool{{name: "tmux", path: lookPath("tmux")}, {name: "lsof", path: lookPath("lsof")}}
}

// listingTimeout bounds a listing. lsof answers in tens of milliseconds
// on a healthy machine; the bound is for the machine with a dead
// network mount, where it hangs, and the processes view must come back
// even so.
const listingTimeout = 5 * time.Second

// listing is what a program prints when asked for a list, kept even when
// it exits in complaint: lsof exits nonzero when any one process denies
// it, which says nothing of the ones that answered. What is an error is
// a program that is not there, or one that did not answer in time.
// WaitDelay is for the process the timeout's kill does not take on.
func listing(name string, args ...string) (string, error) {
	return listingWithin(listingTimeout, name, args...)
}

func listingWithin(timeout time.Duration, name string, args ...string) (string, error) {
	if _, err := exec.LookPath(name); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = 2 * time.Second
	out, _ := cmd.Output()
	if ctx.Err() != nil {
		return "", fmt.Errorf("gave no answer in %s", timeout)
	}
	return string(out), nil
}
