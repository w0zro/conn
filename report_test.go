package main

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/w0zro/conn/internal/draw"
	"github.com/w0zro/conn/internal/station"

	"github.com/w0zro/conn/internal/config"
)

// A station on file: a laptop on battery, in the evening in Los Angeles,
// built off a commit past the tag.
var (
	testNow     = time.Date(2026, 9, 8, 19, 58, 41, 0, time.FixedZone("PDT", -7*3600))
	testStation = station.Station{
		Machine: station.Machine{
			System: "macOS 26.6.2", SystemBuild: "25G83", Kernel: "Darwin 25.6.0",
			Model: "Mac15,6", Processor: "Apple M3 Pro", CPUs: 11, PerfCores: 5, EffCores: 6,
			Memory: 18 << 30, Available: 77, Pressure: station.PressureNormal,
			SwapTotal: 5 << 30, SwapUsed: 3<<30 + 700<<20, SwapEncrypt: true,
			Booted:    time.Date(2026, 9, 4, 0, 47, 0, 0, time.UTC),
			Load:      [3]float64{1.85, 2.07, 1.99},
			LoadRead:  true,
			Processes: 747,
			Power:     station.Power{Source: "battery", Percent: 81, State: "discharging", Remaining: "9:04"},
			SIP:       "enabled",
			Page:      16384,
		},
		Login: station.Login{
			User: "w0zro", UID: "501", Admin: true, Host: "station", Home: "/Users/w0zro",
			Shell: "/bin/zsh", ShellVer: "5.9", TTY: "ttys004",
			Terminal: "ghostty", TerminalVer: "1.3.1",
			Lang: "en_US.UTF-8", Zone: "America/Los_Angeles",
			Cwd: "/Users/w0zro/projects/w0zro/conn", PID: 67032, PPID: 67031,
			SSHFrom:  "10.0.0.5",
			EnvCount: 62, PathCount: 23,
			Exe: "/Users/w0zro/projects/w0zro/conn/conn", ExeSize: 5_500_000,
			Term:      "xterm-256color · truecolor",
			GoVersion: "go1.27.0", Platform: "darwin/arm64", Threads: 11,
		},
		Build:   station.Build{Tag: "0.7.0", Commit: "4af550d", Time: time.Date(2026, 9, 9, 2, 55, 24, 0, time.UTC), Modified: true},
		Volume:  station.Volume{FS: "apfs", Free: 412_000_000_000, Total: 994_662_584_320},
		Network: station.Network{Up: 2, First: "en0 192.168.68.58"},
		NetRead: true,
		State:   station.StateDir{Path: "/Users/w0zro/.local/state/conn"},
		Config: config.State{
			Path: "/Users/w0zro/.config/conn/config.json", Present: true, Names: true, Source: config.RootsFile,
			Roots: []config.RootState{{Path: "/Users/w0zro/projects"}, {Path: "/Users/w0zro/work/checkouts"}},
		},
		// A macOS station needs both, and the console of record is the
		// console this station prints.
		Tools: []station.Tool{{Name: "tmux", Path: "/opt/homebrew/bin/tmux"}, {Name: "lsof", Path: "/usr/sbin/lsof"}},
	}
)

