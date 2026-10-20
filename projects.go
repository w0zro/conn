package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// A project is what work is done on: a git repository, with its
// worktrees, or a directory work was done in that is not one. conn is
// told none of them. It reads where work has happened from what the
// machine already records — the directories Claude Code has worked in,
// and where the panes conn holds are — and the newest is first.
type project struct {
	Name string
	Root string
	Last time.Time // when work last happened in it
	// What runs in it in conn, and its conversations waiting on you,
	// wherever they run. Turning is what is at work on something that
	// will come to rest: its conversations working, wherever they run,
	// and its checks running.
	Running, Waiting, Turning int
	// Group is a folder holding two or more repositories: listed with
	// them beneath it, and a project of its own that takes in all of
	// theirs. Depth is a project's place under its folder in the list.
	Group bool
	Depth int
}

// isFolder says whether a directory is a folder of repositories: two or
// more of the directories directly in it are git checkouts. A folder
// directly in home (~/projects, ~/src) is where projects are kept, not
// a project, and home and / are where everything is.
func isFolder(dir, home string) bool {
	return filepath.Dir(dir) != home && holdsRepos(dir, home)
}

// holdsRepos says whether two or more of the directories directly in a
// directory are git checkouts.
func holdsRepos(dir, home string) bool {
	if dir == home || dir == "/" || dir == "" {
		return false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, e.Name(), ".git")); err == nil {
			n++
			if n == 2 {
				return true
			}
		}
	}
	return false
}

// group lays the projects out as the list shows them: each folder of
// repositories with its repositories beneath it, and the rest on their
// own. A folder is listed with what it holds summed — what waits, what
// runs, the newest work — and comes where that puts it: what waits
// first, then the newest. folder says whether a directory is a folder
// of repositories.
func group(ps []project, folder func(string) bool) []project {
	folders := map[string]*project{}
	under := map[string][]project{}
	var top []project
	ensure := func(root string) *project {
		f := folders[root]
		if f == nil {
			f = &project{Name: filepath.Base(root), Root: root, Group: true}
			folders[root] = f
		}
		return f
	}
	add := func(f *project, p project) {
		f.Running += p.Running
		f.Waiting += p.Waiting
		f.Turning += p.Turning
		if p.Last.After(f.Last) {
			f.Last = p.Last
		}
	}
	for _, p := range ps {
		switch parent := filepath.Dir(p.Root); {
		case folder(p.Root):
			// Work done in the folder itself is the folder's.
			add(ensure(p.Root), p)
		case folder(parent):
			add(ensure(parent), p)
			p.Depth = 1
			under[parent] = append(under[parent], p)
		default:
			top = append(top, p)
		}
	}
	for _, f := range folders {
		top = append(top, *f)
	}
	order := func(list []project) {
		sort.SliceStable(list, func(i, j int) bool {
			a, b := list[i], list[j]
			if (a.Waiting > 0) != (b.Waiting > 0) {
				return a.Waiting > 0
			}
			if !a.Last.Equal(b.Last) {
				return a.Last.After(b.Last)
			}
			return a.Root < b.Root
		})
	}
	order(top)
	var out []project
	for _, p := range top {
		out = append(out, p)
		if p.Group {
			kids := under[p.Root]
			order(kids)
			out = append(out, kids...)
		}
	}
	return out
}

// belongs says whether work in a directory is a project's: the
// directory's project is it, or, for a folder of repositories, is in
// it. root is the directory's project.
func (p project) belongs(root string) bool {
	return root != "" && (root == p.Root || (p.Group && within(root, p.Root)))
}

// rootOf is the project a directory belongs to: the repository it is in,
// by git's common directory, so a worktree is its repository's; or the
// directory itself where it is in none. A directory that is gone, or a
// temporary one, belongs to no project.
func rootOf(dir string) string {
	if dir == "" || temporary(dir) {
		return ""
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return ""
	}
	return repoRoot(dir)
}

// repoRoot is the repository a directory is in, or the directory.
func repoRoot(dir string) string {
	root, _ := repository(dir)
	return root
}

// repository is the repository a directory is in, and whether it is in
// one: a directory in none stands as itself.
func repository(dir string) (string, bool) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return dir, false
	}
	common := strings.TrimSpace(string(out))
	if filepath.Base(common) == ".git" {
		return filepath.Dir(common), true
	}
	// A bare repository's worktrees: the repository is the project.
	return common, true
}

