package main

import (
	"strings"
	"testing"
	"time"
)

// A station on file: a laptop on battery, in the evening in Los Angeles,
// built off a commit past the tag.
var (
	testNow     = time.Date(2026, 9, 8, 19, 58, 41, 0, time.FixedZone("PDT", -7*3600))
	testStation = station{
		machine: machine{
			system: "macOS 26.6.2", systemBuild: "25G83", kernel: "Darwin 25.6.0",
			model: "Mac15,6", processor: "Apple M3 Pro", cpus: 11, perfCores: 5, effCores: 6,
			memory: 18 << 30, available: 77, pressure: pressureNormal,
			swapTotal: 5 << 30, swapUsed: 3<<30 + 700<<20, swapEncrypt: true,
			booted:    time.Date(2026, 9, 4, 0, 47, 0, 0, time.UTC),
			load:      [3]float64{1.85, 2.07, 1.99},
			loadRead:  true,
			processes: 747,
			power:     power{source: "battery", percent: 81, state: "discharging", remaining: "9:04"},
			sip:       "enabled",
			page:      16384,
		},
		login: login{
			user: "w0zro", uid: "501", admin: true, host: "station", home: "/Users/w0zro",
			shell: "/bin/zsh", shellVer: "5.9", tty: "ttys004",
			terminal: "ghostty", terminalVer: "1.3.1",
			lang: "en_US.UTF-8", zone: "America/Los_Angeles",
			cwd: "/Users/w0zro/projects/w0zro/conn", pid: 67032, ppid: 67031,
			sshFrom:  "10.0.0.5",
			envCount: 62, pathCount: 23,
			exe: "/Users/w0zro/projects/w0zro/conn/conn", exeSize: 5_500_000,
			term:      "xterm-256color · truecolor",
			goVersion: "go1.27.0", platform: "darwin/arm64", threads: 11,
		},
		build:   build{tag: "0.7.0", commit: "4af550d", time: time.Date(2026, 9, 9, 2, 55, 24, 0, time.UTC), modified: true},
		volume:  volume{fs: "apfs", free: 412_000_000_000, total: 994_662_584_320},
		network: network{up: 2, first: "en0 192.168.68.58"},
		netRead: true,
		state:   stateDir{path: "/Users/w0zro/.local/state/conn"},
	}
)

