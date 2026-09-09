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

// A fact is a line of the readout: what is reported and what was found.
type fact struct {
	label, value string
}

// A check is a line of the start-up checks: what was checked, what was
// found, the word for how it stands, and whether that is a fault.
type check struct {
	label, value, status string
	fault                bool
}

const nominal = "NOMINAL"

// A report is everything the station says of itself as it comes up: the
// identification in the header; the system, which is the machine; the
// session, which is who is at it and how; and the checks, of what the
// machine itself can fail at. The screen adds its own check, since it
// knows its size.
type report struct {
	version, note, build, station, term, clock string
	system, session                            []fact
	checks                                     []check
}

// A machine is what the kernel says of the hardware and itself; the
// platform files fill it. A field left blank leaves its line off.
type machine struct {
	system, kernel, model, processor, cores string
	memory                                  uint64
	available                               int // percent of memory available; -1 unread
	swapUsed, swapTotal                     uint64
	swapNote                                string
	booted                                  time.Time
	load                                    [3]float64
	processes                               int
	power                                   string // what the machine is running on
	powerLow                                bool
	extra                                   []fact // what only this platform has to say
}

// stationReport reads the machine. Nothing here waits on the network;
// the commands it runs answer from disk and are given a moment each.
func stationReport() report {
	u, _ := user.Current()
	who := "someone"
	if u != nil && u.Username != "" {
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
	built, stamp := buildStamp()

	r := report{
		version: version,
		note:    note,
		build:   stamp,
		station: who + "@" + host,
		term:    join(" · ", os.Getenv("TERM"), os.Getenv("COLORTERM")),
		clock:   zulu(time.Now()),
	}
	r.system = systemFacts(host, home, m)
	r.session = sessionFacts(u, who, home)
	r.checks = []check{
		stateCheck(home),
		diskCheck(home),
		memoryCheck(m),
		loadCheck(m),
		networkCheck(),
		powerCheck(m),
		clockCheck(built),
	}
	return r
}

// systemFacts is the machine: what it is, what it has, and how it is
// doing.
func systemFacts(host, home string, m machine) []fact {
	memory := gigabytes(m.memory, 1<<30)
	if m.available >= 0 && memory != "" {
		memory += " · " + strconv.Itoa(m.available) + "% AVAILABLE"
	}
	swap := ""
	if m.swapTotal > 0 {
		swap = join(" · ", gigabytes(m.swapUsed, 1<<30)+" USED OF "+gigabytes(m.swapTotal, 1<<30), m.swapNote)
	} else if m.swapUsed == 0 && m.swapTotal == 0 && m.memory > 0 {
		swap = "NONE"
	}
	up := uptime(m.booted)
	if up != "" {
		up += " · UP SINCE " + m.booted.UTC().Format("02-Jan 15:04") + " Z"
	}
	processes := ""
	if m.processes > 0 {
		processes = strconv.Itoa(m.processes) + " RUNNING"
	}
	facts := []fact{
		{"HOST", host},
		{"SYSTEM", m.system},
		{"KERNEL", join(" · ", m.kernel, pageSize())},
		{"MODEL", m.model},
		{"CPU", join(" · ", m.processor, m.cores)},
		{"MEMORY", memory},
		{"SWAP", swap},
		{"VOLUME", join(" · ", volumeType(home), gigabytes(diskTotal(home), 1e9))},
		{"UPTIME", up},
		{"PROCESSES", processes},
	}
	facts = append(facts, m.extra...)
	return kept(facts)
}

// sessionFacts is who is at the station and how: the user, the shell,
// the terminal, where and when, and the conn that is running.
func sessionFacts(u *user.User, who, home string) []fact {
	userLine := who
	if u != nil {
		userLine = join(" · ", who, "UID "+u.Uid, adminNote(u))
	}
	shell := os.Getenv("SHELL")
	if shell != "" {
		shell = join(" ", strings.ToUpper(filepath.Base(shell)), commandVersion(shell, "--version"))
	}
	terminal := join(" ", os.Getenv("TERM_PROGRAM"), os.Getenv("TERM_PROGRAM_VERSION"))
	if os.Getenv("TMUX") != "" {
		terminal = join(" · ", terminal, "IN TMUX")
	}
	session := "LOCAL"
	if c := os.Getenv("SSH_CONNECTION"); c != "" {
		if f := strings.Fields(c); len(f) > 0 {
			session = "SSH FROM " + f[0]
		}
	}
	cwd, _ := os.Getwd()
	exe, _ := os.Executable()
	binary := ""
	if exe != "" {
		binary = tilde(exe, home)
		if info, err := os.Stat(exe); err == nil {
			binary = join(" · ", binary, sizeShort(uint64(info.Size())))
		}
	}
	return kept([]fact{
		{"USER", userLine},
		{"SHELL", shell},
		{"TTY", ttyName()},
		{"TERMINAL", terminal},
		{"SESSION", session},
		{"LOCALE", join(" · ", os.Getenv("LANG"), os.Getenv("LC_ALL"))},
		{"TIME ZONE", timeZone()},
		{"CWD", tilde(cwd, home)},
		{"PROCESS", fmt.Sprintf("PID %d · PARENT %d", os.Getpid(), os.Getppid())},
		{"ENV", fmt.Sprintf("%d VARIABLES · PATH %d ENTRIES", len(os.Environ()), len(filepath.SplitList(os.Getenv("PATH"))))},
		{"RUNTIME", join(" · ", runtime.Version(), runtime.GOOS+"/"+runtime.GOARCH, strconv.Itoa(runtime.GOMAXPROCS(0))+" THREADS")},
		{"BINARY", binary},
	})
}

// kept is the facts with something to say.
func kept(facts []fact) []fact {
	var out []fact
	for _, f := range facts {
		if strings.TrimSpace(f.value) != "" {
			out = append(out, f)
		}
	}
	return out
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

// tilde writes a path under home from ~.
func tilde(path, home string) string {
	if home != "" && (path == home || strings.HasPrefix(path, home+"/")) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

// adminNote says ADMIN when the user is in the group that administers
// the machine.
func adminNote(u *user.User) string {
	ids, err := u.GroupIds()
	if err != nil {
		return ""
	}
	for _, name := range []string{"admin", "sudo", "wheel"} {
		g, err := user.LookupGroup(name)
		if err != nil {
			continue
		}
		for _, id := range ids {
			if id == g.Gid {
				return "ADMIN"
			}
		}
	}
	return ""
}

// ttyName is the terminal device on stdin, without /dev/.
func ttyName() string {
	info, err := os.Stdin.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return ""
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}
	for _, dir := range []string{"/dev/pts", "/dev"} {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if dir == "/dev" && !strings.HasPrefix(e.Name(), "tty") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			fi, err := os.Stat(path)
			if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
				continue
			}
			if s, ok := fi.Sys().(*syscall.Stat_t); ok && s.Rdev == st.Rdev {
				return strings.TrimPrefix(path, "/dev/")
			}
		}
	}
	return ""
}

