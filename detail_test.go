package main

import (
	"strings"
	"testing"
)

func TestCountTreeCountsDescendants(t *testing.T) {
	n := &ProcNode{Proc: Proc{PID: 1}, Children: []*ProcNode{
		{Proc: Proc{PID: 2}, Children: []*ProcNode{{Proc: Proc{PID: 3}}}},
		{Proc: Proc{PID: 4}},
	}}
	if got := countTree(n); got != 4 {
		t.Errorf("countTree = %d, want 4", got)
	}
}

func TestPlural(t *testing.T) {
	if got := plural(1, "process", "processes"); got != "1 process" {
		t.Errorf("got %q", got)
	}
	if got := plural(0, "process", "processes"); got != "0 processes" {
		t.Errorf("got %q", got)
	}
}

func TestDetailKeyDistinguishesReposFromProcesses(t *testing.T) {
	repo := detailKey(navRow{kind: rowProject, project: Project{Path: "/p/a"}})
	proc := detailKey(navRow{kind: rowProc, node: &ProcNode{Proc: Proc{PID: 7}}})
	if repo == proc {
		t.Error("a repo and a process should not share a detail key")
	}
}

func TestGitErrorsSayWhatGitSaid(t *testing.T) {
	_, err := git(t.TempDir(), "rev-parse", "HEAD")
	if err == nil {
		t.Fatal("expected an error outside a repository")
	}
	if strings.Contains(err.Error(), "exit status") {
		t.Errorf("error = %q, want git's own message rather than an exit code", err)
	}
	if !strings.Contains(err.Error(), "repository") {
		t.Errorf("error = %q, want it to mention the missing repository", err)
	}
}
