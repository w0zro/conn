package work

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

// ReadProcesses reads the process table: sysctl for every process's
// parent, group, owner, terminal, state and start; lsof for the working
// directory and name of each of the user's, which sysctl does not have;
// and kern.procargs2 for what each of the user's was started as.
func ReadProcesses(uid int) ([]Process, error) {
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
	dirs := ParseLsof(out)
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
	cpu := ParsePsTimes(out)
	// And what each has open to the world: lsof again, for the internet
	// sockets and for the unix ones, which it will not list in one
	// breath with the working directories. A listing that fails here
	// costs the reading the sockets and nothing else: a row is a row
	// without them.
	sockets := map[int][]Socket{}
	if out, err := listing("lsof", "-nP", "-u", strconv.Itoa(uid), "-a", "-i", "-F", "pcnPT"); err == nil {
		sockets = ParseSockets(out)
	}
	if out, err := listing("lsof", "-nP", "-u", strconv.Itoa(uid), "-a", "-U", "-F", "pcn"); err == nil {
		for pid, held := range ParseUnixSockets(out) {
			sockets[pid] = append(sockets[pid], held...)
		}
	}
	ttys := ttyNames()
	procs := make([]Process, 0, len(kinfo))
	for _, k := range kinfo {
		p := Process{
			PID:     int(k.Proc.P_pid),
			PPID:    int(k.Eproc.Ppid),
			PGID:    int(k.Eproc.Pgid),
			UID:     int(k.Eproc.Ucred.Uid),
			Started: time.Unix(k.Proc.P_starttime.Sec, int64(k.Proc.P_starttime.Usec)*1000),
			State:   darwinState(k.Proc.P_stat),
		}
		if uint32(k.Eproc.Tdev) != 0xffffffff {
			p.TTY = ttys[uint32(k.Eproc.Tdev)]
			p.Foreground = k.Eproc.Tpgid == k.Eproc.Pgid
		}
		if d, ok := dirs[p.PID]; ok {
			p.Cwd, p.Command = d.Cwd, d.Command
		}
		// lsof lists nothing for a process that has ended and not been
		// collected, there being no directory left to list, and its
		// arguments are gone with it; the kernel still has its name,
		// and a row that says sleep and ENDED is a row that can be
		// read, where a row with no name was not.
		if p.Command == "" {
			p.Command = unix.ByteSliceToString(k.Proc.P_comm[:])
		}
		p.CPU = cpu[p.PID]
		p.Sockets = sockets[p.PID]
		if p.UID == uid {
			if raw, err := unix.SysctlRaw("kern.procargs2", p.PID); err == nil {
				p.Args = ParseProcargs(raw)
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
	return ListingWithin(listingTimeout, name, args...)
}

func ListingWithin(timeout time.Duration, name string, args ...string) (string, error) {
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
