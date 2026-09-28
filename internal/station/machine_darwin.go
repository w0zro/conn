package station

import (
	"os"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// readMachine asks sysctl, one name at a time, and pmset and csrutil for
// what sysctl does not have. A name that goes unanswered leaves its
// field zero.
func readMachine() Machine {
	m := Machine{Available: -1, Power: Power{Percent: -1}, Page: os.Getpagesize()}
	if v, err := unix.Sysctl("kern.osproductversion"); err == nil {
		m.System = "macOS " + v
		m.SystemBuild, _ = unix.Sysctl("kern.osversion")
	}
	if v, err := unix.Sysctl("kern.osrelease"); err == nil {
		m.Kernel = "Darwin " + v
	}
	m.Model, _ = unix.Sysctl("hw.model")
	m.Processor, _ = unix.Sysctl("machdep.cpu.brand_string")
	m.Processor = strings.TrimSpace(m.Processor)
	if n, err := unix.SysctlUint32("hw.ncpu"); err == nil {
		m.CPUs = int(n)
	}
	p, perr := unix.SysctlUint32("hw.perflevel0.physicalcpu")
	e, eerr := unix.SysctlUint32("hw.perflevel1.physicalcpu")
	if perr == nil && eerr == nil && e > 0 {
		m.PerfCores, m.EffCores = int(p), int(e)
	}
	if t, err := unix.SysctlUint32("sysctl.proc_translated"); err == nil && t == 1 {
		m.Rosetta = true
	}
	m.Memory, _ = unix.SysctlUint64("hw.memsize")
	// kern.memorystatus_level stood here and was read as the memory
	// available to new work. It is not that: it counts the pages work
	// is actively holding among the available ones, and read 83 on a
	// machine with a sixteenth of its memory free and most of its swap
	// in use. The classes are counted instead, which is what the label
	// has always claimed.
	if free, ok := parseVMStat(run("vm_stat")); ok && m.Memory > 0 {
		m.Available = int(free * 100 / m.Memory)
	}
	if level, err := unix.SysctlUint32("kern.memorystatus_vm_pressure_level"); err == nil {
		switch level {
		case 1:
			m.Pressure = PressureNormal
		case 2:
			m.Pressure = PressureWarning
		case 4:
			m.Pressure = PressureCritical
		}
	}
	if raw, err := unix.SysctlRaw("vm.swapusage"); err == nil {
		if total, used, encrypted, ok := parseSwapUsage(raw); ok {
			m.SwapTotal, m.SwapUsed, m.SwapEncrypt = total, used, encrypted
		}
	}
	if tv, err := unix.SysctlTimeval("kern.boottime"); err == nil {
		m.Booted = time.Unix(tv.Sec, 0)
	}
	if raw, err := unix.SysctlRaw("vm.loadavg"); err == nil {
		if load, ok := parseLoadavg(raw); ok {
			m.Load, m.LoadRead = load, true
		}
	}
	if procs, err := unix.SysctlKinfoProcSlice("kern.proc.all"); err == nil {
		m.Processes = len(procs)
	}
	m.Power = parsePmset(run("pmset", "-g", "batt"))
	m.SIP = parseCSRUtil(run("csrutil", "status"))
	return m
}

// volumeType is the file system holding a path, as the kernel names it.
func volumeType(path string) string {
	var st unix.Statfs_t
	if unix.Statfs(path, &st) != nil {
		return ""
	}
	return unix.ByteSliceToString(st.Fstypename[:])
}

// defaultRoute is the interface the machine reaches everything else
// through, and whether the table could be read at all. route answers
// for the whole table in one call and names the interface outright.
func defaultRoute() (string, bool) {
	out := run("route", "-n", "get", "default")
	if out == "" {
		return "", false
	}
	return parseRouteGet(out), true
}

// readTools is what conn needs on this platform past the kernel: tmux,
// to hold the work, and lsof, for the working directories.
func readTools() []Tool {
	return []Tool{{Name: "tmux", Path: LookPath("tmux")}, {Name: "lsof", Path: LookPath("lsof")}}
}
