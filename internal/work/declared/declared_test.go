package declared

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/w0zro/conn/internal/work"
)

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

// A project that declares what works it stands even with nothing
// running in it: a block of its own, holding what it declares, down.
func TestAProjectWithNothingUpStandsForItsFile(t *testing.T) {
	files := map[string]File{"/r/a": {List: []Declaration{{Name: "web", Command: "npm run dev"}}}}
	out := Attach([]work.Project{{Path: "/r/b"}}, files, nil)
	i := work.BlockOf(out, "/r/a")
	if i < 0 {
		t.Fatalf("no block for the project that declares: %v", out)
	}
	if len(out[i].Entries) != 1 || out[i].Entries[0].Status != work.StatusDown {
		t.Errorf("the block holds %+v, want one down row", out[i].Entries)
	}
	// A file that declares nothing stands for nothing.
	out = Attach([]work.Project{{Path: "/r/b"}}, map[string]File{"/r/c": {}}, nil)
	if work.BlockOf(out, "/r/c") >= 0 {
		t.Errorf("a file declaring nothing made a block: %v", out)
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
	projects := []work.Project{
		{Path: app, Entries: []work.Entry{
			{PID: 100, Kind: work.KindShell, Command: "zsh", Typed: "zsh", TTY: "ttys001", Status: work.StatusIdle},
			{PID: 200, Kind: work.KindShell, Command: "sh -c npm run dev", Typed: "sh -c npm run dev", TTY: "ttys002", Status: work.StatusActive},
			{PID: 201, Kind: work.KindRun, Command: "npm run dev", Typed: "npm run dev", TTY: "ttys002", Status: work.StatusActive, Depth: 1},
			{PID: 300, Kind: work.KindShell, Command: "cat", Typed: "cat", TTY: "ttys003", Status: work.StatusActive},
			// By hand: the worker's command typed at the project, in a
			// shell of the operator's own; the api's command, at the
			// project rather than under api; the ghost's, with a word
			// of difference; and a wrong one under nothing.
			{PID: 500, Kind: work.KindShell, Command: "zsh", Typed: "zsh", TTY: "ttys005", Status: work.StatusActive, Cwd: app},
			{PID: 501, Kind: work.KindRun, Command: "make run", Typed: "make  run", TTY: "ttys005", Status: work.StatusWorking, Depth: 1, Cwd: app},
			{PID: 502, Kind: work.KindRun, Command: "go run .", Typed: "go run .", TTY: "ttys005", Status: work.StatusActive, Depth: 1, Cwd: app},
			{PID: 503, Kind: work.KindRun, Command: "sleep 10", Typed: "sleep 10", TTY: "ttys005", Status: work.StatusActive, Depth: 1, Cwd: app},
		}},
		{Path: zed, Entries: []work.Entry{{PID: 400, Kind: work.KindShell, Command: "zsh", TTY: "ttys004", Status: work.StatusIdle}}},
	}
	declared := map[string]File{
		app: {List: []Declaration{
			{Name: "web", Command: "npm run dev"},
			{Name: "api", Command: "go run .", Dir: "api"},
			{Name: "worker", Command: "make run"},
			{Name: "ghost", Command: "sleep 1"},
		}},
		lib: {List: []Declaration{{Name: "docs", Command: "mkdocs serve"}}},
		zed: {Err: ".conn: line 1: want name [dir]: command"},
	}
	panes := map[string]Pane{
		"ttys002": {ID: "%2", TTY: "ttys002", Declared: Mark(app, "web")},
		"ttys003": {ID: "%3", TTY: "ttys003", Declared: Mark(app, "api"), Exit: "1"},
		"ttys009": {ID: "%9", TTY: "ttys009", Declared: Mark(app, "ghost")},
	}
	got := Attach(projects, declared, panes)
	var rows []string
	for _, pl := range got {
		rows = append(rows, pl.Path+" · "+pl.Note)
		for _, e := range pl.Entries {
			rows = append(rows, strings.Repeat(" ", e.Depth+1)+e.Kind+" "+e.Command+" "+e.Status+" "+e.TTY+" "+e.Declared)
		}
	}
	want := []string{
		app + " · ",
		" SHELL zsh IDLE ttys001 ",
		" RUN web · npm run dev ACTIVE ttys002 " + Mark(app, "web"),
		"  RUN npm run dev ACTIVE ttys002 ",
		" RUN api · go run . EXIT 1 ttys003 " + Mark(app, "api"),
		" SHELL zsh ACTIVE ttys005 ",
		"  RUN worker · make run WORKING ttys005 " + Mark(app, "worker"),
		"  RUN go run . ACTIVE ttys005 ",
		"  RUN sleep 10 ACTIVE ttys005 ",
		zed + " · .conn: line 1: want name [dir]: command",
		" SHELL zsh IDLE ttys004 ",
		lib + " · ",
		" RUN docs · mkdocs serve DOWN  " + Mark(lib, "docs"),
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("rows:\n%s\nwant:\n%s", strings.Join(rows, "\n"), strings.Join(want, "\n"))
	}
	// The row started by hand keeps its own pid and place, and the
	// exited one is a fault where a clean end is not.
	if e := got[0].Entries[5]; e.PID != 501 || e.Depth != 1 || e.Cwd != app {
		t.Errorf("the row started by hand: %+v", e)
	}
	if e := got[0].Entries[3]; !e.Fault {
		t.Error("an exit of 1 is not a fault")
	}
	panes["ttys003"] = Pane{ID: "%3", TTY: "ttys003", Declared: Mark(app, "api"), Exit: "0"}
	if e := Attach(projects, declared, panes)[0].Entries[3]; e.Status != work.StatusEnded || e.Fault {
		t.Errorf("a clean end: %s, fault %v", e.Status, e.Fault)
	}
	// With nothing started by hand the worker is a down row, with a
	// pid of its own to hold the cursor with.
	got = Attach(projects[:1], declared, panes)
	got[0].Entries = got[0].Entries[:4]
	if e := Attach([]work.Project{{Path: app, Entries: projects[0].Entries[:4]}}, declared, panes)[0].Entries[4]; e.PID != PID(app, "worker") || e.Cwd != app || e.Status != work.StatusDown {
		t.Errorf("the down row: %+v", e)
	}
	// The projects given are left as they were.
	if len(projects[0].Entries) != 8 || projects[0].Entries[1].Kind != work.KindShell {
		t.Error("the model's own rows were written to")
	}
	if Attach(projects, nil, panes)[0].Path != app || len(Attach(projects, nil, panes)) != 2 {
		t.Error("with nothing declared the projects are not as they were")
	}
}

// The file is one process a line, name [dir]: command; comments and
// blank lines are nothing; the dir is cleaned and the project itself
// is no dir at all.
func TestTheFileIsOneProcessALine(t *testing.T) {
	got, err := parseDeclared("# the processes\n\nweb frontend: npm run dev\napi: go run ./cmd/api\nworker services/queue/: make run\nhere .: ls  \n")
	if err != nil {
		t.Fatal(err)
	}
	want := []Declaration{
		{Name: "web", Dir: "frontend", Command: "npm run dev", Line: 3},
		{Name: "api", Command: "go run ./cmd/api", Line: 4},
		{Name: "worker", Dir: "services/queue", Command: "make run", Line: 5},
		{Name: "here", Command: "ls", Line: 6},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parsed:\n%+v\nwant:\n%+v", got, want)
	}
	if got, err := parseDeclared(""); err != nil || len(got) != 0 {
		t.Errorf("an empty file declares %v, %v", got, err)
	}
}

// Read from a project, no file is nothing and no error; a dir a line
// names has to be there; and the cache reads again only what changed,
// or what would not read.
func TestTheFilesAreReadOnceAndAgainWhenChanged(t *testing.T) {
	project := t.TempDir()
	other := t.TempDir()
	if list, err := Read(project); err != nil || list != nil {
		t.Errorf("no file: %v, %v", list, err)
	}
	if err := os.MkdirAll(filepath.Join(project, "frontend"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(project, FileName)
	write := func(text string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("web frontend: npm run dev\napi: go run .\n")
	got := Refresh(nil, []string{project, other})
	if _, ok := got[other]; ok {
		t.Error("a project with no file is in the answer")
	}
	d := got[project]
	if d.Err != "" || len(d.List) != 2 || d.List[0].Name != "web" {
		t.Fatalf("read: %+v", d)
	}
	// Unchanged, the cached reading stands: a mark left on it survives.
	got[project].List[0].Command = "kept"
	again := Refresh(got, []string{project})
	if again[project].List[0].Command != "kept" {
		t.Error("an unchanged file was read again")
	}
	// Changed, it is read again; the stamp has to move, so the time is
	// set back rather than waited for.
	write("web frontend: npm start\n")
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	again = Refresh(again, []string{project})
	if l := again[project].List; len(l) != 1 || l[0].Command != "npm start" {
		t.Errorf("a changed file was not read again: %+v", l)
	}
	// A dir that is not there is the file's error, and an erring file
	// is read again every time, since the fix may be beside it.
	write("web frontend: npm start\nworker queue: make run\n")
	older := old.Add(-time.Hour)
	if err := os.Chtimes(path, older, older); err != nil {
		t.Fatal(err)
	}
	again = Refresh(again, []string{project})
	if want := ".conn: line 2: queue is not a directory"; again[project].Err != want {
		t.Errorf("a missing dir: %q, want %q", again[project].Err, want)
	}
	if err := os.MkdirAll(filepath.Join(project, "queue"), 0o755); err != nil {
		t.Fatal(err)
	}
	again = Refresh(again, []string{project})
	if again[project].Err != "" || len(again[project].List) != 2 {
		t.Errorf("the dir made, the file still errs: %+v", again[project])
	}
}

// A mark carries the name and the project with no space in it, and
// reads back; the pid is below every container's and the same every
// time, and differs across projects for one name.
func TestTheMarkAndThePid(t *testing.T) {
	mark := Mark("/Users/w0 zro/app", "web")
	if strings.Contains(mark, " ") {
		t.Errorf("the mark has a space: %q", mark)
	}
	if project, name, ok := Unmark(mark); !ok || project != "/Users/w0 zro/app" || name != "web" {
		t.Errorf("the mark reads back as %q %q %v", project, name, ok)
	}
	if _, _, ok := Unmark("nomark"); ok {
		t.Error("a string with no seam is a mark")
	}
	a, b := PID("/a", "web"), PID("/b", "web")
	if a >= -16777217 || b >= -16777217 || a == b || a != PID("/a", "web") {
		t.Errorf("pids: %d %d", a, b)
	}
}

// The projects asked for a file are the blocks that are projects, once
// each, in order, and every project already read for stays asked
// whether or not it still has a block.
func TestTheProjectsAskedAreTheBlocks(t *testing.T) {
	projects := []work.Project{{Path: "/r/b"}, {Path: "/home"}, {Path: "/r/a"}, {Path: "/r/b"}}
	isProject := func(p string) bool { return strings.HasPrefix(p, "/r/") }
	got := Paths(projects, nil, isProject)
	if want := []string{"/r/a", "/r/b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("paths: %v, want %v", got, want)
	}
	was := map[string]File{"/r/c": {}, "/r/a": {}, "/nope": {}}
	got = Paths(projects, was, isProject)
	if want := []string{"/r/a", "/r/b", "/r/c"}; !reflect.DeepEqual(got, want) {
		t.Errorf("paths with the last reading: %v, want %v", got, want)
	}
}

// What a project has panes for, by mark: the declarations up, to pass
// over, and the panes holding an ended one, to replace; another
// project's are neither, and a down row is neither.
func TestWhatIsUpAndWhatIsHeld(t *testing.T) {
	app, lib := "/r/app", "/r/lib"
	projects := []work.Project{{Path: app, Entries: []work.Entry{
		{PID: 1, TTY: "ttys001", Declared: Mark(app, "web")},
		{PID: 2, TTY: "ttys002", Declared: Mark(app, "api")},
		{PID: PID(app, "worker"), Status: work.StatusDown, Declared: Mark(app, "worker")},
		{PID: 4, TTY: "ttys004", Declared: Mark(lib, "docs")},
		// Started by hand, somewhere with no terminal conn can see.
		{PID: 5, Status: work.StatusActive, Declared: Mark(app, "cron")},
	}}}
	panes := map[string]Pane{
		"ttys001": {ID: "%1", TTY: "ttys001"},
		"ttys002": {ID: "%2", TTY: "ttys002", Exit: "1"},
		"ttys004": {ID: "%4", TTY: "ttys004"},
	}
	up, held := UpAndHeld(projects, panes, app)
	if !reflect.DeepEqual(up, map[string]bool{Mark(app, "web"): true, Mark(app, "cron"): true}) {
		t.Errorf("up: %v", up)
	}
	if !reflect.DeepEqual(held, map[string]string{Mark(app, "api"): "%2"}) {
		t.Errorf("held: %v", held)
	}
}
