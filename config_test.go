package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeConfig puts a config file where conn will look for it, under a
// config home of the test's own, and answers the home it was written
// for.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	home := t.TempDir()
	path := configPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

// A machine with no config file is the ordinary case, and reads as a
// config that says nothing rather than as an error.
func TestNoConfigFileIsNoError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	c, err := readConfig(t.TempDir())
	if err != nil {
		t.Fatalf("a machine with no config file says %v", err)
	}
	if len(c.Roots) != 0 {
		t.Errorf("a config that is not there names roots: %q", c.Roots)
	}
}

// The roots come from the config file when the environment says
// nothing, and a leading ~ is the home: nothing else expands it, since
// no shell has read the file.
func TestTheRootsComeFromTheConfigFile(t *testing.T) {
	t.Setenv("CONN_ROOTS", "")
	home := writeConfig(t, `{"roots": ["~/projects", "/srv/src", "~"]}`)
	got := roots(t, home)
	want := []string{filepath.Join(home, "projects"), "/srv/src", home}
	if len(got) != len(want) {
		t.Fatalf("the roots are %q, not %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("the roots are %q, not %q", got, want)
		}
	}
}

// The environment is the nearer word: a conn started for one job takes
// CONN_ROOTS over the file that says what is usually meant.
func TestTheEnvironmentIsAskedBeforeTheFile(t *testing.T) {
	home := writeConfig(t, `{"roots": ["/from/the/file"]}`)
	t.Setenv("CONN_ROOTS", "/from/the/environment")
	if got := roots(t, home); len(got) != 1 || got[0] != "/from/the/environment" {
		t.Errorf("the roots are %q", got)
	}
}

// A file that names no roots leaves conn where it would have been
// without a file at all: ~/projects.
func TestAFileThatNamesNoRootsLeavesTheDefault(t *testing.T) {
	t.Setenv("CONN_ROOTS", "")
	home := writeConfig(t, `{"roots": []}`)
	if got := roots(t, home); len(got) != 1 || got[0] != filepath.Join(home, "projects") {
		t.Errorf("the roots are %q", got)
	}
}

// A file that will not parse is said out loud, named so the reader can
// open it, and conn is still a working conn on the default roots.
func TestAFileThatWillNotParseIsSaid(t *testing.T) {
	t.Setenv("CONN_ROOTS", "")
	home := writeConfig(t, `{"roots": ["~/projects",`)
	got, err := projectRoots(home)
	if err == nil {
		t.Fatal("a config file that will not parse says nothing")
	}
	if !strings.Contains(err.Error(), "config.json") {
		t.Errorf("the error does not name the file: %v", err)
	}
	if len(got) != 1 || got[0] != filepath.Join(home, "projects") {
		t.Errorf("a config that could not be read left the roots %q", got)
	}
}

