package main

import (
	"os"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// readMachine asks sysctl, one name at a time, and pmset and csrutil for
// what sysctl does not have. A name that goes unanswered leaves its
// field zero.
func readMachine() machine {
	m := machine{available: -1, power: power{percent: -1}, page: os.Getpagesize()}
	if v, err := unix.Sysctl("kern.osproductversion"); err == nil {
		m.system = "macOS " + v
		m.systemBuild, _ = unix.Sysctl("kern.osversion")
	}
	if v, err := unix.Sysctl("kern.osrelease"); err == nil {
		m.kernel = "Darwin " + v
	}
	m.model, _ = unix.Sysctl("hw.model")
	m.processor, _ = unix.Sysctl("machdep.cpu.brand_string")
	m.processor = strings.TrimSpace(m.processor)
	if n, err := unix.SysctlUint32("hw.ncpu"); err == nil {
		m.cpus = int(n)
	}
	p, perr := unix.SysctlUint32("hw.perflevel0.physicalcpu")
	e, eerr := unix.SysctlUint32("hw.perflevel1.physicalcpu")
	if perr == nil && eerr == nil && e > 0 {
		m.perfCores, m.effCores = int(p), int(e)
	}
	if t, err := unix.SysctlUint32("sysctl.proc_translated"); err == nil && t == 1 {
		m.rosetta = true
	}
	m.memory, _ = unix.SysctlUint64("hw.memsize")
	// kern.memorystatus_level stood here and was read as the memory
	// available to new work. It is not that: it counts the pages work
	// is actively holding among the available ones, and read 83 on a
	// machine with a sixteenth of its memory free and most of its swap
	// in use. The classes are counted instead, which is what the label
	// has always claimed.
	if free, ok := parseVMStat(run("vm_stat")); ok && m.memory > 0 {
		m.available = int(free * 100 / m.memory)
	}
	if level, err := unix.SysctlUint32("kern.memorystatus_vm_pressure_level"); err == nil {
		switch level {
		case 1:
			m.pressure = pressureNormal
		case 2:
			m.pressure = pressureWarning
		case 4:
			m.pressure = pressureCritical
		}
	}
	if raw, err := unix.SysctlRaw("vm.swapusage"); err == nil {
		if total, used, encrypted, ok := parseSwapUsage(raw); ok {
			m.swapTotal, m.swapUsed, m.swapEncrypt = total, used, encrypted
		}
	}
	if tv, err := unix.SysctlTimeval("kern.boottime"); err == nil {
		m.booted = time.Unix(tv.Sec, 0)
	}
	if raw, err := unix.SysctlRaw("vm.loadavg"); err == nil {
		if load, ok := parseLoadavg(raw); ok {
			m.load = load
		}
	}
	if procs, err := unix.SysctlKinfoProcSlice("kern.proc.all"); err == nil {
		m.processes = len(procs)
	}
	m.power = parsePmset(run("pmset", "-g", "batt"))
	switch out := run("csrutil", "status"); {
	case strings.Contains(out, "enabled"):
		m.sip = "enabled"
	case strings.Contains(out, "disabled"):
		m.sip = "disabled"
	}
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
