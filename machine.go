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

// An item is a line of the report: what is being reported, what was
// found, and — for a check — the word for how it stands. A fault is a
// check that did not come up nominal and matters.
type item struct {
	label, value, status string
	fault                bool
}

const nominal = "NOMINAL"

// A report is everything the station says of itself as it comes up: the
// identification in the header, the facts of the machine, and the checks.
type report struct {
	version, build, station, term, clock string
	facts, checks                        []item
}

// stationReport reads the machine. Nothing here waits on the network;
// the commands it runs answer from disk and are given a moment each.
func stationReport() report {
	who, uid := "someone", ""
	if u, err := user.Current(); err == nil && u.Username != "" {
		who, uid = u.Username, u.Uid
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "somewhere"
	}
	host, _, _ = strings.Cut(host, ".")
	home, _ := os.UserHomeDir()
	m := readMachine()

	r := report{
		version: buildVersion(),
		build:   buildStamp(),
		station: who + "@" + host,
		term:    strings.TrimSpace(os.Getenv("TERM") + "  " + os.Getenv("COLORTERM")),
		clock:   zulu(time.Now()),
	}
	fact := func(label, value string) {
		if value = strings.TrimSpace(value); value != "" {
			r.facts = append(r.facts, item{label: label, value: value})
		}
	}
	fact("HOST", host)
	fact("SYSTEM", m.system)
	fact("KERNEL", m.kernel+" "+runtime.GOARCH)
	fact("MODEL", m.model)
	fact("PROCESSOR", strings.TrimSpace(m.processor+fmt.Sprintf("  %d CORES", runtime.NumCPU())))
	fact("MEMORY", gigabytes(m.memory, 1<<30))
	fact("UPTIME", uptime(m.booted))
	fact("USER", strings.TrimSpace(who+" "+uid))
	fact("SHELL", os.Getenv("SHELL"))
	fact("HOME", home)
	fact("LOCALE", os.Getenv("LANG"))
	fact("TIME ZONE", timeZone())
	fact("PROCESS", fmt.Sprintf("PID %d  PARENT %d", os.Getpid(), os.Getppid()))
	fact("RUNTIME", runtime.Version())

	r.checks = append(r.checks, commandCheck("TMUX", "tmux", "-V", true))
	if runtime.GOOS == "darwin" {
		r.checks = append(r.checks, commandCheck("LSOF", "lsof", "-v", true))
	} else {
		r.checks = append(r.checks, procCheck())
	}
	r.checks = append(r.checks,
		commandCheck("GIT", "git", "--version", true),
		commandCheck("DOCKER", "docker", "", false),
		stateCheck(home),
		diskCheck(home),
		loadCheck(m.load),
		networkCheck(),
	)
	return r
}

// buildStamp is the commit and the date the build came from, when the
// module system recorded them.
func buildStamp() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	var rev, when, dirty string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if len(s.Value) > 7 {
				rev = s.Value[:7]
			} else {
				rev = s.Value
			}
		case "vcs.time":
			if t, err := time.Parse(time.RFC3339, s.Value); err == nil {
				when = t.UTC().Format("02-Jan-2006")
			}
		case "vcs.modified":
			if s.Value == "true" {
				dirty = "MODIFIED"
			}
		}
	}
	return strings.Join(strings.Fields(rev+" "+when+" "+dirty), "  ")
}

// commandCheck looks for a program on the path and, given a flag, asks it
// for its version. A program conn needs is a fault when missing; one it
// can do without is only absent.
func commandCheck(label, name, flag string, needed bool) item {
	path, err := exec.LookPath(name)
	if err != nil {
		if needed {
			return item{label: label, value: "NOT FOUND", status: "MISSING", fault: true}
		}
		return item{label: label, value: "NOT FOUND", status: "ABSENT"}
	}
	value := path
	if flag != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if out, err := exec.CommandContext(ctx, name, flag).CombinedOutput(); err == nil {
			if v := firstVersion(string(out)); v != "" {
				value = v
			}
		}
	}
	return item{label: label, value: value, status: nominal}
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