// The console says where conn's configuration is and how it read. A
// machine with no file is not a fault; a file that will not parse is.
func TestTheConsoleSaysHowTheConfigRead(t *testing.T) {
	t.Setenv("CONN_ROOTS", "")
	for _, c := range []struct {
		what   string
		body   string // "" writes no file at all
		status string
		fault  bool
	}{
		{"a file that names roots", `{"roots": ["~"]}`, nominal, false},
		{"a file that names none", `{"roots": []}`, noRoots, false},
		{"a file of nothing conn knows", `{"projectsDir": "~/projects"}`, noRoots, false},
		{"a file that will not parse", `{"roots": [`, notRead, true},
		{"no file at all", "", notWritten, false},
	} {
		home := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		if c.body != "" {
			path := configPath(home)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(c.body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		k := configCheck(readConfigState(home), home)
		if k.status != c.status || k.fault != c.fault {
			t.Errorf("%s reads %q (fault %v), not %q (fault %v)", c.what, k.status, k.fault, c.status, c.fault)
		}
	}
}

// The environment standing in front of the file is said on the file's
// own line: the roots below it are then not the ones in it.
func TestTheConsoleSaysWhenTheEnvironmentIsInForce(t *testing.T) {
	home := writeConfig(t, `{"roots": ["/from/the/file"]}`)
	t.Setenv("CONN_ROOTS", "/from/the/environment")
	k := configCheck(readConfigState(home), home)
	if !strings.Contains(k.value, "CONN_ROOTS IN FORCE") {
		t.Errorf("the line does not say the environment is in force: %q", k.value)
	}
	if k.status != nominal {
		t.Errorf("a file that reads fine is %q", k.status)
	}
}

// A root gets a line of its own, and says what it turned out to be
// here. A root that is not on this machine is not a fault; something
// that is not a directory at all is.
func TestEveryRootGetsALine(t *testing.T) {
	t.Setenv("CONN_ROOTS", "")
	home := writeConfig(t, `{"roots": ["~", "~/nowhere", "~/afile"]}`)
	if err := os.WriteFile(filepath.Join(home, "afile"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	ks := rootChecks(readConfigState(home), home)
	if len(ks) != 3 {
		t.Fatalf("the roots take %d lines, not 3", len(ks))
	}
	for i, want := range []struct {
		status string
		fault  bool
	}{{nominal, false}, {missing, false}, {"NOT A DIR", true}} {
		if ks[i].status != want.status || ks[i].fault != want.fault {
			t.Errorf("root %d reads %q (fault %v), not %q (fault %v)", i, ks[i].status, ks[i].fault, want.status, want.fault)
		}
		if ks[i].label != "ROOT" {
			t.Errorf("root %d is labelled %q", i, ks[i].label)
		}
	}
}

// Every status is right-aligned in a column statusW wide, and one that
// does not fit runs into the dots that lead to it. The words conn has
// are held to the column here, where the console is not being read.
func TestEveryStatusFitsItsColumn(t *testing.T) {
	for _, status := range []string{nominal, unknown, unchecked, notWritten, noRoots, missing, notRead, "NOT A DIR", "READ ONLY", "NO PATH"} {
		if len(status) > statusW {
			t.Errorf("%q is %d wide, and the column is %d", status, len(status), statusW)
		}
	}
}

// The editor is the operator's own, and vi where they have named none.
func TestTheEditorIsTheOperatorsOwn(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	if got := editor(); got != "vi" {
		t.Errorf("with nothing named the editor is %q", got)
	}
	t.Setenv("EDITOR", "nvim")
	if got := editor(); got != "nvim" {
		t.Errorf("EDITOR names %q", got)
	}
	t.Setenv("VISUAL", "emacs")
	if got := editor(); got != "emacs" {
		t.Errorf("VISUAL is not taken first: %q", got)
	}
}

// Editing a config there is none of makes one, carrying the roots conn
// is walking as it stands: what opens says what conn is doing. What it
// wrote is a file conn reads back as the same roots, which is the whole
// point of writing it rather than an empty buffer.
func TestEditingMakesTheFileWhereThereIsNone(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CONN_ROOTS", "")
	home := t.TempDir()
	path, err := openableConfig(home)
	if err != nil {
		t.Fatal(err)
	}
	if path != configPath(home) {
		t.Errorf("the file was made at %q", path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"~/projects"`) {
		t.Errorf("the file conn wrote does not name the roots it walks:\n%s", b)
	}
	got := roots(t, home)
	if len(got) != 1 || got[0] != filepath.Join(home, "projects") {
		t.Errorf("conn reads back %q from the file it wrote", got)
	}
}

// A file the operator already has is theirs, and editing it opens what
// is there rather than anything conn would have written.
func TestEditingLeavesAFileThatIsThereAlone(t *testing.T) {
	body := `{"roots": ["/theirs"]}`
	home := writeConfig(t, body)
	if _, err := openableConfig(home); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(configPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != body {
		t.Errorf("conn wrote over a file that was there:\n%s", b)
	}
}

// The command puts the operator in the file, with a path a shell takes
// back whole however it is spelt.
func TestTheEditCommandNamesTheFile(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "nvim")
	if got := editConfigCommand("/a path/config.json"); got != `nvim '/a path/config.json'` {
		t.Errorf("the command reads %q", got)
	}
}