// timeZone is the zone the machine keeps, by name where the system links
// to one, its offset from UTC, and the local time.
func timeZone() string {
	name := os.Getenv("TZ")
	if name == "" {
		if target, err := os.Readlink("/etc/localtime"); err == nil {
			if _, after, ok := strings.Cut(target, "zoneinfo/"); ok {
				name = after
			}
		}
	}
	now := time.Now()
	abbr, off := now.Zone()
	sign := "+"
	if off < 0 {
		sign, off = "-", -off
	}
	utc := fmt.Sprintf("UTC%s%02d:%02d", sign, off/3600, off%3600/60)
	if name == "" {
		name = abbr
	}
	return join(" · ", name, utc, now.Format("15:04")+" LOCAL")
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

// buildStamp is when the build's commit was made, and the commit, the
// date and whether the tree was modified as a line, when the module
// system recorded them.
func buildStamp() (time.Time, string) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return time.Time{}, ""
	}
	var rev, when, dirty string
	var built time.Time
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
			if len(rev) > 7 {
				rev = rev[:7]
			}
		case "vcs.time":
			if t, err := time.Parse(time.RFC3339, s.Value); err == nil {
				built = t
				when = strings.ToUpper(t.UTC().Format("02-Jan-2006"))
			}
		case "vcs.modified":
			if s.Value == "true" {
				dirty = "MODIFIED"
			}
		}
	}
	return built, join(" · ", rev, when, dirty)
}

