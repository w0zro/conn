package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLsListsWhatTheNavigatorLists(t *testing.T) {
	// conn ls is the navigator's list, plain: one run per line — the head
	// pid, the place, the row's name, its state, its exit and its ports —
	// built from the same places, processes and held shells, so a script
	// reads what the window shows. A shell the plan named web running cat
	// is web, running, in its repository.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(root, "app")
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Skipf("git init: %v\n%s", err, out)
	}
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "conn"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(home, "conn", "config.json"), `{"projectsDir": "`+root+`"}`); err != nil {
		t.Fatal(err)
	}

	m := connected(t, withProcList(90, 14, []Project{{Name: "app", Path: repo}}, nil))
	m.server.open(repo, "cat", "web")
	m = pump(t, m, func(m model) bool { return len(m.terms) == 1 }, 10*time.Second)
	var pid int
	for p := range m.terms {
		pid = p
	}

	var out strings.Builder
	if err := runLS(&out); err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("%d\t%s\tweb\trunning\t\t\n", pid, repo)
	if !strings.Contains(out.String(), want) {
		t.Errorf("ls = %q, want a line %q", out.String(), want)
	}
}

func TestLsWithoutAServerListsNoShells(t *testing.T) {
	// No server means no shells are held: the list without them — the
	// processes of the places, and the machine's own under global — not
	// an error, and certainly not a server started just to say so.
	tmuxOnSocket(t)
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "conn"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(home, "conn", "config.json"), `{"projectsDir": "`+t.TempDir()+`"}`); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	if err := runLS(&out); err != nil {
		t.Fatal(err)
	}
	for line := range strings.SplitSeq(strings.TrimSpace(out.String()), "\n") {
		if line != "" && !strings.Contains(line, "\t"+globalPlace+"\t") {
			t.Errorf("ls with no server listed %q, want only the machine's own under global", line)
		}
	}
	if _, err := tmuxCommand("has-session", "-t", tmuxSession); !errors.Is(err, errNoServer) {
		t.Errorf("has-session after ls: %v, want no server started by asking", err)
	}
}
