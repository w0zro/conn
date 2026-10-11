package main

import (
	"strings"
	"testing"

	"github.com/w0zro/conn/internal/work"
)

// What runs the CLI is not a row: the CLI, and the shells an agent ran
// it in, which have no terminal and are gone with the command. The
// contact above them is, and so is a person's shell, which has one.
func TestTheCLIIsNotItsOwnRow(t *testing.T) {
	table := []work.Process{
		{PID: 10, PPID: 1, TTY: "ttys009", Command: "claude", Args: []string{"claude"}},
		{PID: 11, PPID: 10, Command: "zsh", Args: []string{"/bin/zsh", "-c", "conn ls"}},
		{PID: 12, PPID: 11, Command: "conn", Args: []string{"conn", "ls"}},
		{PID: 20, PPID: 1, TTY: "ttys004", Command: "zsh", Args: []string{"-zsh"}},
		{PID: 21, PPID: 20, TTY: "ttys004", Command: "conn", Args: []string{"conn", "ls"}},
	}
	for self, want := range map[int][]int{12: {11, 12}, 21: {21}} {
		out := calling(table, self)
		if len(out) != len(want) {
			t.Errorf("run as %d, %v left out; want %v", self, out, want)
		}
		for _, pid := range want {
			if !out[pid] {
				t.Errorf("run as %d, %d stays a row", self, pid)
			}
		}
	}
}

// A row a line, its address in it: the name it is declared under, or
// its program's; its ports; and the command under what runs it.
func TestLsWritesARowALine(t *testing.T) {
	projects := []work.Project{{Path: "/home/me/projects/shop", Entries: []work.Entry{
		{PID: 0, Status: work.StatusDown, Kind: work.KindRun, Command: "bundle exec puma", Declared: "web@%2Fhome%2Fme%2Fprojects%2Fshop"},
		{PID: 40, Status: work.StatusActive, Kind: work.KindRun, Command: "postgres -D data", Sockets: []work.Socket{{Proto: "TCP", Addr: "127.0.0.1:5433", State: "LISTEN"}}},
		{PID: 41, Status: work.StatusIdle, Kind: work.KindShell, Command: "zsh", Depth: 1},
	}}}
	var b strings.Builder
	_ = writeLs(&b, projects, "/home/me")
	got := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	if len(got) != 4 || got[0] != "~/projects/shop" {
		t.Fatalf("ls wrote:\n%s", b.String())
	}
	for i, want := range [][]string{
		{"-", "DOWN", "run", "web", "-", "bundle exec puma"},
		{"40", "ACTIVE", "run", "postgres", "5433", "postgres -D data"},
		{"41", "IDLE", "shell", "zsh", "-", "zsh"},
	} {
		if f := strings.Fields(got[i+1]); strings.Join(f, " ") != strings.Join(want, " ") {
			t.Errorf("row %d reads %q, want %q", i, f, want)
		}
	}
	if !strings.HasSuffix(got[3], "    zsh") {
		t.Errorf("a row one deep is not indented under what runs it: %q", got[3])
	}
}
