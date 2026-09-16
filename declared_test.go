package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// The file is one process a line, name [dir]: command; comments and
// blank lines are nothing; the dir is cleaned and the project itself
// is no dir at all.
func TestTheFileIsOneProcessALine(t *testing.T) {
	got, err := parseDeclared("# the processes\n\nweb frontend: npm run dev\napi: go run ./cmd/api\nworker services/queue/: make run\nhere .: ls  \n")
	if err != nil {
		t.Fatal(err)
	}
	want := []declaration{
		{name: "web", dir: "frontend", command: "npm run dev", line: 3},
		{name: "api", command: "go run ./cmd/api", line: 4},
		{name: "worker", dir: "services/queue", command: "make run", line: 5},
		{name: "here", command: "ls", line: 6},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parsed:\n%+v\nwant:\n%+v", got, want)
	}
	if got, err := parseDeclared(""); err != nil || len(got) != 0 {
		t.Errorf("an empty file declares %v, %v", got, err)
	}
}

// A line that will not parse is said with its number, and nothing is
// guessed at in its place.
func TestALineThatWillNotParseIsSaid(t *testing.T) {
	for _, c := range []struct{ text, want string }{
		{"web npm run dev", "line 1: want name [dir]: command"},
		{"\nweb front end: npm", "line 2: want name [dir]: command"},
		{": npm", "line 1: want name [dir]: command"},
		{"w@b: npm", `line 1: "w@b" is not a name`},
		{"web /tmp: npm", "line 1: /tmp is not under the project"},
		{"web ../x: npm", "line 1: ../x is not under the project"},
		{"web:   ", "line 1: web runs nothing"},
		{"web: a\nweb: b", "line 2: web is declared twice"},
	} {
		_, err := parseDeclared(c.text)
		if err == nil || err.Error() != c.want {
			t.Errorf("parseDeclared(%q) = %v, want %s", c.text, err, c.want)
		}
	}
}

