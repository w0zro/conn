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