// The station is worded as the console says it.
func TestStationIsWorded(t *testing.T) {
	r := Compose(testStation, testNow)
	if r.version != "0.7.0" || r.note != "(devel)" || r.build != "4af550d · 09-SEP-2026 · MODIFIED" {
		t.Errorf("identification: %q %q %q", r.version, r.note, r.build)
	}
	if r.station != "w0zro@station" || r.clock != "09-Sep-2026  02:58:41 Z" {
		t.Errorf("station and clock: %q %q", r.station, r.clock)
	}
	want := map[string]string{
		"SYSTEM":    "macOS 26.6.2 (25G83)",
		"KERNEL":    "Darwin 25.6.0 · 16 KB PAGES",
		"CPU":       "Apple M3 Pro · 11 CORES (5P + 6E)",
		"MEMORY":    "18 GB · 77% AVAILABLE",
		"SWAP":      "3.7 GB USED OF 5 GB · ENCRYPTED",
		"VOLUME":    "apfs · 995 GB",
		"UPTIME":    "5D 02H 12M · UP SINCE 04-Sep 00:47 Z",
		"PROCESSES": "747",
		"SIP":       "ENABLED",
		"USER":      "UID 501 · ADMIN",
		"SHELL":     "zsh 5.9",
		"TERMINAL":  "ghostty 1.3.1",
		"SESSION":   "SSH FROM 10.0.0.5",
		"TIME ZONE": "America/Los_Angeles · UTC-07:00 · 19:58 LOCAL",
		"CWD":       "~/projects/w0zro/conn",
		"PROCESS":   "PID 67032 · PARENT 67031",
		"ENV":       "62 VARIABLES · PATH 23 ENTRIES",
		"RUNTIME":   "go1.27.0 · darwin/arm64 · 11 THREADS",
		"BINARY":    "~/projects/w0zro/conn/conn · 5.2 MB",
	}
	got := map[string]draw.Fact{}
	for _, f := range append(r.system, r.login...) {
		got[f.Label] = f
	}
	for label, value := range want {
		if got[label].Value != value {
			t.Errorf("%s: %q, want %q", label, got[label].Value, value)
		}
	}
	if !got["CWD"].Path || !got["BINARY"].Path || got["SHELL"].Path {
		t.Error("the paths are not marked as paths, or a shell is")
	}
	if len(r.system) != 10 || len(r.login) != 12 {
		t.Errorf("%d machine, %d session facts", len(r.system), len(r.login))
	}
	statuses := map[string]string{}
	for _, c := range r.checks {
		statuses[c.label] = c.status
		if c.fault {
			t.Errorf("%s faults on a healthy station: %+v", c.label, c)
		}
	}
	for _, label := range []string{"STATE", "DISK", "MEMORY", "LOAD", "NET", "POWER", "CLOCK"} {
		if statuses[label] != nominal {
			t.Errorf("%s is %q", label, statuses[label])
		}
	}
}

// A station with nothing read words to a header and a column of checks
// that know they were not answered, and none of them a fault.
func TestAnEmptyStationIsWorded(t *testing.T) {
	r := Compose(station.Station{}, testNow)
	if r.station != "someone@somewhere" || r.version != "" || r.note != "(devel)" || r.build != "" {
		t.Errorf("identification: %+v", r)
	}
	// The one row left is the clock's own zone, which is read from the
	// process rather than from the station. Where a session is reached
	// from is not known of a station nothing was read of, and the row
	// that used to call every such station LOCAL is not written.
	if len(r.system) != 0 || len(r.login) != 1 || r.login[0].Label != "TIME ZONE" {
		t.Errorf("readout: %+v %+v", r.system, r.login)
	}
	for _, c := range r.checks {
		// STATE and CONFIG are conn's own paths, read from the process
		// rather than from the station, and answer whatever was read.
		if c.fault || (c.status != unknown && c.status != unchecked && c.label != "STATE" && c.label != "CONFIG") {
			t.Errorf("%+v", c)
		}
	}
}

