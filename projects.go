package main

import (
	"cmp"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The projects: every place work could happen, where the watch is every
// place it is happening. They are found by walking the roots — where the
// checkouts are kept — for repositories, and the shape of what is found
// is the declaration: a folder holding two or more of them is the
// project they collectively make, and gets a row of its own, since that
// is the level the work is often about. A folder of one stays flat, so
// nothing grows a header per repository.

// A project is a place work could happen: a repository under one of the
// roots, or the folder that groups two or more of them.
type project struct {
	name    string // what the list calls it: enough of the path to tell it apart
	path    string
	grouped bool // a repository under a group, listed beneath it
	repos   int  // a group's repositories; none for a repository
}

// roots are the directories conn looks for projects under: CONN_ROOTS,
// a list in the path list separator's spelling, or ~/projects. A root
// that is not on this machine is still a root; the walk decides whether
// it is there, not the environment.
func projectRoots(home string) []string {
	list := os.Getenv("CONN_ROOTS")
	if list == "" {
		return []string{filepath.Join(home, "projects")}
	}
	var out []string
	for _, d := range filepath.SplitList(list) {
		if d = strings.TrimSpace(d); d != "" {
			out = append(out, d)
		}
	}
	return out
}

// realRoots is the roots as the process table has them. A root reached
// through a symlink is one name for a directory and lsof answers with
// the other, so the two are made the same name before anything is
// compared against them. A root that is not there is left as it was
// written: it names nothing either way.
func realRoots(roots []string) []string {
	out := make([]string, 0, len(roots))
	for _, r := range roots {
		if real, err := filepath.EvalSymlinks(r); err == nil {
			r = real
		}
		out = append(out, r)
	}
	return out
}

// skipDirs are never entered. They hold what a package manager put
// there, not the projects the list is for, and walking them is most of
// what walking a root would cost.
var skipDirs = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	"target":       true,
	".build":       true,
	"Pods":         true,
	".venv":        true,
	"venv":         true,
}

// findProjects is every project under the roots, in the order the list
// draws them: each group followed by its repositories, and the
// repositories that stand alone among them, by name.
//
// A root that cannot be walked is passed over so long as another can:
// the same environment rides between machines, and a checkout that is
// only on the other one should not empty the list. Only every root
// failing is an error, so a lone mistyped root still says so.
func findProjects(roots []string) ([]project, error) {
	var repos []project
	rootOf := map[string]string{} // a repository's path, to the root it was found under
	seen := map[string]bool{}
	var firstErr error
	walked := false
	for _, root := range roots {
		if real, err := filepath.EvalSymlinks(root); err == nil {
			root = real
		}
		found, err := walkRoot(root)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		walked = true
		for _, path := range found {
			if seen[path] {
				continue // one root under another would list a repository twice
			}
			seen[path] = true
			rootOf[path] = root
			repos = append(repos, project{name: relName(root, path), path: path})
		}
	}
	if !walked && firstErr != nil {
		return nil, firstErr
	}
	groups := deriveGroups(repos, rootOf)
	qualify(repos, rootOf)
	qualify(groups, groupRoots(groups, repos, rootOf))
	return order(groups, repos), nil
}

// walkRoot is every git repository under a root, at any depth. A
// repository is not descended into: a checkout within a checkout is the
// business of the one that holds it.
func walkRoot(root string) ([]string, error) {
	var found []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == root {
				return err // the root itself: there is nothing to walk
			}
			return nil // a directory that cannot be read is passed over
		}
		if !d.IsDir() {
			return nil
		}
		if path != root && (skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
			return fs.SkipDir
		}
		if isRepo(path) {
			found = append(found, path)
			return fs.SkipDir
		}
		// A directory tagged as a cache says itself that it holds nothing
		// to find. CACHEDIR.TAG is the tag the tools that keep caches
		// agreed to leave for a walk like this one, and it reaches what no
		// list of names can.
		if path != root && isCacheDir(path) {
			return fs.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return found, nil
}

// isRepo says whether a directory is the top of a git repository. .git
// is a directory in a clone and a file in a worktree or a submodule.
func isRepo(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// isCacheDir says whether a directory carries a CACHEDIR.TAG.
func isCacheDir(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "CACHEDIR.TAG"))
	return err == nil
}

