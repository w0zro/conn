package main

import (
	"context"
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
	dirs := parseLsof(listing("lsof", "-nP", "-u", strconv.Itoa(uid), "-a", "-d", "cwd", "-F", "pcn"))
	// The processor time each has used. kinfo_proc carries no such
	// thing conn can rely on, so ps is asked, the way lsof is asked for
	// the working directories.
	cpu := parsePsTimes(listing("ps", "-axo", "pid=,time="))
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
		p.cpu = cpu[p.pid]
		if p.uid == uid && p.tty != "" {
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
// on a healthy machine; the bound is for the machine with a dead network
// mount, where it hangs, and the watch must come back even so.
const listingTimeout = 5 * time.Second

// listing is what a program prints when asked for a list, kept even when
// it exits in complaint: lsof exits nonzero when any one process denies
// it, which says nothing of the ones that answered. WaitDelay is for the
// process the timeout's kill does not take on.
func listing(name string, args ...string) string {
	if _, err := exec.LookPath(name); err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), listingTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = 2 * time.Second
	out, _ := cmd.Output()
	return string(out)
}
