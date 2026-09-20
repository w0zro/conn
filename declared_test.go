package main

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
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

// The projects asked for a file are the blocks that are projects, once
// each, in order, and every project already read for stays asked
// whether or not it still has a block.
func TestTheProjectsAskedAreTheBlocks(t *testing.T) {
	projects := []project{{path: "/r/b"}, {path: "/home"}, {path: "/r/a"}, {path: "/r/b"}}
	isProject := func(p string) bool { return strings.HasPrefix(p, "/r/") }
	got := declaredPaths(projects, nil, isProject)
	if want := []string{"/r/a", "/r/b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("paths: %v, want %v", got, want)
	}
	was := map[string]declared{"/r/c": {}, "/r/a": {}, "/nope": {}}
	got = declaredPaths(projects, was, isProject)
	if want := []string{"/r/a", "/r/b", "/r/c"}; !reflect.DeepEqual(got, want) {
		t.Errorf("paths with the last reading: %v, want %v", got, want)
	}
}

// A project that declares what works it stands even with nothing
// running in it: a block of its own, holding what it declares, down.
func TestAProjectWithNothingUpStandsForItsFile(t *testing.T) {
	files := map[string]declared{"/r/a": {list: []declaration{{name: "web", command: "npm run dev"}}}}
	out := attachDeclared([]project{{path: "/r/b"}}, files, nil)
	i := blockOf(out, "/r/a")
	if i < 0 {
		t.Fatalf("no block for the project that declares: %v", out)
	}
	if len(out[i].entries) != 1 || out[i].entries[0].status != statusDown {
		t.Errorf("the block holds %+v, want one down row", out[i].entries)
	}
	// A file that declares nothing stands for nothing.
	out = attachDeclared([]project{{path: "/r/b"}}, map[string]declared{"/r/c": {}}, nil)
	if blockOf(out, "/r/c") >= 0 {
		t.Errorf("a file declaring nothing made a block: %v", out)
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
	want := "npm run dev # dev\n'/opt/bin/tmux' set-option -p -t \"$TMUX_PANE\" @conn_exit \"$?\"\nprintf '\\n[web exited]\\n'\nexec cat"
	if got != want {
		t.Errorf("line:\n%s\nwant:\n%s", got, want)
	}
}

// The declarations among the rows: one with no pane is a down row at
// the foot of its block; one with a pane marked as its own is that
// pane's head, relabelled, and worded by its end once it has one; a
// pane whose rows are not read yet is no row; a project with a file
// and no block of its own is given one, holding what it declares,
// down; a file that would not read is the block's note.
func TestTheDeclarationsAmongTheRows(t *testing.T) {
	app, lib, zed := "/r/app", "/r/lib", "/r/zed"
	projects := []project{
		{path: app, entries: []entry{
			{pid: 100, kind: kindShell, command: "zsh", typed: "zsh", tty: "ttys001", status: statusIdle},
			{pid: 200, kind: kindShell, command: "sh -c npm run dev", typed: "sh -c npm run dev", tty: "ttys002", status: statusActive},
			{pid: 201, kind: kindRun, command: "npm run dev", typed: "npm run dev", tty: "ttys002", status: statusActive, depth: 1},
			{pid: 300, kind: kindShell, command: "cat", typed: "cat", tty: "ttys003", status: statusActive},
			// By hand: the worker's command typed at the project, in a
			// shell of the operator's own; the api's command, at the
			// project rather than under api; the ghost's, with a word
			// of difference; and a wrong one under nothing.
			{pid: 500, kind: kindShell, command: "zsh", typed: "zsh", tty: "ttys005", status: statusActive, cwd: app},
			{pid: 501, kind: kindRun, command: "make run", typed: "make  run", tty: "ttys005", status: statusWorking, depth: 1, cwd: app},
			{pid: 502, kind: kindRun, command: "go run .", typed: "go run .", tty: "ttys005", status: statusActive, depth: 1, cwd: app},
			{pid: 503, kind: kindRun, command: "sleep 10", typed: "sleep 10", tty: "ttys005", status: statusActive, depth: 1, cwd: app},
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
		" SHELL zsh ACTIVE ttys005 ",
		"  RUN worker · make run WORKING ttys005 " + markDeclared(app, "worker"),
		"  RUN go run . ACTIVE ttys005 ",
		"  RUN sleep 10 ACTIVE ttys005 ",
		zed + " · .conn: line 1: want name [dir]: command",
		" SHELL zsh IDLE ttys004 ",
		lib + " · ",
		" RUN docs · mkdocs serve DOWN  " + markDeclared(lib, "docs"),
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("rows:\n%s\nwant:\n%s", strings.Join(rows, "\n"), strings.Join(want, "\n"))
	}
	// The row started by hand keeps its own pid and place, and the
	// exited one is a fault where a clean end is not.
	if e := got[0].entries[5]; e.pid != 501 || e.depth != 1 || e.cwd != app {
		t.Errorf("the row started by hand: %+v", e)
	}
	if e := got[0].entries[3]; !e.fault {
		t.Error("an exit of 1 is not a fault")
	}
	panes["ttys003"] = pane{id: "%3", tty: "ttys003", declared: markDeclared(app, "api"), exit: "0"}
	if e := attachDeclared(projects, declared, panes)[0].entries[3]; e.status != statusEnded || e.fault {
		t.Errorf("a clean end: %s, fault %v", e.status, e.fault)
	}
	// With nothing started by hand the worker is a down row, with a
	// pid of its own to hold the cursor with.
	got = attachDeclared(projects[:1], declared, panes)
	got[0].entries = got[0].entries[:4]
	if e := attachDeclared([]project{{path: app, entries: projects[0].entries[:4]}}, declared, panes)[0].entries[4]; e.pid != declaredPID(app, "worker") || e.cwd != app || e.status != statusDown {
		t.Errorf("the down row: %+v", e)
	}
	// The projects given are left as they were.
	if len(projects[0].entries) != 8 || projects[0].entries[1].kind != kindShell {
		t.Error("the model's own rows were written to")
	}
	if attachDeclared(projects, nil, panes)[0].path != app || len(attachDeclared(projects, nil, panes)) != 2 {
		t.Error("with nothing declared the projects are not as they were")
	}
}

// What a project has panes for, by mark: the declarations up, to pass
// over, and the panes holding an ended one, to replace; another
// project's are neither, and a down row is neither.
func TestWhatIsUpAndWhatIsHeld(t *testing.T) {
	app, lib := "/r/app", "/r/lib"
	projects := []project{{path: app, entries: []entry{
		{pid: 1, tty: "ttys001", declared: markDeclared(app, "web")},
		{pid: 2, tty: "ttys002", declared: markDeclared(app, "api")},
		{pid: declaredPID(app, "worker"), status: statusDown, declared: markDeclared(app, "worker")},
		{pid: 4, tty: "ttys004", declared: markDeclared(lib, "docs")},
		// Started by hand, somewhere with no terminal conn can see.
		{pid: 5, status: statusActive, declared: markDeclared(app, "cron")},
	}}}
	panes := map[string]pane{
		"ttys001": {id: "%1", tty: "ttys001"},
		"ttys002": {id: "%2", tty: "ttys002", exit: "1"},
		"ttys004": {id: "%4", tty: "ttys004"},
	}
	up, held := upAndHeld(projects, panes, app)
	if !reflect.DeepEqual(up, map[string]bool{markDeclared(app, "web"): true, markDeclared(app, "cron"): true}) {
		t.Errorf("up: %v", up)
	}
	if !reflect.DeepEqual(held, map[string]string{markDeclared(app, "api"): "%2"}) {
		t.Errorf("held: %v", held)
	}
}

// A declared command that runs compose is read for what it hands
// compose — the words before up — and the services it names after it;
// anything else is not a compose up.
func TestAComposeDeclarationIsReadForItsWords(t *testing.T) {
	for _, c := range []struct {
		command    string
		pre, named []string
		ok         bool
	}{
		{"docker compose up", nil, nil, true},
		{"docker compose -f stack.yml -p shop up", []string{"-f", "stack.yml", "-p", "shop"}, nil, true},
		{"docker compose up -d web db", nil, []string{"web", "db"}, true},
		{"docker-compose up api", nil, []string{"api"}, true},
		{"docker compose logs", nil, nil, false},
		{"npm run dev", nil, nil, false},
		{"docker", nil, nil, false},
	} {
		pre, named, ok := composeArgs(c.command)
		if ok != c.ok || !slices.Equal(pre, c.pre) || !slices.Equal(named, c.named) {
			t.Errorf("%q read as pre %q named %q ok %v", c.command, pre, named, ok)
		}
	}
	// Named services are the answer without asking compose; nothing to
	// ask with is nothing.
	if got := composeServices("/nowhere", nil, []string{"web", "db"}); !slices.Equal(got, []string{"web", "db"}) {
		t.Errorf("named services came back as %q", got)
	}
}

// A compose declaration that is down has the services it would bring
// up as rows under it, each down; one that is up has a down row for
// each service that has no container among its rows yet, and none for
// the ones that have.
func TestAComposeDeclarationsServicesAreRows(t *testing.T) {
	shop := "/r/shop"
	stack := declaration{name: "stack", command: "docker compose up"}
	declared := map[string]declared{shop: {
		list:     []declaration{stack},
		services: map[string][]string{"stack": {"api", "db", "web"}},
	}}
	// Down: nothing runs it, and a shell is open in the project.
	projects := []project{{path: shop, entries: []entry{
		{pid: 100, kind: kindShell, command: "zsh", typed: "zsh", tty: "ttys001", status: statusIdle},
	}}}
	got := attachDeclared(projects, declared, nil)
	rows := func(pl project) []string {
		var out []string
		for _, e := range pl.entries {
			out = append(out, strings.Repeat(" ", e.depth)+e.kind+" "+e.command+" "+e.status)
		}
		return out
	}
	want := []string{"SHELL zsh IDLE", "RUN stack · docker compose up DOWN", " SERVICE api DOWN", " SERVICE db DOWN", " SERVICE web DOWN"}
	if !slices.Equal(rows(got[0]), want) {
		t.Errorf("down:\n%s\nwant:\n%s", strings.Join(rows(got[0]), "\n"), strings.Join(want, "\n"))
	}
	// Each service row holds the cursor by a pid of its own, and is
	// nothing to signal, stop or enter.
	if e := got[0].entries[2]; e.pid == 0 || e.pid == got[0].entries[1].pid || e.declared != "" || e.container != "" || e.cwd != shop {
		t.Errorf("a down service row: %+v", e)
	}
	// And the fold keeps them, as rows that want bringing up.
	if kept := fold(got)[0]; len(kept.entries) != 5 {
		t.Errorf("the fold took the down services: %d rows", len(kept.entries))
	}

	// Up: the head runs, and docker has two of the three services under
	// it; the third is down under the head, after the rows it has.
	mark := markDeclared(shop, "stack")
	up := []project{{path: shop, entries: []entry{
		{pid: 200, kind: kindShell, command: "sh -c docker compose up", typed: "sh -c docker compose up", tty: "ttys002", status: statusActive},
		{pid: 201, kind: kindRun, command: "docker compose up", typed: "docker compose up", tty: "ttys002", status: statusActive, depth: 1},
		{pid: -5, kind: kindService, command: "api", typed: "api", ports: []string{"3000"}, status: statusActive, depth: 2, container: "aaa"},
		{pid: -6, kind: kindService, command: "web", typed: "web", ports: []string{"8080"}, status: statusActive, depth: 2, container: "bbb"},
		{pid: 300, kind: kindShell, command: "zsh", typed: "zsh", tty: "ttys003", status: statusIdle},
	}}}
	panes := map[string]pane{"ttys002": {id: "%2", tty: "ttys002", declared: mark}}
	got = attachDeclared(up, declared, panes)
	want = []string{"RUN stack · docker compose up ACTIVE", " RUN docker compose up ACTIVE", "  SERVICE api ACTIVE", "  SERVICE web ACTIVE", " SERVICE db DOWN", "SHELL zsh IDLE"}
	if !slices.Equal(rows(got[0]), want) {
		t.Errorf("up:\n%s\nwant:\n%s", strings.Join(rows(got[0]), "\n"), strings.Join(want, "\n"))
	}
	if len(up[0].entries) != 5 {
		t.Error("the model's own rows were written to")
	}

	// By hand: the same stack typed into a shell, no pane marked. The
	// row that ran the command is the stack, and the missing service
	// is down under it, not under the shell.
	hand := []project{{path: shop, entries: []entry{
		{pid: 300, kind: kindShell, command: "zsh", typed: "zsh", tty: "ttys003", status: statusIdle, cwd: shop},
		{pid: 301, kind: kindRun, command: "docker compose up", typed: "docker compose up", tty: "ttys003", status: statusActive, depth: 1, cwd: shop},
		{pid: -5, kind: kindService, command: "api", typed: "api", ports: []string{"3000"}, status: statusActive, depth: 2, container: "aaa"},
		{pid: -6, kind: kindService, command: "web", typed: "web", ports: []string{"8080"}, status: statusActive, depth: 2, container: "bbb"},
	}}}
	got = attachDeclared(hand, declared, nil)
	want = []string{"SHELL zsh IDLE", " RUN stack · docker compose up ACTIVE", "  SERVICE api ACTIVE", "  SERVICE web ACTIVE", "  SERVICE db DOWN"}
	if !slices.Equal(rows(got[0]), want) {
		t.Errorf("by hand:\n%s\nwant:\n%s", strings.Join(rows(got[0]), "\n"), strings.Join(want, "\n"))
	}
	if e := got[0].entries[1]; e.declared != mark || e.pid != 301 {
		t.Errorf("the row started by hand: %+v", e)
	}
}
