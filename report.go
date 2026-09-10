package main

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// A station is everything conn reads as it comes up, before a word is
// put to any of it: the machine, the session, the build, the volume
// under home, the network, and the state directory. readStation reads
// it; compose words it. Keeping the two apart is what lets the words be
// tested against a station on file, and re-said as the clock turns.
type station struct {
	machine machine
	session session
	build   build
	volume  volume
	network network
	netRead bool
	state   stateDir
	tools   []tool // what the platform needs past the kernel
}

// readStation reads the station. Nothing here waits on the network; the
// programs it runs answer from disk and are given a moment each.
func readStation() station {
	st := station{build: readBuild(), session: readSession()}
	st.machine = readMachine()
	st.volume = readVolume(st.session.home)
	st.network, st.netRead = readNetwork()
	st.state = readStateDir(st.session.home)
	st.tools = readTools()
	return st
}

// A fact is a line of the readout: what is reported and what was found.
// A path is shown as it is, where every other value is set in capitals.
type fact struct {
	label, value string
	path         bool
}

// A check is a line of the start-up checks: what was checked, what was
// found, the word for how it stands, and whether that is a fault.
type check struct {
	label, value, status string
	fault                bool
	path                 bool
}

// The words a check stands under. UNKNOWN is a check the machine would
// not answer; UNCHECKED one that has nothing to check against. Neither
// is a fault.
const (
	nominal   = "NOMINAL"
	unknown   = "UNKNOWN"
	unchecked = "UNCHECKED"
)

// A report is the station worded for the console: the identification
// in the header; the system, which is the machine; the session, which
// is who is at it and how; and the checks, of what the machine itself
// can fail at. The screen adds its own check, since it knows its size.
type report struct {
	version, note, build, station, term, clock string
	system, session                            []fact
	checks                                     []check
	// The verdict's chip is an annunciator: lit on one second, dark on
	// the next, while the console is up. Everywhere else — a pipe, a
	// test, a reading that is not being watched turn by turn — it is
	// lit, since there is no second to be dark on.
	lit bool
}

// The thresholds the checks hold the machine to.
const (
	memoryLowPercent = 10                      // memory available, under which it is LOW
	powerLowPercent  = 10                      // a discharging battery, under which it is LOW
	diskLowShare     = 10                      // disk free under this fraction of the volume is LOW...
	diskLowFloor     = 5 * 1000 * 1000 * 1000  // ...but under this many bytes always is...
	diskLowCeiling   = 50 * 1000 * 1000 * 1000 // ...and with this many free never is
)

// compose words the station as of a moment.
func compose(st station, now time.Time) report {
	who := st.session.user
	if who == "" {
		who = "someone"
	}
	host := st.session.host
	if host == "" {
		host = "somewhere"
	}
	note := ""
	if !st.build.exact {
		note = "(devel)"
	}
	r := report{
		version: st.build.tag,
		note:    note,
		build:   buildLine(st.build),
		station: who + "@" + host,
		term:    st.session.term,
		clock:   zulu(now),
		lit:     true,
	}
	r.system = systemFacts(st, now)
	r.session = sessionFacts(st.session, now)
	r.checks = []check{
		stateCheck(st.state, st.session.home),
		diskCheck(st.volume),
		memoryCheck(st.machine),
		loadCheck(st.machine),
		networkCheck(st.network, st.netRead),
		powerCheck(st.machine.power),
		clockCheck(st.build, now),
	}
	for _, t := range st.tools {
		r.checks = append(r.checks, toolCheck(t))
	}
	return r
}

// toolCheck is a program the platform needs: where it is, or MISSING.
func toolCheck(t tool) check {
	if t.path == "" {
		return check{label: t.name, value: "NOT ON PATH", status: "MISSING", fault: true}
	}
	return check{label: t.name, value: t.path, status: nominal, path: true}
}

// buildLine is the commit, its date, and MODIFIED when the tree had
// changes past it.
func buildLine(b build) string {
	when := ""
	if !b.time.IsZero() {
		when = strings.ToUpper(b.time.UTC().Format("02-Jan-2006"))
	}
	modified := ""
	if b.modified {
		modified = "MODIFIED"
	}
	return join(" · ", b.commit, when, modified)
}

