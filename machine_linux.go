package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// readMachine reads uname, sysinfo, and the files under /etc, /proc and
// /sys; what cannot be read leaves its field blank and the line off the
// report.
func readMachine() machine {
	m := machine{available: -1}
	var u syscall.Utsname
	if err := syscall.Uname(&u); err == nil {
		m.kernel = "Linux " + cstring(u.Release[:])
	}
	m.system = join(" ", osRelease(), virtualization())
	vendor, _ := os.ReadFile("/sys/devices/virtual/dmi/id/sys_vendor")
	product, _ := os.ReadFile("/sys/devices/virtual/dmi/id/product_name")
	m.model = join(" ", strings.TrimSpace(string(vendor)), strings.TrimSpace(string(product)))
	m.processor = cpuModel()
	m.cores = strconv.Itoa(len(procList("/proc/cpuinfo", "processor"))) + " CORES"
	var si syscall.Sysinfo_t
	if err := syscall.Sysinfo(&si); err == nil {
		unit := uint64(si.Unit)
		m.memory = si.Totalram * unit
		m.swapTotal = si.Totalswap * unit
		m.swapUsed = (si.Totalswap - si.Freeswap) * unit
		m.booted = time.Now().Add(-time.Duration(si.Uptime) * time.Second)
		for i := range m.load {
			m.load[i] = float64(si.Loads[i]) / 65536
		}
		m.processes = int(si.Procs)
	}
	if total, avail := meminfo("MemTotal"), meminfo("MemAvailable"); total > 0 && avail > 0 {
		m.available = int(avail * 100 / total)
	}
	m.power, m.powerLow = batteryStatus()
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

// virtualization says when the system is a container or under WSL.
func virtualization() string {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return "(CONTAINER)"
	}
	if b, err := os.ReadFile("/proc/version"); err == nil && strings.Contains(strings.ToLower(string(b)), "microsoft") {
		return "(WSL)"
	}
	return ""
}

// cpuModel is the processor's name from /proc/cpuinfo, under whichever
// key the architecture files it.
func cpuModel() string {
	for _, key := range []string{"model name", "Model", "Hardware"} {
		if v := procList("/proc/cpuinfo", key); len(v) > 0 {
			return v[0]
		}
	}
	return ""
}

// procList is every value under a key in a /proc file of key : value
// lines.
func procList(path, key string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(l, ":")
		if ok && strings.TrimSpace(k) == key {
			out = append(out, strings.TrimSpace(v))
		}
	}
	return out
}

// meminfo is a figure from /proc/meminfo, in kilobytes.
func meminfo(key string) uint64 {
	v := procList("/proc/meminfo", key)
	if len(v) == 0 {
		return 0
	}
	n, _ := strconv.ParseUint(strings.Fields(v[0])[0], 10, 64)
	return n
}

// batteryStatus reads /sys/class/power_supply: the battery's charge and
// state when there is one, the mains otherwise.
func batteryStatus() (string, bool) {
	entries, err := os.ReadDir("/sys/class/power_supply")
	if err != nil {
		return "", false
	}
	read := func(name, file string) string {
		b, _ := os.ReadFile(filepath.Join("/sys/class/power_supply", name, file))
		return strings.TrimSpace(string(b))
	}
	mains := ""
	for _, e := range entries {
		switch read(e.Name(), "type") {
		case "Battery":
			pct := read(e.Name(), "capacity")
			state := strings.ToUpper(read(e.Name(), "status"))
			n, _ := strconv.Atoi(pct)
			source := "BATTERY"
			if state == "CHARGING" || state == "FULL" {
				source = "AC POWER"
			}
			return join(" · ", source, pct+"%", state), n < 10 && state == "DISCHARGING"
		case "Mains":
			if read(e.Name(), "online") == "1" {
				mains = "AC POWER"
			}
		}
	}
	return mains, false
}

// volumeType is the file system holding a path, by the magic the kernel
// reports for it.
func volumeType(path string) string {
	var st unix.Statfs_t
	if path == "" || unix.Statfs(path, &st) != nil {
		return ""
	}
	switch uint64(st.Type) {
	case 0xEF53:
		return "EXT4"
	case 0x9123683E:
		return "BTRFS"
	case 0x58465342:
		return "XFS"
	case 0x2FC12FC1:
		return "ZFS"
	case 0x01021994:
		return "TMPFS"
	case 0x794C7630:
		return "OVERLAYFS"
	case 0xF15F:
		return "ECRYPTFS"
	default:
		return fmt.Sprintf("FS 0x%X", st.Type)
	}
}