// Read from a project, no file is nothing and no error; a dir a line
// names has to be there; and the cache reads again only what changed,
// or what would not read.
func TestTheFilesAreReadOnceAndAgainWhenChanged(t *testing.T) {
	project := t.TempDir()
	other := t.TempDir()
	if list, err := readDeclared(project); err != nil || list != nil {
		t.Errorf("no file: %v, %v", list, err)
	}
	if err := os.MkdirAll(filepath.Join(project, "frontend"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(project, declaredName)
	write := func(text string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("web frontend: npm run dev\napi: go run .\n")
	got := refreshDeclared(nil, []string{project, other})
	if _, ok := got[other]; ok {
		t.Error("a project with no file is in the answer")
	}
	d := got[project]
	if d.err != "" || len(d.list) != 2 || d.list[0].name != "web" {
		t.Fatalf("read: %+v", d)
	}
	// Unchanged, the cached reading stands: a mark left on it survives.
	got[project].list[0].command = "kept"
	again := refreshDeclared(got, []string{project})
	if again[project].list[0].command != "kept" {
		t.Error("an unchanged file was read again")
	}
	// Changed, it is read again; the stamp has to move, so the time is
	// set back rather than waited for.
	write("web frontend: npm start\n")
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	again = refreshDeclared(again, []string{project})
	if l := again[project].list; len(l) != 1 || l[0].command != "npm start" {
		t.Errorf("a changed file was not read again: %+v", l)
	}
	// A dir that is not there is the file's error, and an erring file
	// is read again every time, since the fix may be beside it.
	write("web frontend: npm start\nworker queue: make run\n")
	older := old.Add(-time.Hour)
	if err := os.Chtimes(path, older, older); err != nil {
		t.Fatal(err)
	}
	again = refreshDeclared(again, []string{project})
	if want := ".conn: line 2: queue is not a directory"; again[project].err != want {
		t.Errorf("a missing dir: %q, want %q", again[project].err, want)
	}
	if err := os.MkdirAll(filepath.Join(project, "queue"), 0o755); err != nil {
		t.Fatal(err)
	}
	again = refreshDeclared(again, []string{project})
	if again[project].err != "" || len(again[project].list) != 2 {
		t.Errorf("the dir made, the file still errs: %+v", again[project])
	}
}

// The projects asked for a file are the blocks that are projects and
// what the walk found, once each, in order.
func TestTheProjectsAskedAreTheBlocksAndTheWalk(t *testing.T) {
	projects := []project{{path: "/r/b"}, {path: "/home"}, {path: "/r/a"}}
	walked := []projectRow{{path: "/r/a"}, {path: "/r/c"}, {path: "/r/c", pid: 12}, {path: ""}}
	isProject := func(p string) bool { return strings.HasPrefix(p, "/r/") }
	got := declaredPaths(projects, walked, isProject)
	if want := []string{"/r/a", "/r/b", "/r/c"}; !reflect.DeepEqual(got, want) {
		t.Errorf("paths: %v, want %v", got, want)
	}
}

// A mark carries the name and the project with no space in it, and
// reads back; the pid is below every container's and the same every
// time, and differs across projects for one name.
func TestTheMarkAndThePid(t *testing.T) {
	mark := markDeclared("/Users/w0 zro/app", "web")
	if strings.Contains(mark, " ") {
		t.Errorf("the mark has a space: %q", mark)
	}
	if project, name, ok := unmarkDeclared(mark); !ok || project != "/Users/w0 zro/app" || name != "web" {
		t.Errorf("the mark reads back as %q %q %v", project, name, ok)
	}
	if _, _, ok := unmarkDeclared("nomark"); ok {
		t.Error("a string with no seam is a mark")
	}
	a, b := declaredPID("/a", "web"), declaredPID("/b", "web")
	if a >= -16777217 || b >= -16777217 || a == b || a != declaredPID("/a", "web") {
		t.Errorf("pids: %d %d", a, b)
	}
}

// The line run in the pane is the command as written, then the exit
// recorded and the hold, each on a line of its own.
func TestTheLineRunInThePane(t *testing.T) {
	got := declaredLine(declaration{name: "web", command: "npm run dev # dev"}, "/opt/bin/tmux")
	want := "npm run dev # dev\n'/opt/bin/tmux' set-option -p @conn_exit \"$?\"\nprintf '\\n[web exited]\\n'\nexec cat"
	if got != want {
		t.Errorf("line:\n%s\nwant:\n%s", got, want)
	}
}

// The declarations among the rows: one with no pane is a down row at
// the foot of its block; one with a pane marked as its own is that
// pane's head, relabelled, and worded by its end once it has one; a
// pane whose rows are not read yet is no row; a project with a file
// and no work gets a block in its place by path; a file that would not
// read is the block's note.
func TestTheDeclarationsAmongTheRows(t *testing.T) {
	app, lib, zed := "/r/app", "/r/lib", "/r/zed"
	projects := []project{
		{path: app, entries: []entry{
			{pid: 100, kind: kindShell, command: "zsh", typed: "zsh", tty: "ttys001", status: statusIdle},
			{pid: 200, kind: kindShell, command: "sh -c npm run dev", typed: "sh -c npm run dev", tty: "ttys002", status: statusActive},
			{pid: 201, kind: kindRun, command: "npm run dev", typed: "npm run dev", tty: "ttys002", status: statusActive, depth: 1},
			{pid: 300, kind: kindShell, command: "cat", typed: "cat", tty: "ttys003", status: statusActive},
		}},
		{path: zed, entries: []entry{{pid: 400, kind: kindShell, command: "zsh", tty: "ttys004", status: statusIdle}}},
	}
	declared := map[string]declared{
		app: {list: []declaration{
			{name: "web", command: "npm run dev"},
			{name: "api", command: "go run .", dir: "api"},
			{name: "worker", command: "make run"},
			{name: "ghost", command: "sleep 1"},
		}},
		lib: {list: []declaration{{name: "docs", command: "mkdocs serve"}}},
		zed: {err: ".conn: line 1: want name [dir]: command"},
	}
	panes := map[string]pane{
		"ttys002": {id: "%2", tty: "ttys002", declared: markDeclared(app, "web")},
		"ttys003": {id: "%3", tty: "ttys003", declared: markDeclared(app, "api"), exit: "1"},
		"ttys009": {id: "%9", tty: "ttys009", declared: markDeclared(app, "ghost")},
	}
	got := attachDeclared(projects, declared, panes)
	var rows []string
	for _, pl := range got {
		rows = append(rows, pl.path+" · "+pl.note)
		for _, e := range pl.entries {
			rows = append(rows, strings.Repeat(" ", e.depth+1)+e.kind+" "+e.command+" "+e.status+" "+e.tty+" "+e.declared)
		}
	}
	want := []string{
		app + " · ",
		" SHELL zsh IDLE ttys001 ",
		" RUN web · npm run dev ACTIVE ttys002 " + markDeclared(app, "web"),
		"  RUN npm run dev ACTIVE ttys002 ",
		" RUN api · go run . EXIT 1 ttys003 " + markDeclared(app, "api"),
		" RUN worker · make run DOWN  " + markDeclared(app, "worker"),
		lib + " · ",
		" RUN docs · mkdocs serve DOWN  " + markDeclared(lib, "docs"),
		zed + " · .conn: line 1: want name [dir]: command",
		" SHELL zsh IDLE ttys004 ",
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("rows:\n%s\nwant:\n%s", strings.Join(rows, "\n"), strings.Join(want, "\n"))
	}
	// The down row has a pid of its own to hold the cursor with, and
	// the exited one is a fault where a clean end is not.
	if e := got[0].entries[4]; e.pid != declaredPID(app, "worker") || e.cwd != app {
		t.Errorf("the down row: %+v", e)
	}
	if e := got[0].entries[3]; !e.fault {
		t.Error("an exit of 1 is not a fault")
	}
	panes["ttys003"] = pane{id: "%3", tty: "ttys003", declared: markDeclared(app, "api"), exit: "0"}
	if e := attachDeclared(projects, declared, panes)[0].entries[3]; e.status != statusEnded || e.fault {
		t.Errorf("a clean end: %s, fault %v", e.status, e.fault)
	}
	// The projects given are left as they were.
	if len(projects[0].entries) != 4 || projects[0].entries[1].kind != kindShell {
		t.Error("the model's own rows were written to")
	}
	if attachDeclared(projects, nil, panes)[0].path != app || len(attachDeclared(projects, nil, panes)) != 2 {
		t.Error("with nothing declared the projects are not as they were")
	}
}