// Each check's threshold, either side of it.
func TestChecksHoldTheirThresholds(t *testing.T) {
	gb := uint64(1000 * 1000 * 1000)
	for _, c := range []struct {
		name string
		v    station.Volume
		low  bool
	}{
		{"a tenth of a laptop", station.Volume{Free: 48 * gb, Total: 494 * gb}, true},
		{"just over a tenth", station.Volume{Free: 50 * gb, Total: 494 * gb}, false},
		{"a vast disk at a fiftieth", station.Volume{Free: 80 * gb, Total: 4000 * gb}, false},
		{"a vast disk under the ceiling", station.Volume{Free: 49 * gb, Total: 4000 * gb}, true},
		{"a small disk under the floor", station.Volume{Free: 4 * gb, Total: 32 * gb}, true},
		{"a small disk over the floor", station.Volume{Free: 6 * gb, Total: 32 * gb}, false},
	} {
		if got := diskCheck(c.v); got.fault != c.low || (c.low && got.status != "LOW") {
			t.Errorf("disk, %s: %+v", c.name, got)
		}
	}
	if diskCheck(station.Volume{}).status != unknown {
		t.Error("an unread volume is not unknown")
	}

	// A kernel that keeps its own verdict is reported and not
	// second-guessed, however much memory is left beside it.
	m := testStation.Machine
	m.Available = 9
	if got := memoryCheck(m); got.fault || got.value != "NORMAL PRESSURE" {
		t.Errorf("memory under a normal kernel: %+v", got)
	}
	m.Pressure = station.PressureWarning
	if got := memoryCheck(m); !got.fault || got.status != "WARNING" {
		t.Errorf("memory under pressure: %+v", got)
	}
	m.Pressure = station.PressureCritical
	if got := memoryCheck(m); !got.fault || got.status != "CRITICAL" {
		t.Errorf("memory under critical pressure: %+v", got)
	}

	// Without a verdict of the kernel's, the check is what is left.
	m.Pressure = ""
	if got := memoryCheck(m); !got.fault || got.status != "LOW" {
		t.Errorf("memory at 9%%: %+v", got)
	}
	m.Available = 10
	if got := memoryCheck(m); got.fault {
		t.Errorf("memory at 10%%: %+v", got)
	}
	m.Available = -1
	if got := memoryCheck(m); got.status != unknown {
		t.Errorf("memory unread: %+v", got)
	}

	m = testStation.Machine
	m.Load[0] = 11.01
	if got := loadCheck(m); !got.fault || got.status != "HIGH" {
		t.Errorf("load over the cores: %+v", got)
	}
	m.CPUs = 0
	if got := loadCheck(m); got.fault || got.status != unchecked {
		t.Errorf("load with no core count: %+v", got)
	}
	// A machine with nothing running on it answered, and said zero.
	m = testStation.Machine
	m.Load = [3]float64{}
	if got := loadCheck(m); got.fault || got.status != nominal || got.value != "0.00 0.00 0.00 · 11 CORES" {
		t.Errorf("load of nothing at all: %+v", got)
	}
	m.LoadRead = false
	if got := loadCheck(m); got.status != unknown {
		t.Errorf("load unread: %+v", got)
	}

	if got := networkCheck(station.Network{}, true); !got.fault || got.status != "DOWN" {
		t.Errorf("no interface: %+v", got)
	}
	if got := networkCheck(station.Network{Up: 1, First: "eth0 2001:db8::1"}, true); got.fault || got.value != "eth0 2001:db8::1 · 1 UP" {
		t.Errorf("an IPv6 interface: %+v", got)
	}
	if got := networkCheck(station.Network{}, false); got.status != unknown {
		t.Errorf("unread network: %+v", got)
	}
	// Links up and nowhere to send what is not local. A table conn
	// could not read says nothing either way and leaves the reading.
	if got := networkCheck(station.Network{Up: 2, First: "en0 10.0.0.2", RouteRead: true}, true); !got.fault || got.status != "DOWN" {
		t.Errorf("no default route: %+v", got)
	}
	if got := networkCheck(station.Network{Up: 2, First: "en0 10.0.0.2"}, true); got.fault {
		t.Errorf("an unread route table faulted: %+v", got)
	}
	if got := networkCheck(station.Network{Up: 2, First: "en0 10.0.0.2", Route: "en0", RouteRead: true}, true); got.fault || got.value != "en0 10.0.0.2 · 2 UP" {
		t.Errorf("a route out: %+v", got)
	}

	for _, c := range []struct {
		p     station.Power
		value string
		low   bool
	}{
		{station.Power{Source: "battery", Percent: 9, State: "discharging", Remaining: "0:31"}, "BATTERY · 9% · discharging · 0:31 LEFT", true},
		{station.Power{Source: "battery", Percent: 10, State: "discharging"}, "BATTERY · 10% · discharging", false},
		{station.Power{Source: "ac", Percent: 9, State: "charging", Remaining: "1:12"}, "AC POWER · 9% · charging · 1:12 TO FULL", false},
		{station.Power{Source: "ac", Percent: 100, State: "charged"}, "AC POWER · 100% · charged", false},
		{station.Power{Source: "ac", Percent: -1}, "AC POWER", false},
	} {
		got := powerCheck(c.p)
		if got.value != c.value || got.fault != c.low {
			t.Errorf("power %+v: %+v", c.p, got)
		}
	}
	if powerCheck(station.Power{Percent: -1}).status != unknown {
		t.Error("unread power is not unknown")
	}

	b := testStation.Build
	if got := clockCheck(b, b.Time.Add(-time.Hour)); !got.fault || got.status != "BEHIND" {
		t.Errorf("a clock before the build: %+v", got)
	}
	if got := clockCheck(b, testNow); got.fault || got.value != "AFTER THE BUILD OF 09-SEP-2026" {
		t.Errorf("a clock after the build: %+v", got)
	}
	if got := clockCheck(station.Build{}, testNow); got.status != unchecked {
		t.Errorf("no build time: %+v", got)
	}

	for problem, status := range map[string]string{"": nominal, station.StateNotDir: "NOT A DIR", station.StateReadOnly: "READ ONLY", station.StateNoPath: "NO PATH"} {
		got := stateCheck(station.StateDir{Path: "/Users/w0zro/.local/state/conn", Problem: problem}, "/Users/w0zro")
		if got.status != status || got.fault != (problem != "") || got.value != "~/.local/state/conn" || !got.path {
			t.Errorf("state %q: %+v", problem, got)
		}
	}
}

