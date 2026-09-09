package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// A fact is a line of the system block: what is reported and what was
// found. An anomaly the checks turned up is echoed after the value, lit.
type fact struct {
	label, value, anomaly string
}

// A check is a line of the start-up checks: what was checked, what was
// found, the word for how it stands, and whether that is a fault.
type check struct {
	label, value, status string
	fault                bool
}

const nominal = "NOMINAL"

// A report is everything the station says of itself as it comes up: the
// identification in the header, eight facts of the machine, and five
// checks of what conn runs on.
type report struct {
	version, note, station, term, clock string
	facts                               []fact  // host, system, cpu, user; memory, uptime, load, net
	checks                              []check // terminal is added by the screen, which knows its size
}

// stationReport reads the machine. Nothing here waits on the network;
// the commands it runs answer from disk and are given a moment each.
func stationReport() report {
	who := "someone"
	if u, err := user.Current(); err == nil && u.Username != "" {
		who = u.Username
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "somewhere"
	}
	host, _, _ = strings.Cut(host, ".")
	home, _ := os.UserHomeDir()
	m := readMachine()
	version, note := buildVersion()

	r := report{
		version: version,
		note:    note,
		station: who + "@" + host,
		term:    join(" · ", os.Getenv("TERM"), os.Getenv("COLORTERM")),
		clock:   zulu(time.Now()),
	}

	disk := diskCheck(home)
	load := fact{label: "LOAD"}
	if m.load == [3]float64{} {
		load.value = "UNREAD"
	} else {
		load.value = fmt.Sprintf("%.2f %.2f %.2f", m.load[0], m.load[1], m.load[2])
		if m.load[0] > float64(runtime.NumCPU()) {
			load.value, load.anomaly = "", load.value+" · HIGH"
		}
	}
	memory := fact{label: "MEMORY", value: gigabytes(m.memory, 1<<30)}
	if disk.fault {
		memory.anomaly = strings.TrimSuffix(disk.value, " FREE OF "+gigabytes(diskTotal(home), 1e9)) + " DISK FREE"
	}
	r.facts = []fact{
		{label: "HOST", value: host},
		{label: "SYSTEM", value: join(" · ", m.system, runtime.GOARCH)},
		{label: "CPU", value: join(" · ", m.processor, strconv.Itoa(runtime.NumCPU())+" CORES")},
		{label: "USER", value: join(" · ", who, os.Getenv("SHELL"))},
		memory,
		{label: "UPTIME", value: uptime(m.booted)},
		load,
		networkFact(),
	}
	r.checks = []check{
		commandCheck("TMUX", "tmux", "-V"),
		toolsCheck(),
		stateCheck(home),
		disk,
	}
	return r
}

// join is the parts that are not empty, with the separator between.
func join(sep string, parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, sep)
}

// buildVersion is the version this build reports and a note on it: 0.7.0
// and nothing for a release; the tag it is past and (devel) for a build
// off a commit; no version and (devel) for a build with no record of
// where it came from; whatever else the release stamp says, otherwise.
func buildVersion() (string, string) {
	v := version
	if v == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			v = info.Main.Version
		}
	}
	if v == "" {
		v = "unknown"
	}
	v = strings.TrimPrefix(v, "v")
	if base, rest, ok := strings.Cut(v, "-0.20"); ok && len(rest) > 12 {
		return base, "(devel)"
	}
	if v == "(devel)" {
		return "", "(devel)"
	}
	return v, ""
}

// version is stamped by the release build. A build that came another way
// answers from the module system instead, which go install fills with the
// tag and a plain go build leaves as (devel).
var version string

// zulu writes a time the way the old systems did, in UTC.
func zulu(t time.Time) string {
	return t.UTC().Format("02-Jan-2006  15:04:05") + " Z"
}

// commandVersion asks a program on the path for its version, or its path
// when it will not say; missing, it is the empty string.
func commandVersion(name, flag string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	if flag != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if out, err := exec.CommandContext(ctx, name, flag).CombinedOutput(); err == nil {
			if v := firstVersion(string(out)); v != "" {
				return v
			}
		}
	}
	return path
}

// commandCheck is a program conn needs: its version, or a fault.
func commandCheck(label, name, flag string) check {
	v := commandVersion(name, flag)
	if v == "" {
		return check{label: label, value: "NOT FOUND", status: "MISSING", fault: true}
	}
	return check{label: label, value: v, status: nominal}
}

