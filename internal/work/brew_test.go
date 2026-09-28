package work

import (
	"slices"
	"strings"
	"testing"
	"time"
)

// A declaration that starts a service with brew names the formula; any
// other line, and a brew line that is not a start, does not.
func TestABrewDeclarationNamesItsFormula(t *testing.T) {
	for command, want := range map[string]string{
		"brew services start postgresql@14": "postgresql@14",
		"brew services run redis":           "redis",
		"brew services start --all":         "",
		"brew services stop redis":          "",
		"brew install redis":                "",
		"npm run dev":                       "",
	} {
		got, ok := BrewArgs(command)
		if got != want || ok != (want != "") {
			t.Errorf("%q names %q, want %q", command, got, want)
		}
	}
}

// A service two projects declare stands under each of them: both
// declare it, either is where you would go to it, and each row counts
// how many projects share it, which the page says. It was filed once,
// under the first, while the panel was filed by state and a second row
// of one service in one block would have been the same thing twice.
func TestAServiceTwoProjectsDeclareStandsUnderEach(t *testing.T) {
	services, _ := ParseBrewServices([]byte(brewInfo))
	decl := map[string]Declared{
		"/w/a": {List: []Declaration{{Name: "db", Command: "brew services start postgresql@14"}}},
		"/w/q": {List: []Declaration{{Name: "pg", Command: "brew services start postgresql@14"}, {Name: "cache", Command: "brew services start redis"}}},
	}
	projects := []Project{
		{Path: "/w/a", Entries: []Entry{{PID: 1, Kind: KindShell, Command: "zsh", TTY: "ttys001", Status: StatusIdle}}},
		{Path: "/w/q", Entries: []Entry{{PID: 2, Kind: KindShell, Command: "zsh", TTY: "ttys002", Status: StatusIdle}}},
	}
	sockets := map[int][]Socket{24422: {{"TCP", "127.0.0.1:5432", "LISTEN"}}}
	out := AttachBrew(projects, decl, services, sockets, nil)
	var rows []string
	for _, pl := range out {
		for _, e := range pl.Entries {
			if e.Brew != "" {
				rows = append(rows, pl.Path+" "+e.Brew+" "+e.Status+" x"+string(rune('0'+e.Shared)))
			}
		}
	}
	// A service that ended badly is not running, with its stamp
	// saying how it went, as a container that exited is.
	want := "/w/a postgresql@14 ACTIVE x2, /w/q postgresql@14 ACTIVE x2, /w/q redis EXIT 78 x1"
	if got := strings.Join(rows, ", "); got != want {
		t.Errorf("the rows are %q, want %q", got, want)
	}
}

// An asking of brew sends nothing anywhere: brew's analytics, its update
// check and its hints are off in the environment conn gives it, and the
// beat between askings is slow enough that booting brew's ruby costs
// the machine little.
func TestBrewIsAskedQuietly(t *testing.T) {
	for _, want := range []string{"HOMEBREW_NO_ANALYTICS=1", "HOMEBREW_NO_AUTO_UPDATE=1", "HOMEBREW_NO_ENV_HINTS=1"} {
		if !slices.Contains(BrewEnv, want) {
			t.Errorf("brew is asked without %s", want)
		}
	}
	if BrewBeat < 10*time.Second {
		t.Errorf("brew is asked every %s", BrewBeat)
	}
}

// Brew's answer is read for what each service is: running under what
// pid, or not, and with what exit where its last run ended badly. A
// nought exit is no exit.
func TestBrewServicesAreReadFromBrew(t *testing.T) {
	services, err := ParseBrewServices([]byte(brewInfo))
	if err != nil || len(services) != 3 {
		t.Fatalf("read %d services, %v", len(services), err)
	}
	pg := BrewServiceNamed(services, "postgresql@14")
	if pg == nil || !pg.Running || pg.PID != 24422 || pg.Exit != "" || pg.Log != "/opt/homebrew/var/log/postgresql@14.log" {
		t.Errorf("postgres read as %+v", pg)
	}
	if word, fault := BrewStatus(*pg); word != StatusActive || fault {
		t.Errorf("a running service reads %s", word)
	}
	if word, fault := BrewStatus(*BrewServiceNamed(services, "herdr")); word != StatusDown || fault {
		t.Errorf("a service never started reads %s", word)
	}
	if word, fault := BrewStatus(*BrewServiceNamed(services, "redis")); word != "EXIT 78" || !fault {
		t.Errorf("a service that ended badly reads %s, fault %v", word, fault)
	}
	if BrewServiceNamed(services, "nothing") != nil {
		t.Error("a formula brew did not report was found")
	}
}

// brew services info --all --json as brew writes it: a service never
// started, one running under a pid, and one whose last run ended badly.
const brewInfo = `[
  {"name": "herdr", "running": false, "loaded": false, "pid": null, "exit_code": null, "status": "none",
   "command": "/opt/homebrew/opt/herdr/bin/herdr server", "log_path": "/opt/homebrew/var/log/herdr.log"},
  {"name": "postgresql@14", "running": true, "loaded": true, "pid": 24422, "exit_code": 0, "status": "started",
   "command": "/opt/homebrew/opt/postgresql@14/bin/postgres -D /opt/homebrew/var/postgresql@14",
   "log_path": "/opt/homebrew/var/log/postgresql@14.log"},
  {"name": "redis", "running": false, "loaded": true, "pid": null, "exit_code": 78, "status": "error",
   "command": "/opt/homebrew/opt/redis/bin/redis-server /opt/homebrew/etc/redis.conf", "log_path": "/opt/homebrew/var/log/redis.log"}
]`
