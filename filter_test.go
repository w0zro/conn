package main

import (
	"strings"
	"testing"
)

// found types the query into the navigator's filter, from the narrowed view,
// and returns the rows it lists.
func found(m model, query string) []string {
	return navColumn(typeFilter(press(narrowed(m), "/"), query))
}

func TestTheFilterReachesAShellByThePlansName(t *testing.T) {
	// The row says web; the process table says zsh. Typing what you see has
	// to find what you see.
	m := withProcList(90, 14, []Project{{Name: "brand", Path: "/p/brand"}},
		[]Proc{{PID: 700, PPID: 1, Command: "zsh", Dir: "/p/brand"}})
	m.terms = map[int]*remoteTerm{700: {pid: 700, dir: "/p/brand", name: "web"}}
	m.rebuild()

	wantRows(t, found(m, "web"), []string{"   brand", "    ▸ web"})
}

func TestTheFilterReachesAShellByWhatItsRunSaid(t *testing.T) {
	// A shell at its prompt after a failed run says so beside its name, and
	// "failed" is the thing you would type to find every one of them.
	m := withProcList(90, 14, []Project{{Name: "brand", Path: "/p/brand"}},
		[]Proc{{PID: 700, PPID: 1, Command: "zsh", Dir: "/p/brand"}})
	m.terms = map[int]*remoteTerm{700: {pid: 700, dir: "/p/brand", name: "test", exit: "1", summary: "3 failed"}}
	m.rebuild()

	wantRows(t, found(m, "failed"), []string{"   brand", "    ▸ test · 3 failed"})
}

func TestTheFilterReachesAnAgentByTheNameItsUserGaveIt(t *testing.T) {
	// A session renamed docs is the row that says docs, and its model is
	// beside it: either word finds it, and so does the kind it no longer
	// says, since the kind is still what it is.
	m := withProcList(90, 14, []Project{{Name: "conn", Path: "/p/conn"}},
		[]Proc{
			{PID: 700, PPID: 1, Command: "zsh", Dir: "/p/conn"},
			{PID: 701, PPID: 700, Command: "claude", Dir: "/p/conn"},
		})
	m.agents = asAgents(map[int]claudeSession{
		701: {PID: 701, Name: "docs", NameSource: userNamedSource, Model: "claude-opus-4-8"},
	})
	m.rebuild()

	for _, q := range []string{"docs", "opus", "claude"} {
		wantRows(t, found(m, q), []string{"   conn", "    ▸ docs · opus-4-8"})
	}
}

func TestTheFilterReachesAProcessByWhatItWasRunWith(t *testing.T) {
	// npm run dev is a node to the table and "npm run dev" to the row, and
	// dev is what you would type.
	m := withProcList(90, 14, []Project{{Name: "brand", Path: "/p/brand"}},
		[]Proc{{PID: 700, PPID: 1, Command: "node", Argv: "/opt/homebrew/bin/npm run dev", Dir: "/p/brand"}})

	wantRows(t, found(m, "dev"), []string{"   brand", "    ▸ npm run dev"})
}

func TestTheFilterReachesAProcessByItsPort(t *testing.T) {
	// The port is the thing you were about to go and look up, and so the
	// thing you might have in hand: 8080 finds the server on it, alone or
	// beside the name the row draws it with.
	m := withProcList(90, 14, []Project{{Name: "brand", Path: "/p/brand"}},
		[]Proc{
			{PID: 700, PPID: 1, Command: "zsh", Dir: "/p/brand"},
			{PID: 701, PPID: 700, Command: "python3", Argv: "python3 -m http.server 8080", Dir: "/p/brand", Ports: []string{"8080"}},
		})
	m.terms = map[int]*remoteTerm{700: {pid: 700, dir: "/p/brand", name: "web"}}
	m.rebuild()

	wantRows(t, found(m, "8080"), []string{"   brand", "    ▸ web · :8080"})
	wantRows(t, found(m, "web 8080"), []string{"   brand", "    ▸ web · :8080"})
}

func TestTheFilterStillPrunesWhatDoesNotAnswer(t *testing.T) {
	// Answering by more of the row is not answering to everything: a vim
	// beside the web shell is still pruned away when web is typed.
	m := withProcList(90, 14, []Project{{Name: "brand", Path: "/p/brand"}},
		[]Proc{
			{PID: 700, PPID: 1, Command: "zsh", Dir: "/p/brand"},
			{PID: 702, PPID: 1, Command: "vim", Dir: "/p/brand"},
		})
	m.terms = map[int]*remoteTerm{700: {pid: 700, dir: "/p/brand", name: "web"}}
	m.rebuild()

	rows := found(m, "web")
	if len(rows) != 2 || !strings.Contains(rows[1], "web") {
		t.Fatalf("rows = %q, want brand and its web shell alone", rows)
	}
}

func TestAPlaceAnswersByItsNameAndItsGroupNotTheRootsPath(t *testing.T) {
	// The path within the root is a name — hsg/brand — and the root's is
	// not: the letters of /Users/anyone/projects would let nearly any
	// query find every repository by its absolute path.
	brand := Project{Name: "brand", Path: "/Users/me/projects/hsg/brand"}
	if !matchesFilter(brand, "hsg") || !matchesFilter(brand, "brand") || !matchesFilter(brand, "hsg brand") {
		t.Error("a repository should answer by its name and its parent's")
	}
	if matchesFilter(brand, "users") || matchesFilter(brand, "projects") {
		t.Error("a repository should not answer by the root's path")
	}
	alone := Project{Name: "conn", Path: "/var/folders/T/TestAMoveEndsTheHoldARunPutsOnTheCursor/001/conn"}
	if matchesFilter(alone, "docs") {
		t.Error("a repository should not answer by letters scattered through the path above it")
	}
}