// deriveGroups finds the folders that group repositories: a
// repository's own parent, when it is not a root itself and holds a
// second repository. The repositories were put in one folder because
// they make one project, and there is no file to drop anywhere to say
// so. A repository in a group goes by its own directory name, since the
// header above it already says the rest.
func deriveGroups(repos []project, rootOf map[string]string) []project {
	isRoot := map[string]bool{}
	for _, root := range rootOf {
		isRoot[root] = true
	}
	siblings := map[string][]int{}
	for i, r := range repos {
		if parent := filepath.Dir(r.path); !isRoot[parent] {
			siblings[parent] = append(siblings[parent], i)
		}
	}
	var groups []project
	for parent, idx := range siblings {
		if len(idx) < 2 {
			continue
		}
		for _, i := range idx {
			repos[i].name = filepath.Base(repos[i].path)
			repos[i].grouped = true
		}
		root := rootOf[repos[idx[0]].path]
		groups = append(groups, project{name: relName(root, parent), path: parent, repos: len(idx)})
	}
	return groups
}

// groupRoots is each group's root, taken from a repository of its own.
func groupRoots(groups, repos []project, rootOf map[string]string) map[string]string {
	roots := map[string]string{}
	for _, g := range groups {
		for _, r := range repos {
			if filepath.Dir(r.path) == g.path {
				roots[g.path] = rootOf[r.path]
				break
			}
		}
	}
	return roots
}

// qualify puts the root's own name before the names that two roots both
// offered: with a checkout at work and one at home, the root's name is
// the only thing that tells an api here from an api there.
func qualify(ps []project, rootOf map[string]string) {
	byName := map[string][]int{}
	for i, p := range ps {
		byName[p.name] = append(byName[p.name], i)
	}
	for _, idx := range byName {
		if len(idx) < 2 {
			continue
		}
		roots := map[string]bool{}
		for _, i := range idx {
			roots[rootOf[ps[i].path]] = true
		}
		if len(roots) < 2 {
			continue // one root's own doing, and its names already differ
		}
		for _, i := range idx {
			ps[i].name = filepath.Base(rootOf[ps[i].path]) + "/" + ps[i].name
		}
	}
}

// order lays the projects out as the list draws them: the groups and
// the repositories that stand alone together by name, and each group's
// repositories under it.
func order(groups, repos []project) []project {
	under := map[string][]project{}
	var top []project
	for _, r := range repos {
		if r.grouped {
			under[filepath.Dir(r.path)] = append(under[filepath.Dir(r.path)], r)
			continue
		}
		top = append(top, r)
	}
	top = append(top, groups...)
	slices.SortFunc(top, byName)
	out := make([]project, 0, len(repos)+len(groups))
	for _, p := range top {
		out = append(out, p)
		if p.repos > 0 {
			kids := under[p.path]
			slices.SortFunc(kids, byName)
			out = append(out, kids...)
		}
	}
	return out
}

// byName orders projects by name, case aside, and by path between two
// that share one.
func byName(a, b project) int {
	return cmp.Or(cmp.Compare(strings.ToLower(a.name), strings.ToLower(b.name)), cmp.Compare(a.path, b.path))
}

// relName is a path as its root's, in slashes: the name a project goes
// by when nothing groups it, which is as much of the way down as it
// takes to tell it from the others under that root.
func relName(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.Base(path)
	}
	return filepath.ToSlash(rel)
}

// The list is the projects as a page: a line to type into, and under it
// every project, each group with its repositories beneath it. What is
// typed narrows the rows — a project answers by its own name and by the
// name of the folder that groups it, which is where the work is often
// called by — and the cursor is on one row, which enter opens a shell
// at. The list is drawn on the same measure as the watch, in the rail
// beside the slot.

// The list's words as things stand.
type projectsReport struct {
	filter   string
	rows     []project // the projects the filter left, in the order they draw
	total    int       // how many there are before it
	roots    []string  // where conn looked, from ~, for when it found nothing
	scanning bool
	err      string
}

// composeProjects words the list: the filter's rows out of the whole,
// and the count of both.
func composeProjects(ps []project, filter string, roots []string, home string, scanning bool, err string) projectsReport {
	b := projectsReport{filter: filter, rows: matching(ps, filter), total: len(ps), scanning: scanning, err: err}
	for _, root := range roots {
		b.roots = append(b.roots, tilde(root, home))
	}
	return b
}

// matching is the projects a filter leaves. A repository answers by its
// own name and by its group's, since a group is what a piece of work is
// often called by; a group answers by its name and by its
// repositories', and carries down the ones that answered. A group's
// count is what is under it, so the number says what is drawn.
func matching(ps []project, filter string) []project {
	f := strings.ToLower(strings.TrimSpace(filter))
	if f == "" {
		return ps
	}
	hit := func(p project) bool { return strings.Contains(strings.ToLower(p.name), f) }
	var out []project
	for i := 0; i < len(ps); {
		p := ps[i]
		if p.repos == 0 {
			if hit(p) {
				out = append(out, p)
			}
			i++
			continue
		}
		var kids []project
		j := i + 1
		for ; j < len(ps) && ps[j].grouped; j++ {
			if hit(p) || hit(ps[j]) {
				kids = append(kids, ps[j])
			}
		}
		if len(kids) > 0 {
			p.repos = len(kids)
			out = append(out, p)
			out = append(out, kids...)
		}
		i = j
	}
	return out
}

