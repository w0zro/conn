package main

import (
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/w0zro/conn/internal/config"
	"github.com/w0zro/conn/internal/draw"
	"github.com/w0zro/conn/internal/station"
	"github.com/w0zro/conn/internal/work"
)

// Fixtures the panel's tests share with internal/work's own, which

// are kept there beside the reading they were written for and copied

// here, since a test file cannot export to another package.

// A process table on file: two terminals of work on this machine. On
// ttys004, a shell running conn. On ttys007, a shell running claude,
// which runs a node of its own and a bash it asked for, which runs a go
// test. On ttys009, a shell at its prompt, and one stopped vim. A root
// process, and one with no terminal working at / — no project, so
// nothing adopts it and the processes view leaves it out.
var (
	processesNow = time.Date(2026, 9, 9, 3, 0, 0, 0, time.UTC)
	testProcs    = []work.Process{
		{PID: 1, PPID: 0, UID: 0, Command: "launchd", Started: processesNow.Add(-5 * 24 * time.Hour)},
		{PID: 500, PPID: 1, UID: 501, Command: "distnoted", State: 'S', Started: processesNow.Add(-4 * 24 * time.Hour), Cwd: "/"},
		{PID: 67031, PPID: 1, UID: 501, TTY: "ttys004", State: 'S', Command: "zsh", Args: []string{"-zsh"}, Started: processesNow.Add(-3 * time.Hour), Cwd: "/Users/w0zro/projects/w0zro/conn"},
		{PID: 67032, PPID: 67031, UID: 501, TTY: "ttys004", Foreground: true, State: 'S', Command: "conn", Args: []string{"./conn"}, Started: processesNow.Add(-90 * time.Second), Cwd: "/Users/w0zro/projects/w0zro/conn"},
		{PID: 67033, PPID: 67032, UID: 501, TTY: "ttys004", State: 'S', Command: "tmux", Args: []string{"tmux", "-S", "/Users/w0zro/.local/state/conn/sock", "attach"}, Started: processesNow.Add(-89 * time.Second), Cwd: "/Users/w0zro/projects/w0zro/conn"},
		{PID: 67040, PPID: 67031, UID: 501, TTY: "ttys005", State: 'S', Command: "zsh", Args: []string{"-zsh"}, Started: processesNow.Add(-90 * time.Second), Cwd: "/Users/w0zro/projects/w0zro/conn"},
		{PID: 70001, PPID: 1, UID: 501, TTY: "ttys007", State: 'S', Command: "zsh", Args: []string{"-zsh"}, Started: processesNow.Add(-2 * time.Hour), Cwd: "/Users/w0zro/projects/w0zro/vim.pro/conjurer"},
		{PID: 70100, PPID: 70001, UID: 501, TTY: "ttys007", Foreground: true, State: 'S', Command: "claude", Args: []string{"claude", "--resume"}, Started: processesNow.Add(-47 * time.Minute), Cwd: "/Users/w0zro/projects/w0zro/vim.pro/conjurer"},
		{PID: 70212, PPID: 70100, UID: 501, TTY: "ttys007", State: 'S', Command: "node", Args: []string{"node", "/opt/claude/mcp.js"}, Started: processesNow.Add(-46 * time.Minute), Cwd: "/Users/w0zro/projects/w0zro/vim.pro/conjurer"},
		{PID: 70300, PPID: 70100, UID: 501, TTY: "ttys007", State: 'S', Command: "bash", Args: []string{"bash", "-c", "go test ./..."}, Started: processesNow.Add(-12 * time.Second), Cwd: "/Users/w0zro/projects/w0zro/vim.pro/conjurer/internal"},
		{PID: 70301, PPID: 70300, UID: 501, TTY: "ttys007", State: 'R', Command: "go", Args: []string{"go", "test", "./..."}, Started: processesNow.Add(-11 * time.Second), Cwd: "/Users/w0zro/projects/w0zro/vim.pro/conjurer/internal"},
		{PID: 80001, PPID: 1, UID: 501, TTY: "ttys009", Foreground: true, State: 'S', Command: "zsh", Args: []string{"-zsh"}, Started: processesNow.Add(-26 * time.Hour), Cwd: "/Users/w0zro"},
		{PID: 80002, PPID: 80001, UID: 501, TTY: "ttys009", State: 'T', Command: "vim", Args: []string{"vim", "notes.md"}, Started: processesNow.Add(-25 * time.Hour), Cwd: "/Users/w0zro"},
		{PID: 90000, PPID: 1, UID: 502, TTY: "ttys011", State: 'S', Command: "zsh", Args: []string{"-zsh"}, Started: processesNow.Add(-time.Hour), Cwd: "/Users/other"},
	}
	testRoots = func(dir string) string {
		for _, root := range []string{"/Users/w0zro/projects/w0zro/conn", "/Users/w0zro/projects/w0zro/vim.pro/conjurer"} {
			if dir == root || strings.HasPrefix(dir, root+"/") {
				return root
			}
		}
		return dir
	}
	// The two repositories are projects; home is a directory work
	// happens in and nothing more, which is what keeps it from
	// adopting the machine.
	testIsProject = func(dir string) bool {
		return dir == "/Users/w0zro/projects/w0zro/conn" || dir == "/Users/w0zro/projects/w0zro/vim.pro/conjurer"
	}
	// Where the checkouts are kept, which the processes view names its
	// projects against.
	testProjRoots = []string{"/Users/w0zro/projects"}
)