// headPath is where a checkout keeps its HEAD: a worktree's is in the
// repository's git directory, under worktrees.
func headPath(tree string) string {
	out, err := exec.Command("git", "-C", tree, "rev-parse", "--path-format=absolute", "--git-path", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// branchAt is the branch a HEAD file names, or the checkout's own name
// where it names none: detached, or no HEAD to read.
func branchAt(head, tree string) string {
	b, err := os.ReadFile(head)
	if ref, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "ref: refs/heads/"); err == nil && ok {
		return ref
	}
	return filepath.Base(tree)
}

// temporary says whether a directory is scratch the system clears.
func temporary(dir string) bool {
	for _, t := range []string{os.TempDir(), "/tmp", "/private/tmp", "/private/var/folders", "/var/folders"} {
		t = filepath.Clean(t)
		if dir == t || strings.HasPrefix(dir, t+"/") {
			return true
		}
	}
	return false
}

// A sighting is work seen in a directory at a moment.
type sighting struct {
	Dir  string
	When time.Time
}

// A project is a checkout under home: something you have, not
// somewhere work has happened. repoDepth is how far under home one is
// looked for, enough for a folder of folders of them.
const repoDepth = 4

// repositories is every checkout under a directory, to a depth: a
// directory with a .git in it, which is not looked into further. Hidden
// directories and the ones that hold other people's code, or the
// system's, are passed over. A .git in the directory itself is passed
// over too, as home is where everything is and not a project.
func repositories(home string, depth int) []string {
	var out []string
	var walk func(dir string, left int)
	walk = func(dir string, left int) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		if dir != home {
			for _, e := range entries {
				if e.Name() == ".git" {
					out = append(out, dir)
					return
				}
			}
		}
		if left == 0 {
			return
		}
		for _, e := range entries {
			if e.IsDir() && !passedOver(e.Name()) {
				walk(filepath.Join(dir, e.Name()), left-1)
			}
		}
	}
	walk(home, depth)
	sort.Strings(out)
	return out
}

// passedOver says whether a directory is not looked in for checkouts.
func passedOver(name string) bool {
	switch name {
	case "Library", "node_modules", "vendor":
		return true
	}
	return strings.HasPrefix(name, ".")
}

// findProjects is the projects: the checkouts, and whatever else conn
// holds a pane in, each with its newest work from the sightings, newest
// work first. A sighting somewhere that is no project says nothing.
// home is never a project of its own: it is where everything is.
func findProjects(repos []string, seen []sighting, home string, root func(string) string) []project {
	by := map[string]*project{}
	for _, r := range repos {
		if r == "" || r == home || r == "/" || by[r] != nil {
			continue
		}
		by[r] = &project{Name: filepath.Base(r), Root: r}
	}
	roots := map[string]string{} // directory → project, each asked once
	for _, s := range seen {
		r, ok := roots[s.Dir]
		if !ok {
			r = root(s.Dir)
			roots[s.Dir] = r
		}
		if p := by[r]; p != nil && s.When.After(p.Last) {
			p.Last = s.When
		}
	}
	out := make([]project, 0, len(by))
	for _, p := range by {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Last.Equal(out[j].Last) {
			return out[i].Last.After(out[j].Last)
		}
		return out[i].Root < out[j].Root
	})
	return out
}

// claudeSightings is a sighting for each directory Claude Code has
// worked in, at its newest transcript. cwd reads where a transcript was
// had. Each transcript is asked, not one per folder: a folder can hold
// transcripts of a directory since renamed, and of another whose name
// encodes alike.
func claudeSightings(all []conversation, cwd func(conversation) string) []sighting {
	newest := map[string]time.Time{}
	for _, c := range all {
		if dir := cwd(c); dir != "" && c.When.After(newest[dir]) {
			newest[dir] = c.When
		}
	}
	out := make([]sighting, 0, len(newest))
	for dir, when := range newest {
		out = append(out, sighting{Dir: dir, When: when})
	}
	return out
}

// within says whether a directory is root or under it.
func within(dir, root string) bool {
	return dir == root || strings.HasPrefix(dir, root+"/")
}

// tilde writes a path under home with ~.
func tilde(path, home string) string {
	if home != "" && within(path, home) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

// expand reads a typed path: ~ is home, and a relative path is from home,
// where conn starts.
func expand(typed, home string) string {
	switch {
	case typed == "~":
		return home
	case strings.HasPrefix(typed, "~/"):
		return filepath.Join(home, typed[2:])
	case filepath.IsAbs(typed):
		return filepath.Clean(typed)
	}
	return filepath.Join(home, typed)
}
