package main

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/w0zro/conn/internal/draw"
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
