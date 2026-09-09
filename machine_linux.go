package main

import (
	"os"
	"strings"
	"syscall"
	"time"
)

// machine is what the kernel says of the hardware and itself.
type machine struct {
	system, kernel, model, processor string
	memory                           uint64
	booted                           time.Time
	load                             [3]float64
}

// readMachine reads uname, sysinfo, and the files under /etc, /proc and
// /sys; what cannot be read leaves its field blank and the line off the
// report.
func readMachine() machine {
	var m machine
	var u syscall.Utsname
	if err := syscall.Uname(&u); err == nil {
		m.kernel = strings.ToUpper(cstring(u.Sysname[:])) + " " + cstring(u.Release[:])
	}
	m.system = osRelease()
	if b, err := os.ReadFile("/sys/devices/virtual/dmi/id/product_name"); err == nil {
		m.model = strings.TrimSpace(string(b))
	}
	m.processor = cpuModel()
	var si syscall.Sysinfo_t
	if err := syscall.Sysinfo(&si); err == nil {
		m.memory = si.Totalram * uint64(si.Unit)
		m.booted = time.Now().Add(-time.Duration(si.Uptime) * time.Second)
		for i := range m.load {
			m.load[i] = float64(si.Loads[i]) / 65536
		}
	}
	return m
}

// cstring is the string in a NUL-terminated array.
func cstring(a []int8) string {
	var b strings.Builder
	for _, c := range a {
		if c == 0 {
			break
		}
		b.WriteByte(byte(c))
	}
	return b.String()
}

// osRelease is the distribution's name for itself.
func osRelease() string {
	b, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	for _, l := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(l, "PRETTY_NAME="); ok {
			return strings.Trim(v, `"`)
		}
	}
	return ""
}

// cpuModel is the processor's name from /proc/cpuinfo, under whichever
// key the architecture files it.
func cpuModel() string {
	b, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	for _, l := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(l, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "model name", "Model", "Hardware":
			return strings.TrimSpace(v)
		}
	}
	return ""
}