// The station is worded as the console says it.
func TestStationIsWorded(t *testing.T) {
	r := compose(testStation, testNow)
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
	got := map[string]fact{}
	for _, f := range append(r.system, r.login...) {
		got[f.label] = f
	}
	for label, value := range want {
		if got[label].value != value {
			t.Errorf("%s: %q, want %q", label, got[label].value, value)
		}
	}
	if !got["CWD"].path || !got["BINARY"].path || got["SHELL"].path {
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
	r := compose(station{}, testNow)
	if r.station != "someone@somewhere" || r.version != "" || r.note != "(devel)" || r.build != "" {
		t.Errorf("identification: %+v", r)
	}
	// The one row left is the clock's own zone, which is read from the
	// process rather than from the station. Where a session is reached
	// from is not known of a station nothing was read of, and the row
	// that used to call every such station LOCAL is not written.
	if len(r.system) != 0 || len(r.login) != 1 || r.login[0].label != "TIME ZONE" {
		t.Errorf("readout: %+v %+v", r.system, r.login)
	}
	for _, c := range r.checks {
		if c.fault || (c.status != unknown && c.status != unchecked && c.label != "STATE") {
			t.Errorf("%+v", c)
		}
	}
}

// Each check's threshold, either side of it.
func TestChecksHoldTheirThresholds(t *testing.T) {
	gb := uint64(1000 * 1000 * 1000)
	for _, c := range []struct {
		name string
		v    volume
		low  bool
	}{
		{"a tenth of a laptop", volume{free: 48 * gb, total: 494 * gb}, true},
		{"just over a tenth", volume{free: 50 * gb, total: 494 * gb}, false},
		{"a vast disk at a fiftieth", volume{free: 80 * gb, total: 4000 * gb}, false},
		{"a vast disk under the ceiling", volume{free: 49 * gb, total: 4000 * gb}, true},
		{"a small disk under the floor", volume{free: 4 * gb, total: 32 * gb}, true},
		{"a small disk over the floor", volume{free: 6 * gb, total: 32 * gb}, false},
	} {
		if got := diskCheck(c.v); got.fault != c.low || (c.low && got.status != "LOW") {
			t.Errorf("disk, %s: %+v", c.name, got)
		}
	}
	if diskCheck(volume{}).status != unknown {
		t.Error("an unread volume is not unknown")
	}

	// A kernel that keeps its own verdict is reported and not
	// second-guessed, however much memory is left beside it.
	m := testStation.machine
	m.available = 9
	if got := memoryCheck(m); got.fault || got.value != "NORMAL PRESSURE" {
		t.Errorf("memory under a normal kernel: %+v", got)
	}
	m.pressure = pressureWarning
	if got := memoryCheck(m); !got.fault || got.status != "WARNING" {
		t.Errorf("memory under pressure: %+v", got)
	}
	m.pressure = pressureCritical
	if got := memoryCheck(m); !got.fault || got.status != "CRITICAL" {
		t.Errorf("memory under critical pressure: %+v", got)
	}

	// Without a verdict of the kernel's, the check is what is left.
	m.pressure = ""
	if got := memoryCheck(m); !got.fault || got.status != "LOW" {
		t.Errorf("memory at 9%%: %+v", got)
	}
	m.available = 10
	if got := memoryCheck(m); got.fault {
		t.Errorf("memory at 10%%: %+v", got)
	}
	m.available = -1
	if got := memoryCheck(m); got.status != unknown {
		t.Errorf("memory unread: %+v", got)
	}

	m = testStation.machine
	m.load[0] = 11.01
	if got := loadCheck(m); !got.fault || got.status != "HIGH" {
		t.Errorf("load over the cores: %+v", got)
	}
	m.cpus = 0
	if got := loadCheck(m); got.fault || got.status != unchecked {
		t.Errorf("load with no core count: %+v", got)
	}
	// A machine with nothing running on it answered, and said zero.
	m = testStation.machine
	m.load = [3]float64{}
	if got := loadCheck(m); got.fault || got.status != nominal || got.value != "0.00 0.00 0.00 · 11 CORES" {
		t.Errorf("load of nothing at all: %+v", got)
	}
	m.loadRead = false
	if got := loadCheck(m); got.status != unknown {
		t.Errorf("load unread: %+v", got)
	}

	if got := networkCheck(network{}, true); !got.fault || got.status != "DOWN" {
		t.Errorf("no interface: %+v", got)
	}
	if got := networkCheck(network{up: 1, first: "eth0 2001:db8::1"}, true); got.fault || got.value != "eth0 2001:db8::1 · 1 UP" {
		t.Errorf("an IPv6 interface: %+v", got)
	}
	if got := networkCheck(network{}, false); got.status != unknown {
		t.Errorf("unread network: %+v", got)
	}

	for _, c := range []struct {
		p     power
		value string
		low   bool
	}{
		{power{source: "battery", percent: 9, state: "discharging", remaining: "0:31"}, "BATTERY · 9% · discharging · 0:31 LEFT", true},
		{power{source: "battery", percent: 10, state: "discharging"}, "BATTERY · 10% · discharging", false},
		{power{source: "ac", percent: 9, state: "charging", remaining: "1:12"}, "AC POWER · 9% · charging · 1:12 TO FULL", false},
		{power{source: "ac", percent: 100, state: "charged"}, "AC POWER · 100% · charged", false},
		{power{source: "ac", percent: -1}, "AC POWER", false},
	} {
		got := powerCheck(c.p)
		if got.value != c.value || got.fault != c.low {
			t.Errorf("power %+v: %+v", c.p, got)
		}
	}
	if powerCheck(power{percent: -1}).status != unknown {
		t.Error("unread power is not unknown")
	}

	b := testStation.build
	if got := clockCheck(b, b.time.Add(-time.Hour)); !got.fault || got.status != "BEHIND" {
		t.Errorf("a clock before the build: %+v", got)
	}
	if got := clockCheck(b, testNow); got.fault || got.value != "AFTER THE BUILD OF 09-SEP-2026" {
		t.Errorf("a clock after the build: %+v", got)
	}
	if got := clockCheck(build{}, testNow); got.status != unchecked {
		t.Errorf("no build time: %+v", got)
	}

	for problem, status := range map[string]string{"": nominal, stateNotDir: "NOT A DIR", stateReadOnly: "READ ONLY", stateNoPath: "NO PATH"} {
		got := stateCheck(stateDir{path: "/Users/w0zro/.local/state/conn", problem: problem}, "/Users/w0zro")
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
	if firstVersion("zsh 5.9 (arm-apple-darwin23.0.0)") != "5.9" || firstVersion("GNU bash, version 5.2.37(1)-release") != "5.2.37" || firstVersion("") != "" {
		t.Errorf("firstVersion: %q %q", firstVersion("zsh 5.9 (arm-apple-darwin23.0.0)"), firstVersion("GNU bash, version 5.2.37(1)-release"))
	}
	if tilde("/Users/x/p", "/Users/x") != "~/p" || tilde("/Users/xy", "/Users/x") != "/Users/xy" || tilde("/p", "") != "/p" {
		t.Errorf("tilde: %q %q", tilde("/Users/x/p", "/Users/x"), tilde("/Users/xy", "/Users/x"))
	}
	booted := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	if got := uptime(booted, booted.Add(90*time.Minute)); got != "01H 30M" {
		t.Errorf("uptime: %q", got)
	}
	if got := timeZone("", testNow); got != "PDT · UTC-07:00 · 19:58 LOCAL" {
		t.Errorf("time zone without a name: %q", got)
	}
	if got := join(" · ", "", " a ", "", "b"); got != "a · b" {
		t.Errorf("join: %q", got)
	}
}

// The machine this test runs on can be read without a fuss, and what it
// says is in shape. This is the one test that touches the machine.
func TestTheStationCanBeRead(t *testing.T) {
	st := readStation()
	r := compose(st, time.Now())
	if r.station == "" || strings.HasPrefix(r.station, "someone@") {
		t.Errorf("no station: %q", r.station)
	}
	if len(r.system) < 6 || len(r.login) < 8 {
		t.Errorf("readout thin: %d system, %d session\n%+v\n%+v", len(r.system), len(r.login), r.system, r.login)
	}
	if len(r.checks) != 7+len(st.tools) {
		t.Errorf("%d checks, not %d: %+v", len(r.checks), 7+len(st.tools), r.checks)
	}
	for _, c := range r.checks {
		if c.label == "" || c.value == "" || c.status == "" {
			t.Errorf("check incomplete: %+v", c)
		}
	}
	if st.machine.page == 0 || st.machine.kernel == "" || st.login.goVersion == "" {
		t.Errorf("the machine was not read: %+v", st.machine)
	}
}

// Inside tmux, TERM_PROGRAM is tmux. The row said so twice and never
// said what was drawing the screen, so the client's own answer is
// asked of the server, which conn tells to keep it current.
func TestTheTerminalIsTheOneBeingLookedAt(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "ghostty")
	t.Setenv("TERM_PROGRAM_VERSION", "1.3.1")
	if name, ver := terminalProgram(nil); name != "ghostty" || ver != "1.3.1" {
		t.Errorf("outside tmux: %q %q", name, ver)
	}

	t.Setenv("TERM_PROGRAM", "tmux")
	t.Setenv("TERM_PROGRAM_VERSION", "3.5a")
	client := map[string]string{"TERM_PROGRAM": "ghostty", "TERM_PROGRAM_VERSION": "1.3.1"}
	if name, ver := terminalProgram(client); name != "ghostty" || ver != "1.3.1" {
		t.Errorf("inside a server that carries it: %q %q", name, ver)
	}
	// A server that was not told to carry it, or a terminal that
	// announces nothing, leaves the row the one thing that is true.
	if name, ver := terminalProgram(nil); name != "" || ver != "" {
		t.Errorf("inside a server that does not: %q %q", name, ver)
	}

	// The origin is read the same way, and the environment conn was
	// started with is believed first: it is this process's own.
	t.Setenv("SSH_CONNECTION", "10.0.0.5 51234 10.0.0.9 22")
	if got := sshOrigin(map[string]string{"SSH_CONNECTION": "10.0.0.9 1 2 3"}); got != "10.0.0.5" {
		t.Errorf("own environment: %q", got)
	}
	t.Setenv("SSH_CONNECTION", "")
	if got := sshOrigin(map[string]string{"SSH_CONNECTION": "10.0.0.9 1 2 3"}); got != "10.0.0.9" {
		t.Errorf("the server's answer: %q", got)
	}
	if got := sshOrigin(nil); got != "" {
		t.Errorf("nobody saying: %q", got)
	}
}
