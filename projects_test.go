package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
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
func names(ps []projectRow) []string {
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

// testProjects is a list as the roots would give it: two groups, the
// repositories that stand alone among them, and one name qualified by
// the root it came from.
var testProjects = []projectRow{
	{name: "arboreum.io", path: "/Users/w0zro/projects/arboreum.io", repos: 2},
	{name: "content", path: "/Users/w0zro/projects/arboreum.io/content", grouped: true},
	{name: "welcome", path: "/Users/w0zro/projects/arboreum.io/welcome", grouped: true},
	{name: "compose-demo", path: "/Users/w0zro/projects/compose-demo"},
	{name: "experiments/one-off", path: "/Users/w0zro/projects/experiments/one-off"},
	{name: "work/api", path: "/work/api"},
	{name: "w0zro", path: "/Users/w0zro/projects/w0zro", repos: 3},
	{name: "conn", path: "/Users/w0zro/projects/w0zro/conn", grouped: true},
	{name: "quickfix-pro", path: "/Users/w0zro/projects/w0zro/quickfix-pro", grouped: true},
	{name: "vim.pro", path: "/Users/w0zro/projects/w0zro/vim.pro", grouped: true},
}

func testList(filter string) projectsReport {
	return composeProjects(testProjects, filter, []string{"/Users/w0zro/projects"}, "/Users/w0zro", false, "")
}

// The list in the panel, the list narrowed, and the list with nothing
// found yet are files of record.
func TestProjectsMatchTheGolden(t *testing.T) {
	golden(t, "projects-48x30.txt", texts(drawProjects(testList(""), 0, 48, 30, plain)))
	golden(t, "projects-filtered-48x30.txt", texts(drawProjects(testList("pro"), 2, 48, 30, plain)))
	empty := composeProjects(nil, "", []string{"/Users/w0zro/projects"}, "/Users/w0zro", true, "")
	golden(t, "projects-scanning-48x30.txt", texts(drawProjects(empty, 0, 48, 30, plain)))
	failed := composeProjects(nil, "", []string{"/Users/w0zro/projects"}, "/Users/w0zro", false, "THE ROOTS COULD NOT BE WALKED: no such directory")
	golden(t, "projects-unwalked-48x30.txt", texts(drawProjects(failed, 0, 48, 30, plain)))
}

// The list's rows hold: the count against the right, the filter on its
// own line, a group's repositories indented under it, the cursor on one
// row, and no row past the width.
func TestProjectsLayOut(t *testing.T) {
	rows := drawProjects(testList(""), 6, 48, 30, plain)
	text := texts(rows)
	for _, s := range []string{
		"PROJECTS", "10 FOUND", "FIND  ▏",
		"arboreum.io", "2 REPOS", "  content", "compose-demo",
		"experiments/one-off", " ▸ w0zro", "3 REPOS", "  conn",
	} {
		if !strings.Contains(text, s) {
			t.Errorf("the list lacks %q:\n%s", s, text)
		}
	}
	if len(rows) != 30 {
		t.Errorf("%d rows", len(rows))
	}
	if strings.Count(text, "▸") != 1 {
		t.Errorf("the cursor marks %d rows", strings.Count(text, "▸"))
	}
	for _, r := range drawProjects(testList("conn"), 0, 48, 30, colored()) {
		if w := utf8.RuneCountInString(stripEscapes(r.text)); w != 48 {
			t.Errorf("a colored row paints %d columns", w)
		}
	}
	// The filter is on the header's count.
	b := testList("pro")
	text = texts(drawProjects(b, 0, 48, 30, plain))
	if !strings.Contains(text, "3 OF 10") {
		t.Errorf("narrowed:\n%s", text)
	}
}

// A project answers the filter by its own name and by the name of the
// folder that groups it; a group answers for its repositories, and
// carries down the ones that answered, so its count says what is drawn.
func TestTheFilterAnswersByNameAndByGroup(t *testing.T) {
	for _, c := range []struct {
		filter string
		want   []string
	}{
		{"", []string{"arboreum.io/", "  content", "  welcome", "compose-demo", "experiments/one-off", "work/api", "w0zro/", "  conn", "  quickfix-pro", "  vim.pro"}},
		{"pro", []string{"w0zro/", "  quickfix-pro", "  vim.pro"}},
		{"w0zro", []string{"w0zro/", "  conn", "  quickfix-pro", "  vim.pro"}},
		{"CONN", []string{"w0zro/", "  conn"}},
		{"arbo", []string{"arboreum.io/", "  content", "  welcome"}},
		{"nothing at all", nil},
	} {
		if got := names(matching(testProjects, c.filter)); !equal(got, c.want) {
			t.Errorf("%q leaves %q, not %q", c.filter, got, c.want)
		}
	}
	if got := matching(testProjects, "quickfix"); len(got) != 2 || got[0].repos != 1 {
		t.Errorf("a group narrowed to one repository says %+v", got)
	}
}

// A list taller than the terminal scrolls to keep the cursor in view.
func TestAListThatWillNotFitScrolls(t *testing.T) {
	rows := drawProjects(testList(""), 9, 48, 10, plain)
	text := texts(rows)
	if len(rows) != 10 || !strings.Contains(text, "ABOVE") || !strings.Contains(text, "▸   vim.pro") {
		t.Errorf("at 48x10 with the cursor on the last row:\n%s", text)
	}
}
