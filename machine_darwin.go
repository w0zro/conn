package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// readMachine asks sysctl, one name at a time, and pmset and csrutil for
// what sysctl does not have; a name that goes unanswered leaves its field
// blank and the line off the report.
func readMachine() machine {
	m := machine{available: -1}
	if v, err := unix.Sysctl("kern.osproductversion"); err == nil {
		m.system = "macOS " + v
		if b, err := unix.Sysctl("kern.osversion"); err == nil {
			m.system += " (" + b + ")"
		}
	}
	if v, err := unix.Sysctl("kern.osrelease"); err == nil {
		m.kernel = "Darwin " + v
	}
	m.model, _ = unix.Sysctl("hw.model")
	m.processor, _ = unix.Sysctl("machdep.cpu.brand_string")
	m.processor = strings.TrimSpace(m.processor)
	if n, err := unix.SysctlUint32("hw.ncpu"); err == nil {
		m.cores = strconv.Itoa(int(n)) + " CORES"
		p, perr := unix.SysctlUint32("hw.perflevel0.physicalcpu")
		e, eerr := unix.SysctlUint32("hw.perflevel1.physicalcpu")
		if perr == nil && eerr == nil && e > 0 {
			m.cores += fmt.Sprintf(" (%dP + %dE)", p, e)
		}
	}
	if t, err := unix.SysctlUint32("sysctl.proc_translated"); err == nil && t == 1 {
		m.cores = join(" · ", m.cores, "UNDER ROSETTA")
	}
	m.memory, _ = unix.SysctlUint64("hw.memsize")
	if level, err := unix.SysctlUint32("kern.memorystatus_level"); err == nil {
		m.available = int(level)
	}
	// vm.swapusage is three 64-bit sizes — total, available, used — the
	// page size, and whether the swap is encrypted.
	if raw, err := unix.SysctlRaw("vm.swapusage"); err == nil && len(raw) >= 32 {
		m.swapTotal = binary.LittleEndian.Uint64(raw[0:8])
		m.swapUsed = binary.LittleEndian.Uint64(raw[16:24])
		if binary.LittleEndian.Uint32(raw[28:32]) != 0 {
			m.swapNote = "ENCRYPTED"
		}
	}
	if tv, err := unix.SysctlTimeval("kern.boottime"); err == nil {
		m.booted = time.Unix(tv.Sec, 0)
	}
	// vm.loadavg is three fixed-point words and the scale they are in.
	if raw, err := unix.SysctlRaw("vm.loadavg"); err == nil && len(raw) >= 24 {
		scale := float64(binary.LittleEndian.Uint64(raw[16:24]))
		if scale > 0 {
			for i := range m.load {
				m.load[i] = float64(binary.LittleEndian.Uint32(raw[i*4:i*4+4])) / scale
			}
		}
	}
	if procs, err := unix.SysctlKinfoProcSlice("kern.proc.all"); err == nil {
		m.processes = len(procs)
	}
	m.power, m.powerLow = batteryStatus()
	if out := command("csrutil", "status"); out != "" {
		switch {
		case strings.Contains(out, "enabled"):
			m.extra = append(m.extra, fact{"SIP", "ENABLED"})
		case strings.Contains(out, "disabled"):
			m.extra = append(m.extra, fact{"SIP", "DISABLED"})
		}
	}
	return m
}

// batteryStatus reads pmset: what the machine draws from and, with a
// battery, its charge and whether it is charging.
func batteryStatus() (string, bool) {
	out := command("pmset", "-g", "batt")
	if out == "" {
		return "", false
	}
	source := "AC POWER"
	if strings.Contains(out, "'Battery Power'") {
		source = "BATTERY"
	}
	for _, l := range strings.Split(out, "\n") {
		if !strings.Contains(l, "InternalBattery") {
			continue
		}
		fields := strings.Split(l, "\t")
		if len(fields) < 2 {
			continue
		}
		parts := strings.Split(fields[1], ";")
		pct := strings.TrimSpace(parts[0])
		state := ""
		if len(parts) > 1 {
			state = strings.ToUpper(strings.TrimSpace(parts[1]))
		}
		n, _ := strconv.Atoi(strings.TrimSuffix(pct, "%"))
		low := n < 10 && state == "DISCHARGING"
		remaining := ""
		if len(parts) > 2 {
			if r, _, ok := strings.Cut(strings.TrimSpace(parts[2]), " remaining"); ok && !strings.Contains(r, "(no") {
				remaining = r + " LEFT"
			}
		}
		return join(" · ", source, pct, state, remaining), low
	}
	return source, false
}

// volumeType is the file system holding a path, as the kernel names it.
func volumeType(path string) string {
	var st unix.Statfs_t
	if path == "" || unix.Statfs(path, &st) != nil {
		return ""
	}
	return strings.ToUpper(unix.ByteSliceToString(st.Fstypename[:]))
}

// command runs a program for what it prints, given a moment.
func command(name string, args ...string) string {
	if _, err := exec.LookPath(name); err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return ""
	}
	return string(out)
}