func containersFor(t *testing.T) []work.Container {
	t.Helper()
	cs := work.ParseContainers([]byte(dockerPS), dockerNow)
	if len(cs) != 4 {
		t.Fatalf("parsed %d containers, want 4", len(cs))
	}
	return cs
}

var dockerNow = time.Date(2026, 9, 15, 20, 0, 0, 0, time.UTC)

// What docker ps --format '{{json .}}' says of a compose project: two
// services publishing a port, one of them with a health check, a worker
// that exited badly, and a container docker run started, which compose
// wrote no directory on.
const dockerPS = `{"ID":"9f1c2d3e4a5b","Names":"compose-demo-web-1","Image":"nginx:alpine","State":"running","Status":"Up 3 minutes (healthy)","Ports":"0.0.0.0:8438->80/tcp, [::]:8438->80/tcp","Labels":"com.docker.compose.project=compose-demo,com.docker.compose.service=web,com.docker.compose.project.working_dir=/Users/w0zro/projects/compose-demo,maintainer=NGINX Docker Maintainers <docker-maint@nginx.com>"}
{"ID":"1a2b3c4d5e6f","Names":"compose-demo-cache-1","Image":"redis:alpine","State":"running","Status":"Up 3 minutes","Ports":"0.0.0.0:6390->6379/tcp","Labels":"com.docker.compose.project=compose-demo,com.docker.compose.service=cache,com.docker.compose.project.working_dir=/Users/w0zro/projects/compose-demo"}
{"ID":"abcdef012345","Names":"compose-demo-worker-1","Image":"alpine","State":"exited","Status":"Exited (3) 8 seconds ago","Ports":"","Labels":"com.docker.compose.project=compose-demo,com.docker.compose.service=worker,com.docker.compose.project.working_dir=/Users/w0zro/projects/compose-demo"}
{"ID":"ffee11223344","Names":"stray","Image":"postgres","State":"running","Status":"Up 2 hours","Ports":"0.0.0.0:5432->5432/tcp","Labels":""}
`

// dockerRoots stands in for the root finder: the compose demo is its own
// project, and so is conn.
func dockerRoots(dir string) string {
	for _, root := range []string{"/Users/w0zro/projects/compose-demo", "/Users/w0zro/projects/w0zro/conn"} {
		if dir == root || strings.HasPrefix(dir, root+"/") {
			return root
		}
	}
	return dir
}

// brew services info --all --json as brew writes it: a service never
// started, one running under a pid, and one whose last run ended badly.
const brewInfo = `[
  {"name": "herdr", "running": false, "loaded": false, "pid": null, "exit_code": null, "status": "none",
   "command": "/opt/homebrew/opt/herdr/bin/herdr server", "log_path": "/opt/homebrew/var/log/herdr.log"},
  {"name": "postgresql@14", "running": true, "loaded": true, "pid": 24422, "exit_code": 0, "status": "started",
   "command": "/opt/homebrew/opt/postgresql@14/bin/postgres -D /opt/homebrew/var/postgresql@14",
   "log_path": "/opt/homebrew/var/log/postgresql@14.log"},
  {"name": "redis", "running": false, "loaded": true, "pid": null, "exit_code": 78, "status": "error",
   "command": "/opt/homebrew/opt/redis/bin/redis-server /opt/homebrew/etc/redis.conf", "log_path": "/opt/homebrew/var/log/redis.log"}
]`

// The marks a row can wear, for the tests that tell a row from an
// eyebrow by what stands at the head of it.
var marks = []string{draw.MarkContact, draw.MarkShell, draw.MarkEditor, draw.MarkService, draw.MarkRun}

// isMark says whether a word is a row's mark.
func isMark(s string) bool { return slices.Contains(marks, s) }

// The rows of a view as text, a line to a row; golden holds them to
// the files of record under testdata. internal/console has its own,
// for its own files.
var update = flag.Bool("update", false, "write the golden views under testdata")

func texts(rows []draw.Row) string {
	var b []string
	for _, r := range rows {
		b = append(b, r.Text)
	}
	return strings.Join(b, "\n")
}

// golden holds a rendering to the file of record under testdata. Run
// the tests with -update to write what the console renders now, and
// read the diff before committing it: the file is the design.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	got += "\n"
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to write it)", err)
	}
	if string(want) != got {
		t.Errorf("%s differs from the golden file; run with -update if the change is meant:\n%s", name, got)
	}
}

// stripEscapes drops the color sequences, leaving the cells.
func stripEscapes(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// The station the console is composed from in internal/console's tests,
// for the tests here that bring the console up.
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