// The words for sizes, times and versions.
func TestWordsForNumbers(t *testing.T) {
	if gigabytes(18<<30, 1<<30) != "18 GB" || gigabytes(994_662_584_320, 1e9) != "995 GB" || gigabytes(0, 1e9) != "" {
		t.Errorf("gigabytes: %q %q %q", gigabytes(18<<30, 1<<30), gigabytes(994_662_584_320, 1e9), gigabytes(0, 1e9))
	}
	if sizeShort(4_400_000) != "4.2 MB" || sizeShort(2<<30) != "2 GB" || sizeShort(500) != "1 KB" || sizeShort(0) != "" {
		t.Errorf("sizeShort: %q %q %q %q", sizeShort(4_400_000), sizeShort(2<<30), sizeShort(500), sizeShort(0))
	}
	booted := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	if got := Uptime(booted, booted.Add(90*time.Minute)); got != "01H 30M" {
		t.Errorf("uptime: %q", got)
	}
	if got := timeZone("", testNow); got != "PDT · UTC-07:00 · 19:58 LOCAL" {
		t.Errorf("time zone without a name: %q", got)
	}
	if got := draw.Join(" · ", "", " a ", "", "b"); got != "a · b" {
		t.Errorf("join: %q", got)
	}
}

// The machine this test runs on can be read without a fuss, and what it
// says is in shape. This is the one test that touches the machine.
func TestTheStationCanBeRead(t *testing.T) {
	st := station.Read()
	r := Compose(st, time.Now())
	if r.station == "" || strings.HasPrefix(r.station, "someone@") {
		t.Errorf("no station: %q", r.station)
	}
	// Counting the rows held conn to a machine that answered for
	// everything. The rows a machine can decline to answer for are the
	// ones it has nothing to say about — no terminal, no terminal
	// program that names itself, no ssh it was reached over — and a
	// bare container is not a station conn read badly. What every
	// machine can answer is named instead.
	must := map[string]bool{"USER": true, "TIME ZONE": true, "CWD": true,
		"PROCESS": true, "ENV": true, "RUNTIME": true, "BINARY": true}
	for _, f := range r.login {
		delete(must, f.Label)
	}
	if len(must) > 0 || len(r.system) < 6 {
		t.Errorf("readout thin: %d system, session went unread: %v\n%+v\n%+v",
			len(r.system), slices.Sorted(maps.Keys(must)), r.system, r.login)
	}
	// Seven checks conn always makes, the config file's own, a line for
	// every root configured, and one per tool the platform needs.
	want := 8 + len(st.Config.Roots) + len(st.Tools)
	if len(r.checks) != want {
		t.Errorf("%d checks, not %d: %+v", len(r.checks), want, r.checks)
	}
	for _, c := range r.checks {
		if c.label == "" || c.value == "" || c.status == "" {
			t.Errorf("check incomplete: %+v", c)
		}
	}
	if st.Machine.Page == 0 || st.Machine.Kernel == "" || st.Login.GoVersion == "" {
		t.Errorf("the machine was not read: %+v", st.Machine)
	}
}

