package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fieldValue(fs []field, label string) (string, bool) {
	for _, f := range fs {
		if f.label == label {
			return f.value, true
		}
	}
	return "", false
}

func TestDescribeStatusCountsChanges(t *testing.T) {
	for _, tc := range []struct{ porcelain, want string }{
		{"", "clean"},
		{"?? new.go\n", "1 untracked"},
		{" M edited.go\n", "1 modified"},
		{"M  staged.go\n", "1 staged"},
		{"MM both.go\n", "1 staged, 1 modified"},
		{"M  a.go\n M b.go\n?? c.go\n", "1 staged, 1 modified, 1 untracked"},
	} {
		if got := describeStatus(tc.porcelain); got != tc.want {
			t.Errorf("describeStatus(%q) = %q, want %q", tc.porcelain, got, tc.want)
		}
	}
}

func TestDescribeStateNamesTheCode(t *testing.T) {
	if got := describeState("S+"); !strings.HasPrefix(got, "sleeping") {
		t.Errorf("describeState(\"S+\") = %q, want it to name the state", got)
	}
	if got := describeState("?"); got != "?" {
		t.Errorf("an unknown code should pass through, got %q", got)
	}
}

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

func TestRepoFieldsDescribeARealRepo(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		// Ignore the developer's own git config: commit signing and hooks
		// would otherwise decide whether this test can run.
		cmd.Env = append(cmd.Environ(),
			"GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@e",
			"GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@e",
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	if err := writeFile(dir+"/a.txt", "hi"); err != nil {
		t.Fatal(err)
	}
	run("add", "a.txt")
	run("commit", "-qm", "first commit")
	if err := writeFile(dir+"/dirty.txt", "x"); err != nil {
		t.Fatal(err)
	}

	fs := repoFields(Project{Name: "demo", Path: dir}, 2, nil)

	if v, ok := fieldValue(fs, "branch"); !ok || v != "main" {
		t.Errorf("branch = %q (present=%v), want main", v, ok)
	}
	if v, ok := fieldValue(fs, "status"); !ok || !strings.Contains(v, "untracked") {
		t.Errorf("status = %q, want it to mention the untracked file", v)
	}
	if v, ok := fieldValue(fs, "last commit"); !ok || !strings.Contains(v, "first commit") {
		t.Errorf("last commit = %q, want the subject", v)
	}
	for _, f := range fs {
		if f.label == "last commit" && (len(f.lead) < 7 || f.leadTone != toneAccent) {
			t.Errorf("last commit lead = %q in tone %v, want the hash in the accent", f.lead, f.leadTone)
		}
	}
	if v, ok := fieldValue(fs, "running"); !ok || v != "2 processes" {
		t.Errorf("running = %q, want %q", v, "2 processes")
	}
}

func TestRepoFieldsSurviveANonRepo(t *testing.T) {
	fs := repoFields(Project{Name: "nope", Path: t.TempDir()}, 0, nil)

	if noteOf(fs) == "" {
		t.Error("a non-repo should still report its path")
	}
	if _, ok := fieldValue(fs, "git"); !ok {
		t.Error("a directory git cannot read should say so rather than look clean")
	}
}

func TestProcFieldsDescribeThisProcess(t *testing.T) {
	self := &ProcNode{Proc: Proc{PID: pidOfSelf(), PPID: 1, Command: "test", Dir: "/tmp"}}
	fs := procFields(self, "test", nil, nil)

	// What it is and where it runs head the pane rather than sitting in the
	// list, because they are what the pane is about.
	if got := headingOf(fs); got != procLabel(self) {
		t.Errorf("heading = %q, want %q", got, procLabel(self))
	}
	if got := noteOf(fs); got != "/tmp" {
		t.Errorf("note = %q, want the working directory", got)
	}
	for _, label := range []string{"parent", "argv", "uptime", "cpu", "memory", "state"} {
		if _, ok := fieldValue(fs, label); !ok {
			t.Errorf("procFields is missing %q: %+v", label, fs)
		}
	}
}

func TestDetailKeyDistinguishesReposFromProcesses(t *testing.T) {
	repo := detailKey(navRow{kind: rowProject, project: Project{Path: "/p/a"}})
	proc := detailKey(navRow{kind: rowProc, node: &ProcNode{Proc: Proc{PID: 7}}})
	if repo == proc {
		t.Error("a repo and a process should not share a detail key")
	}
}

func TestRepoFieldsHandleARepoWithNoCommits(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command("git", "-C", dir, "init", "-q", "-b", "main")
	cmd.Env = append(cmd.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	fs := repoFields(Project{Name: "fresh", Path: dir}, 0, nil)

	if v, ok := fieldValue(fs, "branch"); !ok || v != "main" {
		t.Errorf("branch = %q (present=%v), want main; a fresh repo still has one", v, ok)
	}
	if _, ok := fieldValue(fs, "git"); ok {
		t.Error("a fresh repo should not be reported as unreadable")
	}
	if v, ok := fieldValue(fs, "last commit"); !ok || v != "none yet" {
		t.Errorf("last commit = %q, want %q", v, "none yet")
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

// headingOf and noteOf are the lines a pane leads with.
func headingOf(fs []field) string { return firstOfKind(fs, headingField) }
func noteOf(fs []field) string    { return firstOfKind(fs, noteField) }

func firstOfKind(fs []field, kind fieldKind) string {
	for _, f := range fs {
		if f.kind == kind {
			return f.value
		}
	}
	return ""
}

// pairsOf is the labelled lines alone, without the breaks between groups.
func pairsOf(fs []field) []field {
	var out []field
	for _, f := range fs {
		if f.kind == pairField {
			out = append(out, f)
		}
	}
	return out
}

func TestAPaneLeadsWithWhatItIsAbout(t *testing.T) {
	fs := repoFields(Project{Name: "alpha", Path: "/p/alpha"}, 0, nil)
	if got := headingOf(fs); got != "alpha" {
		t.Errorf("heading = %q, want the repository's name", got)
	}
	if got := noteOf(fs); got != "/p/alpha" {
		t.Errorf("note = %q, want its path", got)
	}
	for _, f := range pairsOf(fs) {
		if f.label == "name" || f.label == "path" {
			t.Errorf("%q is still in the list as well as at the top", f.label)
		}
	}
}

func TestEachGroupSetsItsOwnValueColumn(t *testing.T) {
	// One long label should indent its own group and no others, or a single
	// "session id" pushes the whole pane across.
	fields := []field{
		{label: "a", value: "1"},
		gap(),
		{label: "a-very-long-label", value: "2"},
	}
	lines := []string{}
	for _, b := range blocks(fields) {
		lines = append(lines, renderBlock(b, 60)...)
	}

	if got := stripANSI(lines[0]); got != "  a  1" {
		t.Errorf("first group = %q, want it tight to its own widest label", got)
	}
	if got := stripANSI(lines[1]); got != "  a-very-long-label  2" {
		t.Errorf("second group = %q, want its own column", got)
	}
}

func TestAGroupWithNothingInItDrawsNothing(t *testing.T) {
	// A pane that skipped a whole group should not leave a hole where it
	// would have been.
	fields := []field{{label: "a", value: "1"}, gap(), gap(), {label: "b", value: "2"}}

	var lines []string
	for _, b := range blocks(fields) {
		if drawn := renderBlock(b, 60); len(drawn) > 0 {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, drawn...)
		}
	}
	if len(lines) != 3 {
		t.Errorf("lines = %q, want two groups and one break between them", lines)
	}
}

func TestTheRunsPortsAreTheRowsPorts(t *testing.T) {
	// A dev server is a shell running an npm running a node, and it is the
	// node at the bottom that holds the port — the one the fold exists to
	// hide. Asking only the process the row is named for found nothing.
	named := &ProcNode{Proc: Proc{PID: 10, Command: "npm", Dir: "/tmp"}}
	listener := &ProcNode{Proc: Proc{PID: 11, Command: "node", Dir: "/tmp", Ports: []string{"8932"}}}
	run := []*ProcNode{named, listener}

	got, ok := fieldValue(procFields(named, named.Command, run, nil), "listening")
	if !ok {
		t.Fatalf("nothing reported, want the port the run is listening on")
	}
	if got != "8932" {
		t.Errorf("listening = %q, want the port held further down the run", got)
	}

	// And with no run, the row still speaks for itself.
	if _, ok := fieldValue(procFields(listener, listener.Command, nil, nil), "listening"); !ok {
		t.Error("a row that folded nothing should still report its own port")
	}

	// A port two processes of the run hold — a server and its worker — is
	// one port, and the run's ports read lowest first whichever held them.
	twice := []*ProcNode{
		{Proc: Proc{PID: 12, Ports: []string{"8080", "443"}}},
		{Proc: Proc{PID: 13, Ports: []string{"8080", "80"}}},
	}
	if got := strings.Join(runPorts(twice, twice[0]), ","); got != "80,443,8080" {
		t.Errorf("ports = %q, want each once, by number", got)
	}
}

func TestTonesFollowTheFacts(t *testing.T) {
	// A value carries its state in color: alive is green, wrong is red, and
	// what is merely true recedes.
	if got := runningField(2).tone; got != toneGood {
		t.Errorf("2 running tone = %v, want good", got)
	}
	if got := runningField(0).tone; got != toneQuiet {
		t.Errorf("0 running tone = %v, want quiet", got)
	}
	if got := stateTone("R+"); got != toneGood {
		t.Errorf("running state tone = %v, want good", got)
	}
	if got := stateTone("Z"); got != toneBad {
		t.Errorf("zombie state tone = %v, want bad", got)
	}
	if got := stateTone("Ss"); got != toneName {
		t.Errorf("sleeping state tone = %v, want the name of a state", got)
	}
}

func TestARowInAHeldShellCarriesTheShellsTranscript(t *testing.T) {
	// What the process is doing, read without entering it: the last of
	// what the shell has shown, under its own heading, after the facts.
	// A row with no held shell around it has no transcript to carry.
	r := navRow{kind: rowProc, project: Project{Name: "tmp", Path: "/tmp"},
		node: &ProcNode{Proc: Proc{PID: 10, Command: "npm", Dir: "/tmp"}}}
	msg := loadDetail(r, r.node.Command, 0, 0, nil, nil, func() []string { return []string{"$ npm run dev", "ready on :5173"} }, entryState{})().(detailMsg)
	var got []string
	seen := false
	for _, f := range msg.fields {
		if f.kind == headingField && f.value == "transcript" {
			seen = true
			continue
		}
		if f.kind == textField {
			got = append(got, f.value)
		}
	}
	if !seen || strings.Join(got, "|") != "$ npm run dev|ready on :5173" {
		t.Errorf("fields = %+v, want the transcript under its heading", msg.fields)
	}

	msg = loadDetail(r, r.node.Command, 0, 0, nil, nil, nil, entryState{})().(detailMsg)
	for _, f := range msg.fields {
		if f.kind == textField || f.value == "transcript" {
			t.Errorf("a row with no held shell carries a transcript: %+v", f)
		}
	}
	// And a shell that has shown nothing yet is no block at all.
	if fs := transcript(nil); fs != nil {
		t.Errorf("transcript(nil) = %+v, want nothing", fs)
	}
}

func TestTheChecklistSaysHowEachEntryStands(t *testing.T) {
	// Up glows, down sits hollow, and an entry whose command ended badly
	// shows the cross, red through, with how it ended.
	dir := t.TempDir()
	if err := writeFile(filepath.Join(dir, ".conn"), "web: npm run dev\napi: go run .\njob: make\nold: sleep 1\n"); err != nil {
		t.Fatal(err)
	}
	fs := planFields(dir, map[string]entryState{"web": {State: "up"}, "api": {State: "1", At: time.Now().Add(-3 * time.Minute)}, "old": {State: "0"}})
	byName := map[string]field{}
	for _, f := range fs {
		if f.lead != "" {
			byName[f.lead[len(f.lead)-3:]] = f
		}
	}
	if f := byName["web"]; !strings.HasPrefix(f.lead, glyphOn) || f.leadTone != toneGood || f.value != "npm run dev" {
		t.Errorf("web = %+v, want up, lit, its command in ink", f)
	}
	if f := byName["api"]; !strings.HasPrefix(f.lead, glyphFailed) || f.leadTone != toneBad || f.tone != toneBad || !strings.HasSuffix(f.value, "exited 1, 3m ago") {
		t.Errorf("api = %+v, want the cross, red through, with how and when it ended", f)
	}
	if f := byName["job"]; !strings.HasPrefix(f.lead, glyphOff) || f.leadTone != toneQuiet || f.value != "make" {
		t.Errorf("job = %+v, want down and hollow", f)
	}
	if f := byName["old"]; !strings.HasPrefix(f.lead, glyphDone) || f.leadTone != toneGood || !strings.HasSuffix(f.value, "exited 0") {
		t.Errorf("old = %+v, want the check, ended well", f)
	}
}

func TestARowAtItsPromptAfterItsCommandSaysHowItEnded(t *testing.T) {
	r := navRow{kind: rowProc, project: Project{Name: "tmp", Path: "/tmp"},
		node: &ProcNode{Proc: Proc{PID: 10, Command: "zsh", Dir: "/tmp"}}}
	msg := loadDetail(r, r.node.Command, 0, 0, nil, nil, nil, entryState{State: "1", At: time.Now().Add(-90 * time.Second)})().(detailMsg)
	var got field
	for _, f := range msg.fields {
		if f.label == "exited" {
			got = f
		}
	}
	if got.lead != "1" || got.leadTone != toneBad || got.value != "1m ago" {
		t.Errorf("exited = %+v, want the status in red and how long ago", got)
	}
	if exitField(entryState{State: "0"}).leadTone != toneGood || exitField(entryState{State: "0"}).value != "" {
		t.Error("ended well should read green, and a moment unknown say nothing")
	}
}

func TestThePaneSaysHowThePlacesTestsRunAndWent(t *testing.T) {
	dir := t.TempDir()
	if err := writeFile(filepath.Join(dir, "Cargo.toml"), ""); err != nil {
		t.Fatal(err)
	}
	get := func(states map[string]entryState) field {
		for _, f := range verbFields(dir, states) {
			if f.label == "tests" {
				return f
			}
		}
		t.Fatal("no tests line")
		return field{}
	}
	if f := get(nil); f.value != "cargo test" || !strings.HasSuffix(f.lead, "not run") || f.leadTone != toneQuiet {
		t.Errorf("never run = %+v, want the command and not run, quiet", f)
	}
	if f := get(map[string]entryState{testName: {State: "up"}}); !strings.HasSuffix(f.lead, "running") || f.leadTone != toneGood {
		t.Errorf("running = %+v", f)
	}
	if f := get(map[string]entryState{testName: {State: "0", At: time.Now().Add(-2 * time.Hour)}}); !strings.HasPrefix(f.lead, glyphDone) || !strings.HasSuffix(f.lead, "passed  2h ago") || f.leadTone != toneGood {
		t.Errorf("passed = %+v, want the check and how long ago", f)
	}
	if f := get(map[string]entryState{testName: {State: "2"}}); !strings.HasPrefix(f.lead, glyphFailed) || !strings.HasSuffix(f.lead, "exit 2") || f.leadTone != toneBad {
		t.Errorf("failed = %+v", f)
	}
	if fs := verbFields(t.TempDir(), nil); fs != nil {
		t.Errorf("a place that says nothing of its tasks has a line: %+v", fs)
	}
	// Every task the place says how to run has a line, in the verbs'
	// order: a cargo project tests, builds and lints.
	var labels []string
	for _, f := range verbFields(dir, map[string]entryState{"build": {State: "0"}}) {
		if f.kind == pairField {
			labels = append(labels, f.label+":"+f.lead)
		}
	}
	if got := strings.Join(labels, " "); got != "tests:○ not run build:✓ built lint:○ not linted" {
		t.Errorf("lines = %q, want each task's line in order", got)
	}
}

func TestWhatTheRunSaidStandsInForTheWord(t *testing.T) {
	// 3 failed rather than failed, 12 passed rather than passed, on the
	// tasks line; and on the exited line and the checklist beside how
	// long ago.
	dir := t.TempDir()
	if err := writeFile(filepath.Join(dir, "go.mod"), "module x\n"); err != nil {
		t.Fatal(err)
	}
	var tests field
	for _, f := range verbFields(dir, map[string]entryState{testName: {State: "1", Summary: "3 failed"}}) {
		if f.label == "tests" {
			tests = f
		}
	}
	if !strings.HasSuffix(tests.lead, "3 failed") || strings.Contains(tests.lead, "exit") {
		t.Errorf("tests = %+v, want the run's word in place of the status", tests)
	}
	f := exitField(entryState{State: "1", Summary: "3 failed", At: time.Now().Add(-time.Minute)})
	if f.value != "3 failed, 1m ago" {
		t.Errorf("exited = %+v, want the word and the moment", f)
	}
	if err := writeFile(filepath.Join(dir, ".conn"), "web: npm run dev\n"); err != nil {
		t.Fatal(err)
	}
	for _, f := range planFields(dir, map[string]entryState{"web": {State: "1", Summary: "2 errors"}}) {
		if f.lead != "" && !strings.HasSuffix(f.value, "exited 1, 2 errors") {
			t.Errorf("web = %+v, want how it ended and what it said", f)
		}
	}
}

func TestProcFieldsSayTheGroupAndAnOrphan(t *testing.T) {
	n := &ProcNode{Proc: Proc{PID: pidOfSelf(), PPID: 1, PGID: 77, Command: "test", Dir: "/tmp"}}
	got, _ := fieldValue(procFields(n, "test", nil, nil), "parent")
	if got != "1 · group 77 · orphaned" {
		t.Errorf("parent = %q, want the group said and the orphan noted", got)
	}
	n.PPID, n.PGID = 500, 500
	if got, _ = fieldValue(procFields(n, "test", nil, nil), "parent"); got != "500 · group 500" {
		t.Errorf("parent = %q, want the group alone", got)
	}
}

func TestTheExitedLineAndTheChecklistDecodeASignal(t *testing.T) {
	// 137 is a kill, and the pane says so beside the number: a crash and
	// a kill read the same otherwise.
	f := exitField(entryState{State: "137", At: time.Now().Add(-time.Minute)})
	if f.lead != "137" || f.value != "killed, 1m ago" {
		t.Errorf("exited = %+v, want the word beside the number", f)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".conn"), []byte("api: go run .\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := planFields(dir, map[string]entryState{"api": {State: "137"}})
	var got string
	for _, f := range fs {
		if strings.HasSuffix(f.lead, "api") {
			got = f.value
		}
	}
	if got != "go run .   exited 137, killed" {
		t.Errorf("api = %q, want the kill decoded", got)
	}
}
