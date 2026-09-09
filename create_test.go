package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestANameGoesUnderTheRootAndAPathWhereItSays(t *testing.T) {
	// A name goes under the root; a folder typed before it goes in it,
	// made if it has to be; a path from ~ or / is where it says.
	if got, err := newProjectPath("/p", "w0zro/site"); err != nil || got != "/p/w0zro/site" {
		t.Errorf("path = %q (%v), want the name under its folder under the root", got, err)
	}
	if got, err := newProjectPath("/p", "site"); err != nil || got != "/p/site" {
		t.Errorf("path = %q (%v), want the name under the root", got, err)
	}
	if got, err := newProjectPath("/p", "~/elsewhere/site"); err != nil || !strings.HasSuffix(got, "/elsewhere/site") || strings.HasPrefix(got, "/p") {
		t.Errorf("path = %q (%v), want a path from ~ taken as it is", got, err)
	}
}

func TestANameThatClimbsOutIsRefused(t *testing.T) {
	// The finder said the project goes under the root; a name that climbs
	// out, or names the root itself, would put it somewhere else.
	for _, bad := range []string{"", ".", "..", "../x", "w0zro/../../x"} {
		if _, err := newProjectPath("/p", bad); err == nil {
			t.Errorf("%q: want refused", bad)
		}
	}
}

func TestAFolderOnTheWayIsMade(t *testing.T) {
	// A folder typed before the name that is not there yet is made: a new
	// group, with its first repository in it.
	root := t.TempDir()
	if err := createProject(filepath.Join(root, "new", "site")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "new", "site", ".git")); err != nil {
		t.Errorf("no repository at new/site: %v", err)
	}
}

func TestAProjectAlreadyThereIsNotMadeOver(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "site"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := createProject(filepath.Join(root, "site"))
	if err == nil || !strings.Contains(err.Error(), "already there") {
		t.Errorf("err = %v, want the directory left alone", err)
	}
	if _, err := os.Stat(filepath.Join(root, "site", ".git")); err == nil {
		t.Error("a repository was made in a directory that was already there")
	}
}
