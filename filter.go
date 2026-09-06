package main

import (
	"path/filepath"
	"strings"
)

// What the filter reaches. A query is a name, and anything the navigator
// draws a name for is a thing that can be looked for by it: a place by what
// it is called or where it is, a process by anything its row says. The rule
// for a process is the whole of the row — not the command alone, which is
// the one thing the row most often does not say. A shell the plan calls web
// is a zsh underneath, a session called docs redesign is a claude, and npm
// run dev is a node; typing what you see has to find what you see.

// matchesFilter reports whether a repository answers to what has been typed.
// Where it is is searched as well as its name, so a directory that is only
// in the name of a repository's parent still finds it — the path within the
// root, group/repo, not from /: a query is a name, and the letters of
// /Users/anyone/projects are in the way of every name.
func matchesFilter(p Project, filter string) bool {
	return answers(filter, p.Name) ||
		answers(filter, filepath.Base(filepath.Dir(p.Path))+"/"+filepath.Base(p.Path))
}

// rowAnswers reports whether a row itself answers the filter: a process
// row by what its run says, a place by its name or path.
func (m model) rowAnswers(r navRow, f string) bool {
	if r.kind == rowProc {
		return answers(f, m.runText(r.run))
	}
	return matchesFilter(r.project, f)
}

// procAnswers reports whether something running at place answers the
// filter. The whole tree is asked: a shell running a claude answers for
// claude, whichever of them the row happens to be named after.
func (m model) procAnswers(place, filter string) bool {
	f := strings.ToLower(strings.TrimSpace(filter))
	if f == "" {
		return false
	}
	return len(m.matchingProcs(m.byPlace[place], f)) > 0
}

// matchingProcs prunes process trees to what answers the filter, folded and
// lowered already: a process that answers for itself stays with its whole
// subtree, and a parent whose child answers stays as the trimmed copy that
// leads there. The copies carry the original pids, which is all a step-in or
// a kill reads.
func (m model) matchingProcs(ns []*ProcNode, f string) []*ProcNode {
	var out []*ProcNode
	for _, n := range ns {
		if m.procSays(n, f) {
			out = append(out, n)
			continue
		}
		if kept := m.matchingProcs(n.Children, f); len(kept) > 0 {
			c := *n
			c.Children = kept
			out = append(out, &c)
		}
	}
	return out
}

// procSays reports whether a process answers the filter by anything its row
// could say of it. The row is the run the process heads — a shell and the
// one thing running in it fold into a row that says web beside :8080 — so
// the run is what answers: "web 8080" finds that row, though no process in
// it says both. The bound is for a process table that says a process
// started itself.
func (m model) procSays(n *ProcNode, f string) bool {
	run := []*ProcNode{n}
	for i := 0; len(n.Children) == 1 && i < 1024; i++ {
		n = n.Children[0]
		run = append(run, n)
	}
	return answers(f, m.runText(run))
}

// runText is what a run's row could say of it: every process's text, in
// one line.
func (m model) runText(run []*ProcNode) string {
	parts := make([]string, 0, len(run))
	for _, n := range run {
		parts = append(parts, m.procText(n))
	}
	return strings.Join(parts, " ")
}

// procText is everything a process's row could say of it, in one line: the
// command the table knows it by, what it was run with, the name its plan
// gave it, what it advertises as an agent — the name its user gave it, its
// model — the ports it listens on, and what its run said of itself when it
// ended. The facets are joined rather than tried one at a time so a query
// can span them the way the row does.
//
// It is what the filter reads, not what the row draws: a run folded into a
// row draws one name, and each process in it answers for itself, so the
// folded shell still answers to the plan's name while the row is named for
// what runs in it.
func (m model) procText(n *ProcNode) string {
	parts := []string{n.Command, commandOf(n)}
	if t := m.terms[n.PID]; t != nil {
		parts = append(parts, t.name, t.summary)
	}
	if k, ok := agentKindOf(n); ok {
		parts = append(parts, agentLabel(k, m.agentNameOf(n), m.agentModelOf(n)))
	}
	for _, p := range n.Ports {
		parts = append(parts, ":"+p)
	}
	return strings.Join(parts, " ")
}