// procCheck is the Linux counterpart of lsof: the process table is read
// from /proc.
func procCheck() item {
	if _, err := os.Stat("/proc/self/status"); err != nil {
		return item{label: "PROC", value: "/proc NOT MOUNTED", status: "MISSING", fault: true}
	}
	return item{label: "PROC", value: "/proc", status: nominal}
}

// stateCheck is where conn keeps its state: the directory, or the one it
// would be made in, must be writable.
func stateCheck(home string) item {
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
				return item{label: "STATE", value: shown, status: "READ ONLY", fault: true}
			}
			return item{label: "STATE", value: shown, status: nominal}
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return item{label: "STATE", value: shown, status: "NO PATH", fault: true}
		}
		probe = parent
	}
}

// diskCheck is the room on the volume that holds home: low under a tenth.
func diskCheck(home string) item {
	var st syscall.Statfs_t
	if home == "" || syscall.Statfs(home, &st) != nil {
		return item{label: "DISK", value: "UNREAD", status: "UNKNOWN"}
	}
	free := st.Bavail * uint64(st.Bsize)
	total := st.Blocks * uint64(st.Bsize)
	it := item{label: "DISK", value: gigabytes(free, 1e9) + " FREE OF " + gigabytes(total, 1e9), status: nominal}
	if total > 0 && free*10 < total {
		it.status, it.fault = "LOW", true
	}
	return it
}

// loadCheck is the load average against the cores: high when the last
// minute's exceeds them.
func loadCheck(load [3]float64) item {
	if load == [3]float64{} {
		return item{label: "LOAD", value: "UNREAD", status: "UNKNOWN"}
	}
	it := item{label: "LOAD", value: fmt.Sprintf("%.2f  %.2f  %.2f", load[0], load[1], load[2]), status: nominal}
	if load[0] > float64(runtime.NumCPU()) {
		it.status, it.fault = "HIGH", true
	}
	return it
}

// networkCheck is the first interface that is up, not loopback, and has
// an address; none is down.
func networkCheck() item {
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
					return item{label: "NETWORK", value: ifc.Name + "  " + ipn.IP.String(), status: nominal}
				}
			}
		}
	}
	return item{label: "NETWORK", value: "NO INTERFACE UP", status: "DOWN", fault: true}
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

// timeZone is the zone the machine keeps, by name where the system links
// to one, and its offset from UTC.
func timeZone() string {
	name := os.Getenv("TZ")
	if name == "" {
		if target, err := os.Readlink("/etc/localtime"); err == nil {
			if _, after, ok := strings.Cut(target, "zoneinfo/"); ok {
				name = after
			}
		}
	}
	abbr, off := time.Now().Zone()
	sign := "+"
	if off < 0 {
		sign, off = "-", -off
	}
	utc := fmt.Sprintf("UTC%s%02d:%02d", sign, off/3600, off%3600/60)
	if name == "" {
		name = abbr
	}
	return strings.TrimSpace(name + "  " + utc)
}

// version is stamped by the release build. A build that came another way
// answers from the module system instead, which go install fills with the
// tag and a plain go build leaves as (devel).
var version string

// buildVersion is the version this build reports, bare: 0.7.0 for a
// release; (devel), or a tag with commits and a dirty mark after it, for a
// build that is not one.
func buildVersion() string {
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
	// A pseudo-version — a tag, a timestamp and a commit — is a build off
	// a commit past the tag; the commit is on the build line.
	if base, rest, ok := strings.Cut(v, "-0.20"); ok && len(rest) > 12 {
		v = base + " DEVEL"
	}
	return v
}

// zulu writes a time the way the old systems did, in UTC.
func zulu(t time.Time) string {
	return t.UTC().Format("02-Jan-2006  15:04:05") + " Z"
}
