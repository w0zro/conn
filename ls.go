package main

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// conn ls is the faucet on what the navigator lists: every run in every
// place, one per line on stdout, for the reader that is not a person. The
// window shows the same list dressed up; scripts, prompts and grep get it
// plain. It is built the way the navigator builds its own — the places
// found under the roots, the processes filed under them, the shells the
// server holds, the agents by what they advertise — so the two never
// disagree about what is running.

// runLS lists what the navigator lists, in its order, one run per line:
// pid, place, name, state, exit and ports, tab-separated. The pid is the
// run's head, the process a shell was opened around; the name is what the
// row is called — the plan's name for a shell it started, else what was
// run; the state is what the mark says: working, blocked or waiting for
// an agent, failed, done, or running; the exit is how a held shell's
// command ended, empty while it runs; the ports are where the run
// listens. A server not running means no shells are held, which is a
// list without them, not an error — and not a reason to start one, which
// would be an odd side effect of asking a question.
func runLS(w io.Writer) error {
	m, err := readList()
	if err != nil {
		return err
	}
	for _, r := range m.flatten() {
		if r.kind != rowProc {
			continue
		}
		line := []string{
			strconv.Itoa(r.chain().PID),
			r.project.Path,
			m.rowName(r),
			m.stateWord(r),
			m.ended(r),
			strings.Join(runPorts(r.run, r.node), " "),
		}
		if _, err := fmt.Fprintln(w, strings.Join(line, "\t")); err != nil {
			return err
		}
	}
	return nil
}

// readList is a navigator with everything read once and nothing drawn: the
// places, the processes filed under them, the held shells and the agents.
func readList() (model, error) {
	var m model
	places, ok := scanProjects().(projectsMsg)
	if !ok {
		return m, errors.New("could not read the projects")
	}
	if places.err != nil {
		return m, places.err
	}
	procs, ok := scanProcs().(procsMsg)
	if !ok {
		return m, errors.New("could not read the processes")
	}
	if procs.err != nil {
		return m, procs.err
	}
	m.projects, m.groups, m.subs, m.roots = places.projects, places.groups, places.subs, places.roots
	m.host, m.containers = procs.procs, containers()
	m.merge()
	m.terms = map[int]*remoteTerm{}
	held, err := heldShells()
	if err != nil {
		return m, err
	}
	for _, p := range held {
		t := &remoteTerm{pid: p.pid, dir: p.dir, name: p.name, run: p.run}
		m.learnExit(t, p.exit, p.ended)
		m.terms[p.pid] = t
	}
	if agents, ok := scanAgents().(agentsMsg); ok {
		m.agents = agents.agents
	}
	m.groupProcs()
	return m, nil
}

// heldShells is what the server holds, or nothing without a server.
func heldShells() ([]*pane, error) {
	out, err := tmuxCommand("list-panes", "-a", "-F", listFormat)
	if err != nil {
		if errors.Is(err, errNoServer) {
			return nil, nil
		}
		return nil, err
	}
	held, _ := parseListing(out)
	return held, nil
}

// stateWord is a row's mark in a word: what an agent is doing, else how
// the run stands.
func (m model) stateWord(r navRow) string {
	if a := m.agentFor(r); a != nil {
		switch {
		case a.working():
			return "working"
		case m.awaiting(r) != nil:
			if _, blocked := a.blocked(); blocked {
				return "blocked"
			}
			return "waiting"
		}
	}
	switch {
	case m.wrong(r):
		return "failed"
	case m.ended(r) == "0" || containerDone(r):
		return "done"
	}
	return "running"
}
