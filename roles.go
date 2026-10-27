package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// What a project runs is learned from what is run in it. Every process
// started under a name through conn — by an agent at the command line,
// or from the keys — is written down with its role: a check, which runs
// to completion and passes or fails, or the stack, what keeps running
// while the project is worked on. The keys then run the checks and
// bring up the stack as they were last run here, by whoever ran them,
// and ask Claude Code only where nobody has yet. Agents run almost
// every command a project sees, so what they ran is what the project
// runs; nothing is declared, and the file is written by use.
//
// The file is one line a start, beside the socket, and the last line
// for a name in a project is what the name means there.

type role string

const (
	roleStack role = "stack"
	roleCheck role = "check"
	// A client is not a process but what talks to one: it is kept under
	// the process's name without taking the name from it.
	roleClient role = "client"
	// A taken-off is written under a name in place of its role: the
	// name means nothing in the project until it is run again.
	roleOff role = "off"
)

// A started is one start written down: the project, the name it was
// started under, the command, and the role it was started as.
type started struct {
	Root    string    `json:"root"`
	Name    string    `json:"name"`
	Command string    `json:"command"`
	Role    role      `json:"role"`
	When    time.Time `json:"when"`
}

// startedPath is where the starts are kept: beside the socket.
func startedPath(socket string) string {
	return filepath.Join(filepath.Dir(socket), "started.jsonl")
}

// recordStart writes a start down.
func recordStart(path string, r started) error {
	if r.When.IsZero() {
		r.When = time.Now()
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = f.Write(append(b, '\n'))
	return err
}

// readStarted is every start written down, in order; a line that does
// not read is passed over.
func readStarted(path string) []started {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	var out []started
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		var r started
		if json.Unmarshal(sc.Bytes(), &r) == nil && r.Root != "" && r.Name != "" && r.Command != "" {
			out = append(out, r)
		}
	}
	return out
}

// established is what a role means in a project: each name last run in
// the project's roots as that role, with the command it was last run
// with, in the order the names were first run. A name run since as the
// other role has changed its meaning and is not here.
func established(starts []started, in func(root string) bool, as role) []stacked {
	last := map[string]started{}
	var order []string
	for _, r := range starts {
		if !in(r.Root) {
			continue
		}
		key := r.Root + "\x00" + r.Name
		if r.Role == roleClient {
			key += "\x00client"
		}
		if _, seen := last[key]; !seen {
			order = append(order, key)
		}
		last[key] = r
	}
	var out []stacked
	for _, key := range order {
		if r := last[key]; r.Role == as {
			out = append(out, stacked{name: r.Name, command: r.Command, root: r.Root, kept: true})
		}
	}
	return out
}

// down is what a project has written down as its stack and does not
// run: each name established as the stack in the project that no pane
// of the project stands under, in the order they were first run. ran is
// the names the panes stand under, by the root of each. A run that
// ended is a row of its own, and is not down.
func down(starts []started, ran map[string]map[string]bool, p project) []stacked {
	var out []stacked
	for _, s := range established(starts, p.belongs, roleStack) {
		runs := false
		for root, names := range ran {
			runs = runs || p.belongs(root) && names[s.name]
		}
		if !runs {
			out = append(out, s)
		}
	}
	return out
}

// ranIn is the names the panes stand under, by the root of the project
// each is in.
func ranIn(ps []pane, rootOf func(dir string) string) map[string]map[string]bool {
	ran := map[string]map[string]bool{}
	for _, p := range ps {
		if p.Name == "" || p.Hold || p.Panel || p.Dead {
			continue
		}
		root := rootOf(p.Dir)
		if ran[root] == nil {
			ran[root] = map[string]bool{}
		}
		ran[root][p.Name] = true
	}
	return ran
}

// clientFor is what talks to a process, by the name it is kept under,
// where one has been kept.
func clientFor(starts []started, in func(root string) bool, name string) (stacked, bool) {
	for _, s := range established(starts, in, roleClient) {
		if s.name == name {
			return s, true
		}
	}
	return stacked{}, false
}
