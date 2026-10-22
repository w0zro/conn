package console

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/w0zro/conn/internal/station"
)

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
	// A program conn needs that is not on the path.
	//
	// Every status is held to statusW, which is the column the words
	// are right-aligned in; see screen.go.
	missing = "MISSING"
	// A file naming a theme conn does not have: conn comes up in its
	// own, and the file was written meaning otherwise.
	noTheme = "NO THEME"
	// A file that is there and that conn could not use: it would not
	// parse, or it would not open. Which of the two is in the error
	// itself, said where there is room for a sentence.
	notRead = "NOT READ"
)

// A Report is the station worded for the console: the identification
// in the header; the system, which is the machine; the session, which
// is who is at it and how; and the checks, of what the machine itself
// can fail at. The screen adds its own check, since it knows its size.
type Report struct {
	version, note, build, station, term, Clock string
	system, login                              []Fact
	checks                                     []check
	// The verdict's chip is an annunciator: Lit on one second, dark on
	// the next, while the console is up. Everywhere else — a pipe, a
	// test, a reading that is not being watched turn by turn — it is
	// Lit, since there is no second to be dark on.
	Lit bool
}

// The thresholds the checks hold the machine to.
const (
	memoryLowPercent = 10                      // memory available, under which it is LOW
	powerLowPercent  = 10                      // a discharging battery, under which it is LOW
	diskLowShare     = 10                      // disk free under this fraction of the volume is LOW...
	diskLowFloor     = 5 * 1000 * 1000 * 1000  // ...but under this many bytes always is...
	diskLowCeiling   = 50 * 1000 * 1000 * 1000 // ...and with this many free never is
)

// Compose words the station as of a moment.
func Compose(st station.Station, cfg Config, now time.Time) Report {
	who := st.Login.User
	if who == "" {
		who = "someone"
	}
	host := st.Login.Host
	if host == "" {
		host = "somewhere"
	}
	note := ""
	if !st.Build.Exact {
		note = "(devel)"
	}
	r := Report{
		version: st.Build.Tag,
		note:    note,
		build:   BuildLine(st.Build),
		station: who + "@" + host,
		term:    st.Login.Term,
		Clock:   zulu(now),
		Lit:     true,
	}
	r.system = systemFacts(st, now)
	r.login = sessionFacts(st.Login, now)
	r.checks = []check{
		stateCheck(st.State, st.Login.Home),
		configCheck(cfg, st.Login.Home),
		diskCheck(st.Volume),
		memoryCheck(st.Machine),
		loadCheck(st.Machine),
		networkCheck(st.Network, st.NetRead),
		powerCheck(st.Machine.Power),
		clockCheck(st.Build, now),
	}
	for _, t := range st.Tools {
		r.checks = append(r.checks, toolCheck(t))
	}
	return r
}

// toolCheck is a program the platform needs: where it is, or MISSING.
func toolCheck(t station.Tool) check {
	if t.Path == "" {
		return check{label: t.Name, value: "NOT ON PATH", status: missing, fault: true}
	}
	return check{label: t.Name, value: t.Path, status: nominal, path: true}
}

// BuildLine is the commit, its date, and MODIFIED when the tree had
// changes past it.
func BuildLine(b station.Build) string {
	when := ""
	if !b.Time.IsZero() {
		when = strings.ToUpper(b.Time.UTC().Format("02-Jan-2006"))
	}
	modified := ""
	if b.Modified {
		modified = "MODIFIED"
	}
	return Join(" · ", b.Commit, when, modified)
}

