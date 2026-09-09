package main

import (
	"os"
	"path/filepath"
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

func TestTheLookPageDrawsFieldsInBlocks(t *testing.T) {
	// A heading, its note, and pairs whose values line up on the widest
	// label of their block; a lead stands ahead of its value; a gap
	// starts a new block with a column of its own.
	fs := []field{
		heading("api 4402"), note("/p/demo"), gap(),
		{Label: "branch", Value: "main", Tone: toneAccent},
		{Label: "last commit", Lead: "ab12cd", LeadTone: toneAccent, Value: "the subject"},
		gap(),
		{Label: "cpu", Value: "3%"},
	}
	var lines []string
	for _, b := range blocks(fs) {
		lines = append(lines, renderBlock(b, 60)...)
	}
	plain := make([]string, len(lines))
	for i, l := range lines {
		plain[i] = stripANSI(l)
	}
	if len(plain) != 5 || plain[0] != gutter+"api 4402" || plain[1] != gutter+"/p/demo" {
		t.Fatalf("lines = %q, want the heading and the note first", plain)
	}
	if !strings.HasPrefix(plain[2], gutter+"branch     "+gutter+"main") || !strings.HasPrefix(plain[3], gutter+"last commit"+gutter+"ab12cd  the subject") {
		t.Errorf("block = %q, want the values aligned on the widest label and the lead ahead of the value", plain[2:4])
	}
	if plain[4] != gutter+"cpu"+gutter+"3%" {
		t.Errorf("last block = %q, want its own column", plain[4])
	}
}

func TestRepoFieldsReadTheCheckout(t *testing.T) {
	repo := t.TempDir()
	sh := func(args ...string) {
		t.Helper()
		if _, err := git(repo, args...); err != nil {
			t.Fatal(err)
		}
	}
	sh("init", "-q", "-b", "main")
	sh("-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "the first commit")
	if err := os.WriteFile(filepath.Join(repo, ".conn"), []byte("api: go run ./api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := repoFields(Project{Name: "demo", Path: repo}, 2, map[string]entryState{"api": {State: "up", Ports: []string{"8080"}}})
	got := map[string]field{}
	for _, f := range fs {
		if f.Label != "" {
			got[f.Label] = f
		}
	}
	if got["branch"].Value != "main" || got["status"].Value != "1 untracked" {
		t.Errorf("branch %q, status %q; want main and the untracked plan counted", got["branch"].Value, got["status"].Value)
	}
	if got["last commit"].Value != "the first commit" || got["last commit"].Lead == "" {
		t.Errorf("last commit = %+v, want the subject after its hash", got["last commit"])
	}
	if got["running"].Value != "2 processes" {
		t.Errorf("running = %q", got["running"].Value)
	}
	if !strings.Contains(got["needs"].Lead, "api") || !strings.Contains(got["needs"].Value, ":8080") {
		t.Errorf("needs = %+v, want the entry up with its port", got["needs"])
	}
}

func TestILooksAtTheCursorsRowOrTheShownBuffers(t *testing.T) {
	m := heldTabs(80)
	m.all, m.shown = true, 0
	for i, r := range m.rows {
		if r.kind == rowProc && r.holds(702) {
			m.cursor = i
		}
	}
	if r, ok := m.lookRow(); !ok || !r.holds(702) {
		t.Errorf("look row = %+v, %v with the cursor on web, want web", r, ok)
	}
	m.all, m.shown, m.cursor = false, 701, 0
	if r, ok := m.lookRow(); !ok || !r.holds(701) {
		t.Errorf("look row = %+v, %v with 701 shown, want the shown buffer's row", r, ok)
	}
}
