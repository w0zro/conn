package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestADotenvFileIsRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte(`# the database
DATABASE_URL=postgres://app:hunter2@localhost:5432/site
export PORT=3000
NAME="quoted value"
OTHER='single' 
TRAIL=plain # a comment
EMPTY=
not a line
`), 0o600); err != nil {
		t.Fatal(err)
	}
	vars, keys := readDotenv(path)
	want := map[string]string{
		"DATABASE_URL": "postgres://app:hunter2@localhost:5432/site",
		"PORT":         "3000",
		"NAME":         "quoted value",
		"OTHER":        "single",
		"TRAIL":        "plain",
		"EMPTY":        "",
	}
	for name, value := range want {
		if vars[name] != value {
			t.Errorf("%s = %q, want %q", name, vars[name], value)
		}
	}
	if len(vars) != len(want) {
		t.Errorf("read %v, want %d variables", vars, len(want))
	}
	if got := strings.Join(keys, " "); got != "DATABASE_URL PORT NAME OTHER TRAIL EMPTY" {
		t.Errorf("keys = %q, want the file's order", got)
	}
	if vars, keys := readDotenv(filepath.Join(dir, "none")); vars != nil || keys != nil {
		t.Errorf("a missing file read as %v", vars)
	}
}

// chained is a subject under a shell, an npm and an sh: the leaf's
// ancestors nearest first, as readEnvSubject lists them.
func chained() envSubject {
	return envSubject{
		pid:        40,
		chainNames: []string{"sh", "npm run dev", "zsh"},
		chain: []map[string]string{
			{"HOME": "/Users/me", "PATH": "/npm/bin:/usr/bin", "npm_lifecycle_event": "dev", "SHLVL": "3", "EDITOR": "vim"},
			{"HOME": "/Users/me", "PATH": "/npm/bin:/usr/bin", "npm_lifecycle_event": "dev", "SHLVL": "2", "EDITOR": "vim"},
			{"HOME": "/Users/me", "PATH": "/usr/bin", "SHLVL": "1", "EDITOR": "vim"},
		},
		server: map[string]string{"HOME": "/Users/me", "TERM_PROGRAM": "ghostty"},
		dotenv: map[string]string{"PORT": "3000"},
	}
}

func TestAVariablesSourceIsTheAncestorThatSetIt(t *testing.T) {
	s := chained()
	for _, tc := range []struct{ name, value, want string }{
		{"HOME", "/Users/me", "the terminal"},         // every ancestor had it, and the server agrees
		{"EDITOR", "vim", "the shell"},                // every ancestor had it; the server did not: the rc files
		{"npm_lifecycle_event", "dev", "npm run dev"}, // sh had it, npm had it, the shell did not
		{"PATH", "/npm/bin:/usr/bin", "npm run dev"},  // changed by npm, not introduced
		{"SHLVL", "4", "its own"},                     // nobody above had this value
		{"PORT", "3000", ".env"},
		{"TMUX", "/tmp/sock,1,0", "conn"},
	} {
		if got := sourceOf(tc.name, tc.value, s); got != tc.want {
			t.Errorf("sourceOf(%s=%s) = %q, want %q", tc.name, tc.value, got, tc.want)
		}
	}
	// The server alone, with no chain: what agrees with it is the
	// terminal's, and what does not is the shell's.
	bare := envSubject{pid: 7, server: map[string]string{"HOME": "/Users/me"}}
	if got := sourceOf("HOME", "/Users/me", bare); got != "the terminal" {
		t.Errorf("HOME under no chain = %q, want the terminal", got)
	}
	if got := sourceOf("EDITOR", "vim", bare); got != "the shell" {
		t.Errorf("EDITOR under no chain = %q, want the shell", got)
	}
}