// systemFacts is the machine: what it is, what it has, and how it is
// doing.
func systemFacts(st station.Station, now time.Time) []Fact {
	m := st.Machine
	system := m.System
	if m.SystemBuild != "" {
		system += " (" + m.SystemBuild + ")"
	}
	if m.Virtual != "" {
		system = Join(" ", system, "("+m.Virtual+")")
	}
	cores := ""
	if m.CPUs > 0 {
		cores = strconv.Itoa(m.CPUs) + " CORES"
		if m.PerfCores > 0 && m.EffCores > 0 {
			cores += fmt.Sprintf(" (%dP + %dE)", m.PerfCores, m.EffCores)
		}
	}
	if m.Rosetta {
		cores = Join(" · ", cores, "UNDER ROSETTA")
	}
	memory := gigabytes(m.Memory, 1<<30)
	if memory != "" && m.Available >= 0 {
		memory += " · " + strconv.Itoa(m.Available) + "% AVAILABLE"
	}
	swap := ""
	if m.SwapTotal > 0 {
		swap = gigabytes(m.SwapUsed, 1<<30) + " USED OF " + gigabytes(m.SwapTotal, 1<<30)
		if m.SwapEncrypt {
			swap += " · ENCRYPTED"
		}
	} else if m.Memory > 0 {
		swap = "NONE"
	}
	up := Uptime(m.Booted, now)
	if up != "" {
		up += " · UP SINCE " + m.Booted.UTC().Format("02-Jan 15:04") + " Z"
	}
	// How many processes the machine is holding. It said RUNNING of
	// every one of them, and almost none of them are: the kernel
	// answers that every process in the table is runnable, and ps,
	// which does tell them apart, found five of eight hundred actually
	// running. The count is of what is there, which is what was ever
	// read, and the word it cannot earn is not said.
	processes := ""
	if m.Processes > 0 {
		processes = strconv.Itoa(m.Processes)
	}
	sip := ""
	if m.SIP != "" {
		sip = strings.ToUpper(m.SIP)
	}
	// The host is not here. It is on the header, in the station's own
	// name, and a column that said it again would be the second place
	// to read one fact.
	return kept([]Fact{
		{Label: "SYSTEM", Value: system},
		{Label: "KERNEL", Value: Join(" · ", m.Kernel, pageSize(m.Page))},
		{Label: "MODEL", Value: m.Model},
		{Label: "CPU", Value: Join(" · ", m.Processor, cores)},
		{Label: "MEMORY", Value: memory},
		{Label: "SWAP", Value: swap},
		{Label: "VOLUME", Value: Join(" · ", st.Volume.FS, gigabytes(st.Volume.Total, 1e9))},
		{Label: "UPTIME", Value: up},
		{Label: "PROCESSES", Value: processes},
		{Label: "SIP", Value: sip},
	})
}

