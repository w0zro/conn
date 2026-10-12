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
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return dir
	}
	common := strings.TrimSpace(string(out))
	if filepath.Base(common) == ".git" {
		return filepath.Dir(common)
	}
	// A bare repository's worktrees: the repository is the project.
	return common
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

// findProjects is the projects the sightings name, newest work first.
// home is never a project of its own: it is where everything is.
func findProjects(seen []sighting, home string, root func(string) string) []project {
	roots := map[string]string{} // directory → project, each asked once
	by := map[string]*project{}
	for _, s := range seen {
		r, ok := roots[s.Dir]
		if !ok {
			r = root(s.Dir)
			roots[s.Dir] = r
		}
		if r == "" || r == home || r == "/" {
			continue
		}
		p := by[r]
		if p == nil {
			p = &project{Name: filepath.Base(r), Root: r}
			by[r] = p
		}
		if s.When.After(p.Last) {
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