func TestAPathListIsCountedAndItsToolsFound(t *testing.T) {
	dir := t.TempDir()
	first, second := filepath.Join(dir, "first"), filepath.Join(dir, "second")
	for _, d := range []string{first, second} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(second, "node"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(first, "go"), []byte("not executable"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := strings.Join([]string{first, second, filepath.Join(dir, "gone"), first}, ":")
	summary, tn, rows := describePathList("PATH", path, envSubject{tools: []string{"node", "go"}})
	if summary != "4 entries · 1 missing · 1 twice" || tn != toneAttn {
		t.Errorf("summary = %q in tone %d, want the count with the missing and the repeat, worth a glance", summary, tn)
	}
	byName := map[string]envRow{}
	for _, r := range rows {
		if r.name != "" {
			byName[r.name] = r
		}
	}
	if r := byName["node"]; r.note != "entry 2" || !r.always {
		t.Errorf("node = %+v, want it found in entry 2 and shown while folded", r)
	}
	if r := byName["go"]; r.note != "not on the path" {
		t.Errorf("go = %+v, want it not found: the file in first is not executable", r)
	}
	var notes []string
	for _, r := range rows {
		if r.name == "" {
			notes = append(notes, r.note)
		}
	}
	if got := strings.Join(notes, ","); got != ",,missing,again" {
		t.Errorf("entry notes = %q, want the third missing and the fourth a repeat", got)
	}
}

func TestAURLSaysWhoIsListening(t *testing.T) {
	s := envSubject{pid: 40, listeners: map[string]listener{
		"5432": {name: "docker db", pid: -1},
		"3000": {name: "npm run dev", pid: 40},
	}}
	for _, tc := range []struct {
		value, note string
		tone        tone
	}{
		{"postgres://app:hunter2@localhost/site", "docker db listens", toneGood}, // the scheme's port
		{"redis://127.0.0.1:6379", "nothing is listening", toneAttn},
		{"http://localhost:3000", "this process listens", toneGood},
		{"https://api.example.com/v1", "", tonePlain}, // elsewhere: not probed
	} {
		note, tn := urlNote(tc.value, s)
		if note != tc.note || tn != tc.tone {
			t.Errorf("urlNote(%s) = %q in tone %d, want %q in %d", tc.value, note, tn, tc.note, tc.tone)
		}
	}
	rows := describeVar("DATABASE_URL", "postgres://app:hunter2@localhost/site", ".env", s)
	if rows[0].value != "postgres://app:•••@localhost/site" {
		t.Errorf("the password shows: %q", rows[0].value)
	}
	if rows := describeVar("PORT", "3000", "the shell", s); rows[0].note != "this process listens" {
		t.Errorf("PORT's note = %q, want this process listening", rows[0].note)
	}
}

func TestSecretsEmptiesAndPathsAreReadForWhatTheyAre(t *testing.T) {
	dir := t.TempDir()
	s := envSubject{}
	if r := describeVar("API_TOKEN", "abcdef", "the shell", s)[0]; r.value != "set, 6 chars" || r.valueTone != toneQuiet {
		t.Errorf("a secret reads %q", r.value)
	}
	if r := describeVar("API_BASE", "", "the shell", s)[0]; r.value != "empty" || r.valueTone != toneAttn {
		t.Errorf("an empty value reads %q", r.value)
	}
	if r := describeVar("TMPDIR", dir, "the terminal", s)[0]; r.note != "where temporary files go" || r.value != homely(dir) {
		t.Errorf("a path that is there says %q of %q, want its meaning", r.note, r.value)
	}
	if r := describeVar("SSH_AUTH_SOCK", filepath.Join(dir, "gone"), "the terminal", s)[0]; r.note != "missing" || r.noteTone != toneBad {
		t.Errorf("a path that is not there says %q", r.note)
	}
	if r := describeVar("NODE_ENV", "production", "the shell", s)[0]; r.valueTone != toneAttn {
		t.Errorf("production is not picked out")
	}
	if r := describeVar("SHELL", "/bin/zsh", "the terminal", s)[0]; r.note != "the shell s opens" {
		t.Errorf("SHELL's meaning = %q", r.note)
	}
	// The file's word over the process's is said; the meaning waits.
	s.dotenv = map[string]string{"LOG_LEVEL": "debug", "DB_PASSWORD": "x"}
	if r := describeVar("LOG_LEVEL", "info", "the shell", s)[0]; r.note != ".env says debug" || r.noteTone != toneAttn {
		t.Errorf("a value the file disagrees with says %q", r.note)
	}
	if r := describeVar("DB_PASSWORD", "y", "the shell", s)[0]; r.note != ".env says otherwise" {
		t.Errorf("a secret the file disagrees with says %q", r.note)
	}
}

// site is a dev server's environment as the page reads it: the project's
// variables, npm's, the shell's, the terminal's, and an .env the process
// was not given all of.
func site(t *testing.T) envSubject {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	s := chained()
	for _, anc := range s.chain {
		anc["HOME"] = home
		for _, kv := range []string{"DATABASE_URL=postgres://app:hunter2@localhost:5432/site", "REDIS_URL=redis://localhost:6379", "SESSION_SECRET=s3cret", "NODE_ENV=development"} {
			name, value, _ := strings.Cut(kv, "=")
			anc[name] = value
		}
	}
	s.server["HOME"] = home
	s.title, s.dir, s.notes = "npm run dev 40", filepath.Join(home, "site"), []string{"as it was started, 2h ago"}
	s.dotenv = map[string]string{"DATABASE_URL": "postgres://app:hunter2@localhost:5432/site", "API_BASE": "https://api.example.com", "SESSION_SECRET": "s3cret"}
	s.dotenvKeys = []string{"DATABASE_URL", "API_BASE", "SESSION_SECRET"}
	s.listeners = map[string]listener{"5432": {name: "docker db", pid: -1}}
	s.env = []string{
		"DATABASE_URL=postgres://app:hunter2@localhost:5432/site",
		"REDIS_URL=redis://localhost:6379",
		"SESSION_SECRET=s3cret",
		"NODE_ENV=development",
		"HOME=" + home,
		"EDITOR=vim",
		"PATH=/npm/bin:/usr/bin",
		"npm_lifecycle_event=dev",
		"SHLVL=4",
		"TMUX=/tmp/sock,1,0",
	}
	return s
}

func TestThePageGroupsTheProjectsFirstAndTheSettersAfter(t *testing.T) {
	groups, rows := envRows(site(t))
	var titles []string
	for _, g := range groups {
		titles = append(titles, g.title)
	}
	if got := strings.Join(titles, " | "); got != "the project's | the runtime | npm run dev's | conn's | the shell's | the terminal conn started in" {
		t.Errorf("groups = %q", got)
	}
	for _, g := range groups {
		want := g.title != groupProject && g.title != groupRuntime && g.title != groupConn
		if g.folded != want {
			t.Errorf("%s folded = %v, want %v", g.title, g.folded, want)
		}
	}
	in := map[string]string{}
	var project []string
	for _, r := range rows {
		if r.kind == envVarRow {
			in[r.name] = groups[r.group].title
			if groups[r.group].title == groupProject {
				project = append(project, r.name+"="+r.value)
			}
		}
	}
	for name, group := range map[string]string{
		"DATABASE_URL": groupProject, "REDIS_URL": groupProject, "SESSION_SECRET": groupProject, "NODE_ENV": groupProject,
		"API_BASE": groupProject, "PATH": groupRuntime, "npm_lifecycle_event": "npm run dev's", "TMUX": groupConn,
		"EDITOR": groupShell, "SHLVL": groupShell, "HOME": groupTerm,
	} {
		if in[name] != group {
			t.Errorf("%s is under %q, want %q", name, in[name], group)
		}
	}
	got := strings.Join(project, "; ")
	if !strings.Contains(got, "API_BASE=not set") || strings.Contains(got, "s3cret") || strings.Contains(got, "hunter2") {
		t.Errorf("the project's rows: %s", got)
	}
}

// envPage is the page with the site read into it, drawn at a size.
func envPage(t *testing.T, width, height int) envModel {
	t.Helper()
	m := newEnvModel(40, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	next, _ = next.(envModel).Update(envReadMsg{subj: site(t)})
	return next.(envModel)
}

func envPress(m envModel, key string) envModel {
	var msg tea.KeyPressMsg
	switch key {
	case "space":
		msg = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "esc":
		msg = tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		msg = tea.KeyPressMsg{Code: tea.KeyEnter}
	case "backspace":
		msg = tea.KeyPressMsg{Code: tea.KeyBackspace}
	default:
		msg = typed(key)
	}
	next, _ := m.Update(msg)
	return next.(envModel)
}

func TestThePageDrawsFoldsAndFinds(t *testing.T) {
	m := envPage(t, 100, 30)
	page := stripANSI(m.render())
	t.Log("\n" + page)
	for _, want := range []string{"npm run dev 40", "~/site", "as it was started, 2h ago · 10 variables",
		"the project's", "DATABASE_URL", "postgres://app:•••@localhost:5432/site", "docker db listens",
		"REDIS_URL", "nothing is listening", "SESSION_SECRET", "set, 6 chars", "API_BASE", "not set",
		"the runtime", "PATH", "2 entries", "npm run dev's  1 variable ▸", "the shell's  2 variables ▸",
		"space fold · - unfold all · / find · q leave"} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	for _, hidden := range []string{"npm_lifecycle_event", "EDITOR", "hunter2", "s3cret"} {
		if strings.Contains(page, hidden) {
			t.Errorf("the page shows %q", hidden)
		}
	}
	for _, ln := range strings.Split(page, "\n") {
		if w := len([]rune(ln)); w > 100 {
			t.Errorf("a line is %d columns wide: %q", w, ln)
		}
	}
	if lines := strings.Count(page, "\n") + 1; lines != 30 {
		t.Errorf("the page is %d lines, want the window's 30", lines)
	}

	// G to the last row, the terminal's group; space unfolds it.
	m = envPress(envPress(m, "G"), "space")
	if page := stripANSI(m.render()); !strings.Contains(page, "HOME") || !strings.Contains(page, "the home directory") {
		t.Errorf("the terminal's group did not unfold:\n%s", page)
	}
	// - unfolds everything.
	m = envPress(m, "-")
	if page := stripANSI(m.render()); !strings.Contains(page, "EDITOR") || !strings.Contains(page, "npm_lifecycle_event") {
		t.Errorf("- did not unfold all:\n%s", page)
	}
	// A filter finds through the folds, and esc clears it.
	m = envPage(t, 100, 30)
	for _, k := range []string{"/", "e", "d", "i", "t"} {
		m = envPress(m, k)
	}
	page = stripANSI(m.render())
	if !strings.Contains(page, "EDITOR") || strings.Contains(page, "DATABASE_URL") || !strings.Contains(page, "/edit") {
		t.Errorf("the filter did not narrow the page:\n%s", page)
	}
	m = envPress(envPress(m, "enter"), "esc")
	if m.filter != "" || !strings.Contains(stripANSI(m.render()), "DATABASE_URL") {
		t.Errorf("esc did not clear the filter")
	}
	// esc with nothing to clear leaves; so does q.
	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape}); cmd == nil {
		t.Error("esc on a clear page did not leave")
	}
	if _, cmd := m.Update(typed("q")); cmd == nil {
		t.Error("q did not leave")
	}
}

func TestThePageSaysWhenItCannotRead(t *testing.T) {
	m := newEnvModel(1, nil)
	if page := stripANSI(m.render()); !strings.Contains(page, "reading the environment") {
		t.Errorf("before the read: %q", page)
	}
	next, _ := m.Update(envReadMsg{err: errGone})
	if page := stripANSI(next.(envModel).render()); !strings.Contains(page, "already gone") || !strings.Contains(page, "q leaves") {
		t.Errorf("after a failed read: %q", page)
	}
}

func TestTheEnvironmentPopupRunsThePageForThePid(t *testing.T) {
	var popup []string
	run := func(args ...string) (string, error) {
		switch args[0] {
		case "display-message":
			return "200 60", nil
		case "display-popup":
			popup = args
		}
		return "", nil
	}
	if err := showEnv(run, "/opt/conn", "c0", 42); err != nil {
		t.Fatal(err)
	}
	if len(popup) < 11 || popup[3] != "c0" || popup[5] != " environment " || popup[10] != shellQuote("/opt/conn")+" page env 42" {
		t.Errorf("popup = %q, want the page for pid 42 over c0", popup)
	}
}

func TestTheServersEnvironmentIsReadWithoutTheUnsets(t *testing.T) {
	run := func(args ...string) (string, error) {
		if args[0] == "show-environment" && args[1] == "-g" {
			return "HOME=/Users/me\n-DISPLAY\nPATH=/usr/bin\n", nil
		}
		return "", nil
	}
	if got := strings.Join(serverEnv(run), " "); got != "HOME=/Users/me PATH=/usr/bin" {
		t.Errorf("serverEnv = %q", got)
	}
}

func TestTheToolsOfAPlaceAreItsEcosystems(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"go.mod", "package.json", "pyproject.toml", "manage.py"} {
		if err := os.WriteFile(filepath.Join(dir, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := strings.Join(toolsFor(dir), " "); got != "go node npm python3" {
		t.Errorf("toolsFor = %q, want each once", got)
	}
}

// table is a pane's shell, an npm under it, and a node under that, as
// the process table lists them.
func table() []Proc {
	return []Proc{
		{PID: 10, PPID: 1, Command: "zsh", Argv: "-zsh", Dir: "/site", PGID: 10},
		{PID: 20, PPID: 10, Command: "npm", Argv: "npm run dev", Dir: "/site", PGID: 20},
		{PID: 30, PPID: 20, Command: "node", Argv: "node server.js", Dir: "/site", PGID: 20},
	}
}

func TestTheLeafIsReadAndAnAncestorThatCannotBeIsPassedOver(t *testing.T) {
	envs := map[int][]string{
		20: {"HOME=/Users/me", "npm_lifecycle_event=dev"},
		30: {"HOME=/Users/me", "npm_lifecycle_event=dev", "PORT=3000"},
		// 10, the zsh, keeps its environment to itself.
	}
	read := func(pid int) []string { return envs[pid] }
	s := envSubject{server: map[string]string{"HOME": "/Users/me"}}
	if err := s.describe(table(), 10, read, nil); err != nil {
		t.Fatal(err)
	}
	if s.pid != 30 || s.title != "npm run dev 30" || s.dir != "/site" {
		t.Errorf("subject = %s, pid %d in %s; want the node, named for the run", s.title, s.pid, s.dir)
	}
	if got := strings.Join(s.chainNames, " < "); got != "npm run dev" {
		t.Errorf("chain = %q, want npm alone: the zsh could not be read", got)
	}
	if got := sourceOf("npm_lifecycle_event", "dev", s); got != "npm run dev" {
		t.Errorf("npm's variable is %q's", got)
	}
	if got := sourceOf("HOME", "/Users/me", s); got != "the terminal" {
		t.Errorf("HOME is %q's", got)
	}
	if got := sourceOf("PORT", "3000", s); got != "its own" {
		t.Errorf("PORT is %q's", got)
	}
}

func TestAShellAtItsPromptThatCannotBeReadShowsWhatItStartedFrom(t *testing.T) {
	read := func(int) []string { return nil }
	s := envSubject{server: map[string]string{"HOME": "/Users/me"}}
	err := s.describe(table()[:1], 10, read, nil)
	if runtime.GOOS == "linux" {
		if err == nil {
			t.Fatal("on Linux /proc answers for a shell; nothing read is a failure")
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(s.env, " ") != "HOME=/Users/me" || len(s.notes) != 2 || !strings.Contains(s.notes[0], "zsh") {
		t.Errorf("env = %v, notes = %q; want the server's, said to be", s.env, s.notes)
	}
	// A node that cannot be read is a failure: it is not a system binary.
	if err := s.describe(table(), 10, read, nil); err == nil || !strings.Contains(err.Error(), "npm run dev 30") {
		t.Errorf("err = %v, want the leaf named", err)
	}
}

func TestALiveEnvironmentIsTheShellsAsItIsNow(t *testing.T) {
	s := envSubject{server: map[string]string{"HOME": "/Users/me"}}
	if err := s.describe(table(), 10, func(int) []string { return nil }, []string{"HOME=/Users/me", "FOO=bar"}); err != nil {
		t.Fatal(err)
	}
	if s.pid != 10 || s.title != "zsh 10" || len(s.env) != 2 || !strings.Contains(s.notes[0], "at the prompt") {
		t.Errorf("subject = %+v", s)
	}
	if got := sourceOf("FOO", "bar", s); got != "the shell" {
		t.Errorf("an export is %q's", got)
	}
}

func TestTheLivePopupCarriesTheEnvironment(t *testing.T) {
	var popup []string
	run := func(args ...string) (string, error) {
		switch args[0] {
		case "display-message":
			return "200 60", nil
		case "display-popup":
			popup = args
		}
		return "", nil
	}
	if err := showLiveEnv(run, "/opt/conn", "c0", 10, []string{"A=1", "B=two words"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"-e", "A=1", "-e", "B=two words", shellQuote("/opt/conn") + " page env live 10"}
	if got := strings.Join(popup[len(popup)-5:], "|"); got != strings.Join(want, "|") {
		t.Errorf("popup ends %q, want %q", got, strings.Join(want, "|"))
	}
}