// sessionFacts is who is at the station and how: the user, the shell,
// the terminal, where and when, and the conn that is running.
func sessionFacts(s station.Login, now time.Time) []Fact {
	// Who, likewise, is on the header. What is left is what the header
	// does not carry: which user that is to the kernel, and whether
	// they can act as one.
	userLine := ""
	if s.UID != "" {
		userLine = "UID " + s.UID
	}
	if s.Admin {
		userLine = Join(" · ", userLine, "ADMIN")
	}
	shell := ""
	if s.Shell != "" {
		shell = Join(" ", filepath.Base(s.Shell), s.ShellVer)
	}
	terminal := Join(" ", s.Terminal, s.TerminalVer)
	if s.Tmux {
		terminal = Join(" · ", terminal, "IN TMUX")
	}
	sessionLine := ""
	if s.SSHFrom != "" {
		sessionLine = "SSH FROM " + s.SSHFrom
	}
	binary := ""
	if s.Exe != "" {
		binary = Join(" · ", tilde(s.Exe, s.Home), sizeShort(uint64(s.ExeSize)))
	}
	process := ""
	if s.PID > 0 {
		process = fmt.Sprintf("PID %d · PARENT %d", s.PID, s.PPID)
	}
	threads := ""
	if s.Threads > 0 {
		threads = strconv.Itoa(s.Threads) + " THREADS"
	}
	env := ""
	if s.EnvCount > 0 {
		env = fmt.Sprintf("%d VARIABLES · PATH %d ENTRIES", s.EnvCount, s.PathCount)
	}
	return kept([]Fact{
		{Label: "USER", Value: userLine},
		{Label: "SHELL", Value: shell},
		{Label: "TTY", Value: s.TTY},
		{Label: "TERMINAL", Value: terminal},
		{Label: "SESSION", Value: sessionLine},
		{Label: "LOCALE", Value: s.Lang},
		{Label: "TIME ZONE", Value: timeZone(s.Zone, now)},
		{Label: "CWD", Value: tilde(s.Cwd, s.Home), Path: true},
		{Label: "PROCESS", Value: process},
		{Label: "ENV", Value: env},
		{Label: "RUNTIME", Value: Join(" · ", s.GoVersion, s.Platform, threads)},
		{Label: "BINARY", Value: binary, Path: true},
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
	return Join(" · ", name, utc, now.Format("15:04")+" LOCAL")
}

// stateCheck is where conn keeps its state: the directory, or the one it
// would be made in, must be writable.
func stateCheck(s station.StateDir, home string) check {
	c := check{label: "STATE", value: tilde(s.Path, home), path: true, status: nominal}
	switch s.Problem {
	case "":
	case station.StateNotDir:
		c.status, c.fault = "NOT A DIR", true
	case station.StateReadOnly:
		c.status, c.fault = "READ ONLY", true
	default:
		c.status, c.fault = "NO PATH", true
	}
	return c
}

// A Config is conn's own configuration file as it read: where it is,
// whether there is one, what went wrong reading it, and the theme it
// names where conn has no sheet by that name. The console does not
// read the file; conn does, and says here how it went.
type Config struct {
	Path    string
	Present bool
	Err     error  // there, and would not read: it would not parse, or names a ground that is neither
	NoTheme string // the theme it names, where conn has no sheet by that name
}

// configCheck is conn's own configuration file: where it is, and
// whether it read. A machine with no file is the ordinary case and no
// fault — every default stands — but the line still names the path, so
// the reader knows where to put one, and says DEFAULTS after it, so the
// path is not taken for a file that is there. A file that is there and will not
// read is a fault: somebody wrote it meaning it to be read. So is one
// naming a theme conn does not have: conn comes up in its own, and the
// file was written meaning otherwise.
func configCheck(c Config, home string) check {
	k := check{label: "CONFIG", value: tilde(c.Path, home), path: true, status: nominal}
	// A station nothing was read of has no config to report on, and a
	// fault is a claim conn cannot back up.
	if c.Path == "" {
		k.status = unchecked
		return k
	}
	switch {
	case !c.Present:
		// Nominal, and said: the path alone read as a file that is there.
		k.value += " · DEFAULTS"
	case c.Err != nil:
		k.status, k.fault = notRead, true
	case c.NoTheme != "":
		k.status, k.fault = noTheme, true
	}
	return k
}

// tilde writes a path under home from ~.
func tilde(path, home string) string {
	if home != "" && (path == home || strings.HasPrefix(path, home+"/")) {
		return "~" + path[len(home):]
	}
	return path
}

// diskCheck is the room on the volume under home. It is LOW under a
// tenth of the volume, with the tenth held between 5 and 50 GB: a small
// disk is not low at a few hundred megabytes short of a tenth, and a
// vast one is not low with fifty gigabytes free.
func diskCheck(v station.Volume) check {
	if v.Total == 0 {
		return check{label: "DISK", value: "UNREAD", status: unknown}
	}
	// How much is left. How much there is altogether is the volume's
	// own row, two columns to the left of this one.
	c := check{label: "DISK", value: gigabytes(v.Free, 1e9) + " FREE", status: nominal}
	if v.Free < min(max(v.Total/diskLowShare, diskLowFloor), diskLowCeiling) {
		c.status, c.fault = "LOW", true
	}
	return c
}

// memoryCheck is how memory stands. Where the kernel keeps a verdict of
// its own it is reported and not second-guessed: it is the kernel that
// will act on the pressure, and a percentage conn judged for itself
// would be a second opinion over the one that matters. The system
// column already says how much is left, so the check says the thing the
// column cannot.
//
// Where no verdict is published the check is the memory left for new
// work, LOW under a tenth.
func memoryCheck(m station.Machine) check {
	switch m.Pressure {
	case station.PressureNormal:
		return check{label: "MEMORY", value: "NORMAL PRESSURE", status: nominal}
	case station.PressureWarning:
		return check{label: "MEMORY", value: "UNDER PRESSURE", status: "WARNING", fault: true}
	case station.PressureCritical:
		return check{label: "MEMORY", value: "UNDER PRESSURE", status: "CRITICAL", fault: true}
	}
	if m.Available < 0 || m.Memory == 0 {
		return check{label: "MEMORY", value: "UNREAD", status: unknown}
	}
	c := check{label: "MEMORY", value: strconv.Itoa(m.Available) + "% OF " + gigabytes(m.Memory, 1<<30) + " AVAILABLE", status: nominal}
	if m.Available < memoryLowPercent {
		c.status, c.fault = "LOW", true
	}
	return c
}

// loadCheck is the load average against the processors: HIGH when the
// last minute's exceeds them.
func loadCheck(m station.Machine) check {
	// Whether the machine answered, and not whether the answer was
	// zero. A machine with nothing running on it has a load of exactly
	// zero and has answered; reading that as a machine that would not
	// answer called the quietest reading there is no reading at all.
	if !m.LoadRead {
		return check{label: "LOAD", value: "UNREAD", status: unknown}
	}
	load := fmt.Sprintf("%.2f %.2f %.2f", m.Load[0], m.Load[1], m.Load[2])
	if m.CPUs == 0 {
		return check{label: "LOAD", value: load + " · NO CORE COUNT TO CHECK AGAINST", status: unchecked}
	}
	c := check{label: "LOAD", value: fmt.Sprintf("%s · %d CORES", load, m.CPUs), status: nominal}
	if m.Load[0] > float64(m.CPUs) {
		c.status, c.fault = "HIGH", true
	}
	return c
}

// networkCheck is the interfaces that reach past the machine: DOWN when
// none does.
func networkCheck(n station.Network, read bool) check {
	if !read {
		return check{label: "NET", value: "UNREAD", status: unknown}
	}
	if n.Up == 0 {
		return check{label: "NET", value: "NO INTERFACE UP", status: "DOWN", fault: true}
	}
	// An interface being up is a cable being in. Whether the machine
	// can reach anything past its own link is whether it has somewhere
	// to send what is not local, and that is a thing the kernel knows
	// and will say. A table conn could not read leaves the older
	// reading standing rather than claiming either way.
	if n.RouteRead && n.Route == "" {
		return check{label: "NET", value: fmt.Sprintf("%d UP · NO DEFAULT ROUTE", n.Up), status: "DOWN", fault: true}
	}
	return check{label: "NET", value: fmt.Sprintf("%s · %d UP", n.First, n.Up), status: nominal}
}

// powerCheck is what the machine runs on: LOW on a battery under a tenth
// that is discharging.
func powerCheck(p station.Power) check {
	if p.Source == "" {
		return check{label: "POWER", value: "UNREAD", status: unknown}
	}
	source := "AC POWER"
	if p.Source == "battery" {
		source = "BATTERY"
	}
	value := source
	if p.Percent >= 0 {
		value = Join(" · ", source, strconv.Itoa(p.Percent)+"%", p.State)
		switch {
		case p.Remaining != "" && p.State == "discharging":
			value += " · " + p.Remaining + " LEFT"
		case p.Remaining != "" && p.State == "charging":
			value += " · " + p.Remaining + " TO FULL"
		}
	}
	c := check{label: "POWER", value: value, status: nominal}
	if p.State == "discharging" && p.Percent >= 0 && p.Percent < powerLowPercent {
		c.status, c.fault = "LOW", true
	}
	return c
}

// clockCheck is the system clock against the one time conn knows for
// sure has passed, the build's commit: a clock behind it is wrong.
func clockCheck(b station.Build, now time.Time) check {
	if b.Time.IsZero() {
		return check{label: "CLOCK", value: "NO BUILD TIME TO CHECK AGAINST", status: unchecked}
	}
	day := strings.ToUpper(b.Time.UTC().Format("02-Jan-2006"))
	if now.Before(b.Time) {
		return check{label: "CLOCK", value: "BEFORE THE BUILD OF " + day, status: "BEHIND", fault: true}
	}
	return check{label: "CLOCK", value: "AFTER THE BUILD OF " + day, status: nominal}
}

// kept is the facts with something to say.
func kept(facts []Fact) []Fact {
	var out []Fact
	for _, f := range facts {
		if strings.TrimSpace(f.Value) != "" {
			out = append(out, f)
		}
	}
	return out
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

// Uptime is how long the machine has been up, in days, hours and minutes.
func Uptime(booted, now time.Time) string {
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