// convoDirs is the directories a project's conversations could be filed
// under: its own, and for a group each repository beneath it in turn —
// a transcript is filed by the exact directory it was had in, which for
// a group is one of its repositories, not the folder that names them.
func convoDirs(all []project, p project) []string {
	dirs := []string{p.path}
	if p.repos == 0 {
		return dirs
	}
	for _, c := range all {
		if c.grouped && filepath.Dir(c.path) == p.path {
			dirs = append(dirs, c.path)
		}
	}
	return dirs
}

// The list's columns: the name from the margin, a group's repositories
// indented under it, and a group's count flush with the measure. The
// filter's line puts what is typed where a name goes, so the rows read
// down from it.
const (
	nestW = 2
	findW = 6
	caret = "▏"
)

// drawProjects renders the list for a terminal of the given size, with
// the cursor on the given row.
func drawProjects(b projectsReport, cursor, width, height int, p palette) []row {
	width = max(width, railMinCols)
	measure, _, _ := columns(width)
	c := canvas{p: p, width: width}

	// The header: the name of the view, and against the right the count
	// — of everything, or of what the filter left out of it.
	c.blank(0)
	l := c.line()
	l.add(p.orange+p.bold, "PROJECTS")
	right := strconv.Itoa(b.total) + " FOUND"
	switch {
	case b.scanning && b.total == 0:
		right = "" // nothing has been found yet, and none is not a count
	case b.filter != "":
		right = strconv.Itoa(len(b.rows)) + " OF " + strconv.Itoa(b.total)
	}
	l.to(measure - utf8.RuneCountInString(right))
	l.add(p.gray, right)
	c.emit(l, 0, false)
	c.rule(0, measure)

	// The line typed into: the word, and the filter with the caret after
	// it, so it is plain that the keys go here.
	l = c.line()
	l.add(p.gray, "FIND")
	l.to(findW)
	l.add(p.ink+p.bold, fit(b.filter, measure-findW-1, false))
	l.add(p.orange+p.bold, caret)
	c.emit(l, 0, false)

	room := height
	if height == 0 {
		room = 1 << 30
	}
	var body []row
	cursorRow := -1
	d := canvas{p: p, width: width}
	say := func(color, s string) {
		d.blank(0)
		l := d.line()
		l.add(color, s)
		d.emit(l, 0, true)
		body = d.rows
	}
	switch {
	case b.err != "":
		say(p.chip, " "+strings.ToUpper(b.err)+" ")
	case b.scanning && len(b.rows) == 0:
		say(p.gray, "SCANNING")
	case len(b.rows) == 0 && b.filter != "":
		say(p.gray, "NOTHING ANSWERS TO "+strings.ToUpper(b.filter))
	case len(b.rows) == 0:
		say(p.gray, "NO REPOSITORIES UNDER "+strings.ToUpper(strings.Join(b.roots, " · ")))
	default:
		d.blank(0)
		for i, pr := range b.rows {
			l := d.line()
			if i == cursor {
				l.p = p.chosen()
				if p.plain {
					l.mark = "▸"
				}
				cursorRow = len(d.rows)
			}
			// A group is a title with its repositories under it, the way a
			// place is on the watch; a repository that stands alone is a
			// row at the margin like any other.
			switch {
			case pr.repos > 0:
				count := strconv.Itoa(pr.repos) + " REPO"
				if pr.repos != 1 {
					count += "S"
				}
				l.add(p.parchment+p.bold, fit(pr.name, measure-utf8.RuneCountInString(count)-2, true))
				l.to(measure - utf8.RuneCountInString(count))
				l.add(p.gray, count)
			case pr.grouped:
				l.to(nestW)
				l.add(p.ink, fit(pr.name, measure-nestW, true))
			default:
				l.add(p.ink, fit(pr.name, measure, true))
			}
			d.emit(l, 0, false)
		}
		body = d.rows
	}
	c.rows = append(c.rows, scrolled(body, cursorRow, room-len(c.rows), width, p)...)

	// The bottom row is a note's, when there is one, and the ground
	// otherwise, as on the watch.
	if height > 0 {
		for len(c.rows) < height {
			c.blank(0)
		}
	}
	return c.rows
}