// systemFacts is the machine: what it is, what it has, and how it is
// doing.
func systemFacts(st station, now time.Time) []fact {
	m := st.machine
	system := m.system
	if m.systemBuild != "" {
		system += " (" + m.systemBuild + ")"
	}
	if m.virtual != "" {
		system = join(" ", system, "("+m.virtual+")")
	}
	cores := ""
	if m.cpus > 0 {
		cores = strconv.Itoa(m.cpus) + " CORES"
		if m.perfCores > 0 && m.effCores > 0 {
			cores += fmt.Sprintf(" (%dP + %dE)", m.perfCores, m.effCores)
		}
	}
	if m.rosetta {
		cores = join(" · ", cores, "UNDER ROSETTA")
	}
	memory := gigabytes(m.memory, 1<<30)
	if memory != "" && m.available >= 0 {
		memory += " · " + strconv.Itoa(m.available) + "% AVAILABLE"
	}
	swap := ""
	if m.swapTotal > 0 {
		swap = gigabytes(m.swapUsed, 1<<30) + " USED OF " + gigabytes(m.swapTotal, 1<<30)
		if m.swapEncrypt {
			swap += " · ENCRYPTED"
		}
	} else if m.memory > 0 {
		swap = "NONE"
	}
	up := uptime(m.booted, now)
	if up != "" {
		up += " · UP SINCE " + m.booted.UTC().Format("02-Jan 15:04") + " Z"
	}
	processes := ""
	if m.processes > 0 {
		processes = strconv.Itoa(m.processes) + " RUNNING"
	}
	sip := ""
	if m.sip != "" {
		sip = strings.ToUpper(m.sip)
	}
	return kept([]fact{
		{label: "HOST", value: st.session.host},
		{label: "SYSTEM", value: system},
		{label: "KERNEL", value: join(" · ", m.kernel, pageSize(m.page))},
		{label: "MODEL", value: m.model},
		{label: "CPU", value: join(" · ", m.processor, cores)},
		{label: "MEMORY", value: memory},
		{label: "SWAP", value: swap},
		{label: "VOLUME", value: join(" · ", st.volume.fs, gigabytes(st.volume.total, 1e9))},
		{label: "UPTIME", value: up},
		{label: "PROCESSES", value: processes},
		{label: "SIP", value: sip},
	})
}

// sessionFacts is who is at the station and how: the user, the shell,
// the terminal, where and when, and the conn that is running.
func sessionFacts(s session, now time.Time) []fact {
	userLine := s.user
	if s.uid != "" {
		userLine = join(" · ", s.user, "UID "+s.uid)
	}
	if s.admin {
		userLine = join(" · ", userLine, "ADMIN")
	}
	shell := ""
	if s.shell != "" {
		shell = join(" ", filepath.Base(s.shell), s.shellVer)
	}
	terminal := join(" ", s.terminal, s.terminalVer)
	if s.tmux {
		terminal = join(" · ", terminal, "IN TMUX")
	}
	sessionLine := "LOCAL"
	if s.sshFrom != "" {
		sessionLine = "SSH FROM " + s.sshFrom
	}
	binary := ""
	if s.exe != "" {
		binary = join(" · ", tilde(s.exe, s.home), sizeShort(uint64(s.exeSize)))
	}
	process := ""
	if s.pid > 0 {
		process = fmt.Sprintf("PID %d · PARENT %d", s.pid, s.ppid)
	}
	threads := ""
	if s.threads > 0 {
		threads = strconv.Itoa(s.threads) + " THREADS"
	}
	env := ""
	if s.envCount > 0 {
		env = fmt.Sprintf("%d VARIABLES · PATH %d ENTRIES", s.envCount, s.pathCount)
	}
	return kept([]fact{
		{label: "USER", value: userLine},
		{label: "SHELL", value: shell},
		{label: "TTY", value: s.tty},
		{label: "TERMINAL", value: terminal},
		{label: "SESSION", value: sessionLine},
		{label: "LOCALE", value: s.lang},
		{label: "TIME ZONE", value: timeZone(s.zone, now)},
		{label: "CWD", value: tilde(s.cwd, s.home), path: true},
		{label: "PROCESS", value: process},
		{label: "ENV", value: env},
		{label: "RUNTIME", value: join(" · ", s.goVersion, s.platform, threads)},
		{label: "BINARY", value: binary, path: true},
	})
}

