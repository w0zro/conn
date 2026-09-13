package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// The parsers: pure functions from what a platform hands over — the
// text a program prints, the bytes a sysctl returns, the files under
// /proc and /sys — to the machine's fields. Nothing here touches the
// machine, so every one of them is tested on every platform, against
// captures.

// parsePmset reads what pmset -g batt prints: what the machine draws
// from and, with a battery, its charge, state and time left.
func parsePmset(out string) power {
	p := power{percent: -1}
	if strings.TrimSpace(out) == "" {
		return p
	}
	p.source = "ac"
	if strings.Contains(out, "'Battery Power'") {
		p.source = "battery"
	}
	for _, l := range strings.Split(out, "\n") {
		if !strings.Contains(l, "InternalBattery") {
			continue
		}
		_, rest, ok := strings.Cut(l, "\t")
		if !ok {
			continue
		}
		parts := strings.Split(rest, ";")
		if n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(parts[0]), "%")); err == nil {
			p.percent = n
		}
		if len(parts) > 1 {
			p.state = strings.TrimSpace(parts[1])
		}
		if len(parts) > 2 {
			if r, _, ok := strings.Cut(strings.TrimSpace(parts[2]), " remaining"); ok && !strings.HasPrefix(r, "(") && r != "0:00" {
				p.remaining = r
			}
		}
		break
	}
	return p
}

// parseSwapUsage reads the xsw_usage struct behind vm.swapusage: three
// 64-bit sizes — total, available, used — the page size, and whether
// the swap is encrypted.
func parseSwapUsage(raw []byte) (total, used uint64, encrypted, ok bool) {
	if len(raw) < 32 {
		return 0, 0, false, false
	}
	return binary.LittleEndian.Uint64(raw[0:8]),
		binary.LittleEndian.Uint64(raw[16:24]),
		binary.LittleEndian.Uint32(raw[28:32]) != 0,
		true
}

// parseLoadavg reads the loadavg struct behind vm.loadavg: three
// fixed-point words, padding, and the scale they are in.
func parseLoadavg(raw []byte) (load [3]float64, ok bool) {
	if len(raw) < 24 {
		return load, false
	}
	scale := float64(binary.LittleEndian.Uint64(raw[16:24]))
	if scale <= 0 {
		return load, false
	}
	for i := range load {
		load[i] = float64(binary.LittleEndian.Uint32(raw[i*4:i*4+4])) / scale
	}
	return load, true
}

// parseOSRelease is the distribution's name for itself, from
// /etc/os-release.
func parseOSRelease(text string) string {
	for _, l := range strings.Split(text, "\n") {
		if v, ok := strings.CutPrefix(l, "PRETTY_NAME="); ok {
			return strings.Trim(strings.TrimSpace(v), `"`)
		}
	}
	return ""
}

// parseCPUInfo is the processor's name and the count of logical
// processors, from /proc/cpuinfo, under whichever keys the architecture
// files them.
func parseCPUInfo(text string) (model string, cpus int) {
	for _, key := range []string{"model name", "Model", "Hardware"} {
		if v := procValues(text, key); len(v) > 0 {
			model = v[0]
			break
		}
	}
	return model, len(procValues(text, "processor"))
}

// parseMeminfo is the total memory and the memory available for new
// work, in bytes, from /proc/meminfo.
func parseMeminfo(text string) (total, available uint64) {
	kb := func(key string) uint64 {
		v := procValues(text, key)
		if len(v) == 0 {
			return 0
		}
		n, _ := strconv.ParseUint(strings.Fields(v[0])[0], 10, 64)
		return n
	}
	return kb("MemTotal") * 1024, kb("MemAvailable") * 1024
}

// procValues is every value under a key in a file of key : value lines.
func procValues(text, key string) []string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		k, v, ok := strings.Cut(l, ":")
		if ok && strings.TrimSpace(k) == key {
			out = append(out, strings.TrimSpace(v))
		}
	}
	return out
}

// readPowerSupply reads a /sys/class/power_supply directory: the first
// battery's charge and state, or the mains when there is no battery.
func readPowerSupply(dir string) power {
	p := power{percent: -1}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return p
	}
	read := func(name, file string) string {
		b, _ := os.ReadFile(filepath.Join(dir, name, file))
		return strings.TrimSpace(string(b))
	}
	for _, e := range entries {
		switch read(e.Name(), "type") {
		case "Battery":
			p.percent, _ = strconv.Atoi(read(e.Name(), "capacity"))
			p.state = strings.ToLower(read(e.Name(), "status"))
			p.source = "battery"
			if p.state == "charging" || p.state == "full" || p.state == "not charging" {
				p.source = "ac"
			}
			return p
		case "Mains":
			if read(e.Name(), "online") == "1" {
				p.source = "ac"
			}
		}
	}
	return p
}

// readLinuxFiles fills what Linux keeps on file, under a root that is /
// on the machine and a fixture in the tests: the distribution and
// whether it runs in a container or under WSL, the hardware, the
// processor, the memory available, and the power supply.
func readLinuxFiles(root string, m *machine) {
	file := func(path string) string {
		b, _ := os.ReadFile(filepath.Join(root, path))
		return string(b)
	}
	m.system = parseOSRelease(file("etc/os-release"))
	if _, err := os.Stat(filepath.Join(root, ".dockerenv")); err == nil {
		m.virtual = "container"
	} else if strings.Contains(strings.ToLower(file("proc/version")), "microsoft") {
		m.virtual = "wsl"
	}
	m.model = join(" ",
		strings.TrimSpace(file("sys/devices/virtual/dmi/id/sys_vendor")),
		strings.TrimSpace(file("sys/devices/virtual/dmi/id/product_name")))
	m.processor, m.cpus = parseCPUInfo(file("proc/cpuinfo"))
	if total, avail := parseMeminfo(file("proc/meminfo")); total > 0 {
		m.available = int(avail * 100 / total)
	}
	m.power = readPowerSupply(filepath.Join(root, "sys/class/power_supply"))
}