// toolsCheck is the kin tools on one line: git, lsof on macOS, and
// docker, which conn can do without and which is left off when absent.
func toolsCheck() check {
	c := check{label: "GIT", status: nominal}
	var parts []string
	if v := commandVersion("git", "--version"); v != "" {
		parts = append(parts, v)
	} else {
		parts = append(parts, "MISSING")
		c.status, c.fault = "MISSING", true
	}
	if runtime.GOOS == "darwin" {
		if v := commandVersion("lsof", "-v"); v != "" {
			parts = append(parts, "LSOF "+v)
		} else {
			parts = append(parts, "LSOF MISSING")
			c.status, c.fault = "MISSING", true
		}
	}
	if v := commandVersion("docker", "--version"); v != "" {
		parts = append(parts, "DOCKER "+v)
	}
	c.value = strings.Join(parts, " · ")
	return c
}

// firstVersion picks the version out of what a program says of itself:
// the first word that starts with a digit.
func firstVersion(out string) string {
	for _, f := range strings.Fields(out) {
		if f[0] >= '0' && f[0] <= '9' {
			return strings.TrimRight(f, ",;")
		}
	}
	return ""
}

// stateCheck is where conn keeps its state: the directory, or the one it
// would be made in, must be writable.
func stateCheck(home string) check {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		dir = filepath.Join(home, ".local", "state")
	}
	dir = filepath.Join(dir, "conn")
	shown := dir
	if home != "" && strings.HasPrefix(dir, home) {
		shown = "~" + strings.TrimPrefix(dir, home)
	}
	probe := dir
	for {
		if info, err := os.Stat(probe); err == nil {
			if !info.IsDir() || syscall.Access(probe, 2) != nil {
				return check{label: "STATE", value: shown, status: "READ ONLY", fault: true}
			}
			return check{label: "STATE", value: shown, status: nominal}
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return check{label: "STATE", value: shown, status: "NO PATH", fault: true}
		}
		probe = parent
	}
}

// diskCheck is the room on the volume that holds home: low under a tenth.
func diskCheck(home string) check {
	free, total := diskRoom(home)
	if total == 0 {
		return check{label: "DISK", value: "UNREAD", status: "UNKNOWN"}
	}
	c := check{label: "DISK", value: gigabytes(free, 1e9) + " FREE OF " + gigabytes(total, 1e9), status: nominal}
	if free*10 < total {
		c.status, c.fault = "LOW", true
	}
	return c
}

// diskRoom is the free and total bytes of the volume holding a path.
func diskRoom(path string) (free, total uint64) {
	var st syscall.Statfs_t
	if path == "" || syscall.Statfs(path, &st) != nil {
		return 0, 0
	}
	return st.Bavail * uint64(st.Bsize), st.Blocks * uint64(st.Bsize)
}

func diskTotal(path string) uint64 {
	_, total := diskRoom(path)
	return total
}

// networkFact is the first interface that is up, not loopback, and has an
// address; none is an anomaly.
func networkFact() fact {
	ifaces, err := net.Interfaces()
	if err == nil {
		for _, ifc := range ifaces {
			if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
				continue
			}
			addrs, err := ifc.Addrs()
			if err != nil {
				continue
			}
			for _, a := range addrs {
				if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil {
					return fact{label: "NET", value: ifc.Name + " " + ipn.IP.String()}
				}
			}
		}
	}
	return fact{label: "NET", anomaly: "NO INTERFACE UP"}
}

// gigabytes writes a size in whole or tenth gigabytes of the given unit:
// a binary one for memory, as the machine is sold, a decimal one for disk,
// as the volume is.
func gigabytes(n uint64, unit float64) string {
	if n == 0 {
		return ""
	}
	g := float64(n) / unit
	if g >= 100 {
		return strconv.FormatFloat(g, 'f', 0, 64) + " GB"
	}
	return strings.TrimSuffix(strconv.FormatFloat(g, 'f', 1, 64), ".0") + " GB"
}

// uptime is how long the machine has been up, in days, hours and minutes.
func uptime(booted time.Time) string {
	if booted.IsZero() {
		return ""
	}
	d := time.Since(booted).Round(time.Minute)
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dD %02dH %02dM", days, hours, mins)
	}
	return fmt.Sprintf("%02dH %02dM", hours, mins)
}