// timeZone is the zone by name, its offset from UTC, and the local time.
func timeZone(name string, now time.Time) string {
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

// stateCheck is where conn keeps its state: the directory, or the one it
// would be made in, must be writable.
func stateCheck(s stateDir, home string) check {
	c := check{label: "STATE", value: tilde(s.path, home), path: true, status: nominal}
	switch s.problem {
	case "":
	case stateNotDir:
		c.status, c.fault = "NOT A DIR", true
	case stateReadOnly:
		c.status, c.fault = "READ ONLY", true
	default:
		c.status, c.fault = "NO PATH", true
	}
	return c
}

// diskCheck is the room on the volume under home. It is LOW under a
// tenth of the volume, with the tenth held between 5 and 50 GB: a small
// disk is not low at a few hundred megabytes short of a tenth, and a
// vast one is not low with fifty gigabytes free.
func diskCheck(v volume) check {
	if v.total == 0 {
		return check{label: "DISK", value: "UNREAD", status: unknown}
	}
	c := check{label: "DISK", value: gigabytes(v.free, 1e9) + " FREE OF " + gigabytes(v.total, 1e9), status: nominal}
	if v.free < min(max(v.total/diskLowShare, diskLowFloor), diskLowCeiling) {
		c.status, c.fault = "LOW", true
	}
	return c
}

// memoryCheck is the memory available to new work: LOW under a tenth.
func memoryCheck(m machine) check {
	if m.available < 0 || m.memory == 0 {
		return check{label: "MEMORY", value: "UNREAD", status: unknown}
	}
	c := check{label: "MEMORY", value: strconv.Itoa(m.available) + "% OF " + gigabytes(m.memory, 1<<30) + " AVAILABLE", status: nominal}
	if m.available < memoryLowPercent {
		c.status, c.fault = "LOW", true
	}
	return c
}

// loadCheck is the load average against the processors: HIGH when the
// last minute's exceeds them.
func loadCheck(m machine) check {
	if m.load == [3]float64{} {
		return check{label: "LOAD", value: "UNREAD", status: unknown}
	}
	load := fmt.Sprintf("%.2f %.2f %.2f", m.load[0], m.load[1], m.load[2])
	if m.cpus == 0 {
		return check{label: "LOAD", value: load + " · NO CORE COUNT TO CHECK AGAINST", status: unchecked}
	}
	c := check{label: "LOAD", value: fmt.Sprintf("%s · %d CORES", load, m.cpus), status: nominal}
	if m.load[0] > float64(m.cpus) {
		c.status, c.fault = "HIGH", true
	}
	return c
}

// networkCheck is the interfaces that reach past the machine: DOWN when
// none does.
func networkCheck(n network, read bool) check {
	if !read {
		return check{label: "NET", value: "UNREAD", status: unknown}
	}
	if n.up == 0 {
		return check{label: "NET", value: "NO INTERFACE UP", status: "DOWN", fault: true}
	}
	return check{label: "NET", value: fmt.Sprintf("%s · %d UP", n.first, n.up), status: nominal}
}

// powerCheck is what the machine runs on: LOW on a battery under a tenth
// that is discharging.
func powerCheck(p power) check {
	if p.source == "" {
		return check{label: "POWER", value: "UNREAD", status: unknown}
	}
	source := "AC POWER"
	if p.source == "battery" {
		source = "BATTERY"
	}
	value := source
	if p.percent >= 0 {
		value = join(" · ", source, strconv.Itoa(p.percent)+"%", p.state)
		switch {
		case p.remaining != "" && p.state == "discharging":
			value += " · " + p.remaining + " LEFT"
		case p.remaining != "" && p.state == "charging":
			value += " · " + p.remaining + " TO FULL"
		}
	}
	c := check{label: "POWER", value: value, status: nominal}
	if p.state == "discharging" && p.percent >= 0 && p.percent < powerLowPercent {
		c.status, c.fault = "LOW", true
	}
	return c
}

// clockCheck is the system clock against the one time conn knows for
// sure has passed, the build's commit: a clock behind it is wrong.
func clockCheck(b build, now time.Time) check {
	if b.time.IsZero() {
		return check{label: "CLOCK", value: "NO BUILD TIME TO CHECK AGAINST", status: unchecked}
	}
	day := strings.ToUpper(b.time.UTC().Format("02-Jan-2006"))
	if now.Before(b.time) {
		return check{label: "CLOCK", value: "BEFORE THE BUILD OF " + day, status: "BEHIND", fault: true}
	}
	return check{label: "CLOCK", value: "AFTER THE BUILD OF " + day, status: nominal}
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

// zulu writes a time the way the old systems did, in UTC.
func zulu(t time.Time) string {
	return t.UTC().Format("02-Jan-2006  15:04:05") + " Z"
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
	case n > 0:
		return strconv.FormatUint(max(n/1024, 1), 10) + " KB"
	default:
		return ""
	}
}

// uptime is how long the machine has been up, in days, hours and minutes.
func uptime(booted, now time.Time) string {
	if booted.IsZero() {
		return ""
	}
	d := now.Sub(booted).Round(time.Minute)
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dD %02dH %02dM", days, hours, mins)
	}
	return fmt.Sprintf("%02dH %02dM", hours, mins)
}

// pageSize is the kernel's page, in kilobytes.
func pageSize(bytes int) string {
	if bytes > 0 {
		return strconv.Itoa(bytes/1024) + " KB PAGES"
	}
	return ""
}
