package main

import (
	"flag"
	"slices"
	"strings"
	"testing"

	"github.com/w0zro/conn/internal/work"
)

// A folder of two repositories, shop's api and web, each running a
// server, and a third project elsewhere with a server of its own.
func shopHere(root string) here {
	listen := func(port string) []work.Socket {
		return []work.Socket{{Proto: "TCP", Addr: "127.0.0.1:" + port, State: "LISTEN"}}
	}
	return here{root: root, group: "/p/shop", read: read{reading: reading{projects: []work.Project{
		{Path: "/p/shop/api", Entries: []work.Entry{
			{PID: 10, Kind: work.KindRun, Command: "node server.js", Sockets: listen("4000")},
		}},
		{Path: "/p/shop/web", Entries: []work.Entry{
			{PID: 20, Kind: work.KindRun, Command: "vite", Sockets: listen("3000")},
			{PID: 21, Kind: work.KindRun, Command: "node build.js"},
		}},
		{Path: "/p/blog", Entries: []work.Entry{
			{PID: 30, Kind: work.KindRun, Command: "node index.js", Sockets: listen("5000")},
		}},
	}}}}
}

// A name reaches the agent's own repository first, then the folder its
// repository is in, and no further. A port and a pid name one process
// on the machine, wherever it is.
func TestAnAddressReachesTheNearestWorkFirst(t *testing.T) {
	for _, c := range []struct {
		root, addr string
		pid        int
		err        string
	}{
		{"/p/shop/api", "node", 10, ""},
		{"/p/shop/web", "node", 21, ""},
		{"/p/shop/api", "vite", 20, ""},
		{"/p/shop/api", ":5000", 30, ""},
		{"/p/shop/api", "30", 30, ""},
		{"/p/shop", "node", 0, "names 2 processes: 10, 21"},
		{"/p/shop/api", "puma", 0, "no process here answers to puma"},
	} {
		f, err := shopHere(c.root).find(c.addr)
		switch {
		case c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)):
			t.Errorf("%s from %s: %v, want an error saying %q", c.addr, c.root, err, c.err)
		case c.err == "" && (err != nil || f.PID != c.pid):
			t.Errorf("%s from %s found %d (%v), want %d", c.addr, c.root, f.PID, err, c.pid)
		}
	}
}

// Flags stand before the address or after it.
func TestFlagsStandEitherSideOfTheAddress(t *testing.T) {
	for _, args := range [][]string{{"-n", "5", "web"}, {"web", "-n", "5"}} {
		fs := flag.NewFlagSet("l", flag.ContinueOnError)
		n := fs.Int("n", 100, "")
		words, err := parseAnywhere(fs, args)
		if err != nil || *n != 5 || !slices.Equal(words, []string{"web"}) {
			t.Errorf("%q parsed as %q, n %d, %v", args, words, *n, err)
		}
	}
}

// A process with nowhere conn can read it from says why: no terminal,
// or a terminal that is not one of conn's panes.
func TestWhatConnCannotReadIsSaid(t *testing.T) {
	h := here{}
	for _, c := range []struct {
		tty, want string
	}{
		{"", "has no terminal"},
		{"ttys003", "a terminal conn does not hold"},
	} {
		_, err := h.said(found{Entry: work.Entry{PID: 1, Command: "vite", TTY: c.tty}}, 10)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("a row on %q: %v, want %q", c.tty, err, c.want)
		}
	}
}

// What follows -- is one line as written, or words quoted only where
// sh would read them otherwise.
func TestTheCommandIsALineOrWords(t *testing.T) {
	for _, c := range []struct {
		words []string
		want  string
	}{
		{[]string{"npm run dev | tee log"}, "npm run dev | tee log"},
		{[]string{"go", "test", "./..."}, "go test ./..."},
		{[]string{"sh", "-c", "echo a; exit 3"}, "sh -c 'echo a; exit 3'"},
	} {
		if got := shellLine(c.words); got != c.want {
			t.Errorf("%q is %q, want %q", c.words, got, c.want)
		}
	}
}

// A verb that changes a process reaches the caller's own work, the
// folder of repositories it is in included, and not another project.
func TestActingReachesOnlyTheCallersWork(t *testing.T) {
	h := shopHere("/p/shop/api")
	for addr, want := range map[string]bool{"vite": true, ":4000": true, ":5000": false, "30": false} {
		f, err := h.find(addr)
		if err != nil {
			t.Fatalf("%s: %v", addr, err)
		}
		if got := h.reaches(f); got != want {
			t.Errorf("%s reached: %v, want %v", addr, got, want)
		}
	}
}

// In a worktree, a name reaches the rows working in that worktree
// before the repository's others: an agent in one worktree means its
// own web, not another worktree's.
func TestANameReachesTheCallersWorktreeFirst(t *testing.T) {
	h := here{root: "/p/shop", tree: "/p/shop/.claude/worktrees/fix", read: read{reading: reading{projects: []work.Project{
		{Path: "/p/shop", Entries: []work.Entry{
			{PID: 10, Kind: work.KindRun, Command: "vite", Cwd: "/p/shop"},
			{PID: 20, Kind: work.KindRun, Command: "vite", Cwd: "/p/shop/.claude/worktrees/fix/web"},
		}},
	}}}}
	if f, err := h.find("vite"); err != nil || f.PID != 20 {
		t.Errorf("vite from the worktree found %d (%v), want 20", f.PID, err)
	}
	h.tree = ""
	if _, err := h.find("vite"); err == nil {
		t.Error("vite from the repository's own checkout names one of two")
	}
}