// zulu writes a time the way the old systems did, in UTC.
func zulu(t time.Time) string {
	return t.UTC().Format("02-Jan-2006  15:04:05") + " Z"
}

// commandVersion asks a program for its version, given a moment; missing
// or mute, it is the empty string.
func commandVersion(name, flag string) string {
	if _, err := exec.LookPath(name); err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, flag).CombinedOutput()
	if err != nil {
		return ""
	}
	return firstVersion(string(out))
}

// firstVersion picks the version out of what a program says of itself:
// the first word that starts with a digit, up to any parenthesis.
func firstVersion(out string) string {
	for _, f := range strings.Fields(out) {
		if f[0] >= '0' && f[0] <= '9' {
			v, _, _ := strings.Cut(f, "(")
			return strings.TrimRight(v, ",;")
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
	shown := tilde(dir, home)
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

// memoryCheck is the memory available to new work: low under a tenth.
func memoryCheck(m machine) check {
	if m.available < 0 || m.memory == 0 {
		return check{label: "MEMORY", value: "UNREAD", status: "UNKNOWN"}
	}
	c := check{label: "MEMORY", value: strconv.Itoa(m.available) + "% OF " + gigabytes(m.memory, 1<<30) + " AVAILABLE", status: nominal}
	if m.available < 10 {
		c.status, c.fault = "LOW", true
	}
	return c
}

// loadCheck is the load average against the cores: high when the last
// minute's exceeds them.
func loadCheck(m machine) check {
	if m.load == [3]float64{} {
		return check{label: "LOAD", value: "UNREAD", status: "UNKNOWN"}
	}
	c := check{label: "LOAD", value: fmt.Sprintf("%.2f %.2f %.2f · %d CORES", m.load[0], m.load[1], m.load[2], runtime.NumCPU()), status: nominal}
	if m.load[0] > float64(runtime.NumCPU()) {
		c.status, c.fault = "HIGH", true
	}
	return c
}

// networkCheck is the first interface that is up, not loopback, and has
// an address, with the count that are up; none is down.
func networkCheck() check {
	ifaces, err := net.Interfaces()
	if err != nil {
		return check{label: "NET", value: "UNREAD", status: "UNKNOWN"}
	}
	up, first := 0, ""
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
				up++
				if first == "" {
					first = ifc.Name + " " + ipn.IP.String()
				}
				break
			}
		}
	}
	if up == 0 {
		return check{label: "NET", value: "NO INTERFACE UP", status: "DOWN", fault: true}
	}
	return check{label: "NET", value: fmt.Sprintf("%s · %d UP", first, up), status: nominal}
}

// powerCheck is what the machine is running on: low on a battery under
// a tenth that is not charging.
func powerCheck(m machine) check {
	if m.power == "" {
		return check{label: "POWER", value: "UNREAD", status: "UNKNOWN"}
	}
	c := check{label: "POWER", value: m.power, status: nominal}
	if m.powerLow {
		c.status, c.fault = "LOW", true
	}
	return c
}

// clockCheck is the system clock against the one time conn knows for
// sure has passed, the build's commit: a clock behind it is wrong.
func clockCheck(built time.Time) check {
	now := time.Now()
	value := now.Format("15:04:05 MST")
	if built.IsZero() {
		return check{label: "CLOCK", value: value + " · NO BUILD TIME TO CHECK", status: "UNCHECKED"}
	}
	if now.Before(built) {
		return check{label: "CLOCK", value: value + " · BEFORE THE BUILD", status: "BEHIND", fault: true}
	}
	return check{label: "CLOCK", value: value + " · AFTER THE BUILD OF " + strings.ToUpper(built.UTC().Format("02-Jan-2006")), status: nominal}
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

// sizeShort writes a file's size in the unit that fits.
func sizeShort(n uint64) string {
	switch {
	case n >= 1<<30:
		return gigabytes(n, 1<<30)
	case n >= 1<<20:
		return strings.TrimSuffix(strconv.FormatFloat(float64(n)/(1<<20), 'f', 1, 64), ".0") + " MB"
	default:
		return strconv.FormatUint(n/1024, 10) + " KB"
	}
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

// pageSize is the kernel's page, in kilobytes.
func pageSize() string {
	if p := os.Getpagesize(); p > 0 {
		return strconv.Itoa(p/1024) + " KB PAGES"
	}
	return ""
}
