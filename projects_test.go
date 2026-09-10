package main

import (
	"os"
	"path/filepath"
	"testing"
)

// mkRepo makes a directory and puts a .git in it.
func mkRepo(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(path, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// names is what the list would read down its rows, a group's
// repositories marked as such.
func names(ps []project) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		switch {
		case p.repos > 0:
			out = append(out, p.name+"/")
		case p.grouped:
			out = append(out, "  "+p.name)
		default:
			out = append(out, p.name)
		}
	}
	return out
}

// The shape is the declaration: a folder of two repositories is the
// project they make, with its own row and its repositories under it by
// their own names; a folder of one stays flat, and goes by as much of
// its path as tells it apart.
func TestAFolderOfTwoRepositoriesIsAProject(t *testing.T) {
	root := real(t, t.TempDir())
	mkRepo(t, filepath.Join(root, "w0zro", "conn"))
	mkRepo(t, filepath.Join(root, "w0zro", "vim.pro"))
	mkRepo(t, filepath.Join(root, "dotfiles"))
	mkRepo(t, filepath.Join(root, "experiments", "one-off"))

	ps, err := findProjects([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"dotfiles", "experiments/one-off", "w0zro/", "  conn", "  vim.pro"}
	if got := names(ps); !equal(got, want) {
		t.Errorf("the list reads %q, not %q", got, want)
	}
	for _, p := range ps {
		if p.name == "w0zro" && (p.repos != 2 || p.path != filepath.Join(root, "w0zro")) {
			t.Errorf("the group is %+v", p)
		}
	}
}

// A repository is not descended into, the names a package manager
// leaves are not entered, and a directory that says it is a cache is
// taken at its word.
func TestTheWalkStaysOutOfWhatIsNotWork(t *testing.T) {
	root := t.TempDir()
	mkRepo(t, filepath.Join(root, "app"))
	mkRepo(t, filepath.Join(root, "app", "node_modules", "dep"))
	mkRepo(t, filepath.Join(root, "node_modules", "dep"))
	mkRepo(t, filepath.Join(root, ".cache", "clone"))
	cache := filepath.Join(root, "build")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, "CACHEDIR.TAG"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	mkRepo(t, filepath.Join(cache, "clone"))

	ps, err := findProjects([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if got := names(ps); len(got) != 1 || got[0] != "app" {
		t.Errorf("the walk found %q", got)
	}
}

// Two roots: a name they both offer says which root it came from, and a
// root that is not on this machine is passed over so long as one is.
func TestRootsThatBothOfferANameSayWhichIsWhich(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	mkRepo(t, filepath.Join(home, "api"))
	mkRepo(t, filepath.Join(work, "api"))
	mkRepo(t, filepath.Join(work, "web"))

	ps, err := findProjects([]string{home, work, filepath.Join(home, "nowhere")})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Base(home) + "/api", filepath.Base(work) + "/api", "web"}
	if got := names(ps); !equal(got, want) {
		t.Errorf("the list reads %q, not %q", got, want)
	}
	if _, err := findProjects([]string{filepath.Join(home, "nowhere")}); err == nil {
		t.Error("a lone root that is not there says nothing")
	}
}

// The roots come from the environment, and are ~/projects when it says
// nothing.
func TestTheRootsComeFromTheEnvironment(t *testing.T) {
	t.Setenv("CONN_ROOTS", "")
	if got := projectRoots("/Users/w0zro"); len(got) != 1 || got[0] != "/Users/w0zro/projects" {
		t.Errorf("the roots are %q", got)
	}
	t.Setenv("CONN_ROOTS", "/work"+string(filepath.ListSeparator)+"/Users/w0zro/projects")
	if got := projectRoots("/Users/w0zro"); len(got) != 2 || got[0] != "/work" || got[1] != "/Users/w0zro/projects" {
		t.Errorf("the roots are %q", got)
	}
}

// real is a directory as the walk answers it, symlinks resolved: on
// macOS a temporary directory is reached through one.
func real(t *testing.T, dir string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