// The console says where conn's configuration is and how it read. A
// machine with no file is not a fault; a file that will not parse is.
func TestTheConsoleSaysHowTheConfigRead(t *testing.T) {
	t.Setenv("CONN_ROOTS", "")
	for _, c := range []struct {
		what   string
		body   string // "" writes no file at all
		status string
		fault  bool
	}{
		{"a file that names roots", `{"roots": ["~"]}`, nominal, false},
		{"a file that names none", `{"roots": []}`, noRoots, true},
		{"a file of nothing conn knows", `{"projectsDir": "~/projects"}`, noRoots, true},
		{"a file that will not parse", `{"roots": [`, notRead, true},
		{"no file at all", "", notWritten, true},
		{"a file naming a theme conn has", `{"roots": ["~"], "theme": "datum"}`, nominal, false},
		{"a file naming a theme conn does not have", `{"roots": ["~"], "theme": "solarized"}`, noTheme, true},
		{"a file naming no roots and no such theme", `{"theme": "solarized"}`, noRoots, true},
		{"a file naming a ground", `{"roots": ["~"], "ground": "light"}`, nominal, false},
		{"a file naming neither ground", `{"roots": ["~"], "ground": "grey"}`, noGround, true},
	} {
		home := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		if c.body != "" {
			path := config.Path(home)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(c.body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		k := configCheck(config.ReadState(home), home)
		if k.status != c.status || k.fault != c.fault {
			t.Errorf("%s reads %q (fault %v), not %q (fault %v)", c.what, k.status, k.fault, c.status, c.fault)
		}
	}
}

// The environment standing in front of the file is said on the file's
// own line: the roots below it are then not the ones in it.
func TestTheConsoleSaysWhenTheEnvironmentIsInForce(t *testing.T) {
	home := writeConfig(t, `{"roots": ["/from/the/file"]}`)
	t.Setenv("CONN_ROOTS", "/from/the/environment")
	k := configCheck(config.ReadState(home), home)
	if !strings.Contains(k.value, "CONN_ROOTS IN FORCE") {
		t.Errorf("the line does not say the environment is in force: %q", k.value)
	}
	if k.status != nominal {
		t.Errorf("a file that reads fine is %q", k.status)
	}
}

// A root gets a line of its own, and says what it turned out to be
// here. A root that is not on this machine is not a fault; something
// that is not a directory at all is.
func TestEveryRootGetsALine(t *testing.T) {
	t.Setenv("CONN_ROOTS", "")
	home := writeConfig(t, `{"roots": ["~", "~/nowhere", "~/afile"]}`)
	if err := os.WriteFile(filepath.Join(home, "afile"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	ks := rootChecks(config.ReadState(home), home)
	if len(ks) != 3 {
		t.Fatalf("the roots take %d lines, not 3", len(ks))
	}
	for i, want := range []struct {
		status string
		fault  bool
	}{{nominal, false}, {missing, false}, {"NOT A DIR", true}} {
		if ks[i].status != want.status || ks[i].fault != want.fault {
			t.Errorf("root %d reads %q (fault %v), not %q (fault %v)", i, ks[i].status, ks[i].fault, want.status, want.fault)
		}
		if ks[i].label != "ROOT" {
			t.Errorf("root %d is labelled %q", i, ks[i].label)
		}
	}
}

// Every status is right-aligned in a column statusW wide, and one that
// does not fit runs into the dots that lead to it. The words conn has
// are held to the column here, where the console is not being read.
func TestEveryStatusFitsItsColumn(t *testing.T) {
	for _, status := range []string{nominal, unknown, unchecked, notWritten, noRoots, noTheme, noGround, missing, notRead, "NOT A DIR", "READ ONLY", "NO PATH"} {
		if len(status) > StatusW {
			t.Errorf("%q is %d wide, and the column is %d", status, len(status), StatusW)
		}
	}
}
