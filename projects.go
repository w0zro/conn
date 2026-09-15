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

// The projects: every project work could happen, where the processes
// view is every project it is happening in. They are found by walking
// the roots — where the checkouts are kept — for repositories, and the
// shape of what is found is the declaration: a folder holding two or
// more of them is the project they collectively make, and gets a row of
// its own, since that is the level the work is often about. A folder of
// one stays flat, so nothing grows a header per repository.

// A row of the list. Most are projects: a repository under one of the
// roots, or the folder that groups two or more of them. A row with a
// pid is not a project but a process running in the one above it, and
// carries that project's path as well, so every key that acts on the
// project the panel is looking at reaches the same place from either
// row.
type projectRow struct {
	name    string // what the list calls it: enough of the path to tell it apart
	path    string
	grouped bool // a repository under a group, listed beneath it
	repos   int  // a group's repositories; none for a repository
	// A process listed under its project: the terminal the row is
	// reached by, what kind of thing it is, what it is doing, and
	// whether it is waiting on the operator.
	pid     int
	tty     string
	kind    string
	doing   string
	waiting bool
	nest    int // how many steps in from the margin the row is drawn
}

// words are what a row answers a filter with: a project by its name,
// and a process by what it is and what it is doing.
func (p projectRow) words() string {
	if p.pid != 0 {
		return p.kind + " " + p.doing
	}
	return p.name
}

// roots are the directories conn looks for projects under, in the order
// conn asks for them: CONN_ROOTS, a list in the path list separator's
// spelling; then the roots the config file names; then ~/projects. The
// environment is asked first because it is the nearer word — a conn
// started for one job, ahead of the file that says what is usually
// meant. A root that is not on this machine is still a root; the walk
// decides whether it is there, not the environment.
//
// A config file that will not parse is an error, and the error is
// answered with the roots conn would have had without it: the operator
// hears about the file, and conn is still a working conn meanwhile.
func projectRoots(home string) ([]string, error) {
	if out := splitRoots(os.Getenv("CONN_ROOTS"), home); len(out) > 0 {
		return out, nil
	}
	c, err := readConfig(home)
	if err != nil {
		return defaultRoots(home), err
	}
	if out := cleanRoots(c.Roots, home); len(out) > 0 {
		return out, nil
	}
	return defaultRoots(home), nil
}

// defaultRoots is where conn looks when nothing says otherwise.
func defaultRoots(home string) []string {
	return []string{filepath.Join(home, "projects")}
}

// splitRoots is a list of roots as the environment writes one, in the
// path list separator's spelling.
func splitRoots(list, home string) []string {
	if list == "" {
		return nil
	}
	return cleanRoots(filepath.SplitList(list), home)
}

// cleanRoots is the roots as conn will walk them: the blanks dropped,
// and a leading ~ made the home it stands for.
func cleanRoots(roots []string, home string) []string {
	var out []string
	for _, d := range roots {
		if d = strings.TrimSpace(d); d != "" {
			out = append(out, expandHome(d, home))
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
func findProjects(roots []string) ([]projectRow, error) {
	var repos []projectRow
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
			repos = append(repos, projectRow{name: relName(root, path), path: path})
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
func deriveGroups(repos []projectRow, rootOf map[string]string) []projectRow {
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
	var groups []projectRow
	for parent, idx := range siblings {
		if len(idx) < 2 {
			continue
		}
		for _, i := range idx {
			repos[i].name = filepath.Base(repos[i].path)
			repos[i].grouped = true
		}
		root := rootOf[repos[idx[0]].path]
		groups = append(groups, projectRow{name: relName(root, parent), path: parent, repos: len(idx)})
	}
	return groups
}

// groupRoots is each group's root, taken from a repository of its own.
func groupRoots(groups, repos []projectRow, rootOf map[string]string) map[string]string {
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
func qualify(ps []projectRow, rootOf map[string]string) {
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
func order(groups, repos []projectRow) []projectRow {
	under := map[string][]projectRow{}
	var top []projectRow
	for _, r := range repos {
		if r.grouped {
			under[filepath.Dir(r.path)] = append(under[filepath.Dir(r.path)], r)
			continue
		}
		top = append(top, r)
	}
	top = append(top, groups...)
	slices.SortFunc(top, byName)
	out := make([]projectRow, 0, len(repos)+len(groups))
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
func byName(a, b projectRow) int {
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

// withProcesses puts the processes running in each project under it, so
// that the list is the machine and not only the disk: every project
// work could happen in, and in each of them the work happening now.
//
// Only the processes conn holds a live pane for. The list is how you
// get to a process, and one conn can only report has nowhere to go —
// on a machine with forty of them, rows that do nothing are most of the
// length. The processes view is still the census.
//
// Work happening where the walk found no project — a shell in a
// directory under no root, which is most of what an operator has open
// besides their checkouts — would otherwise be the one thing this mode
// could not reach. It gets a heading of its own, named the way the
// processes view names it, at the foot of the list: it is not a project
// and has no place in the order the walk put the projects in.
func withProcesses(ps []projectRow, projects []project, panes map[string]pane, roots []string, home string) []projectRow {
	under := map[string][]projectRow{}
	for _, pl := range projects {
		for _, e := range pl.entries {
			if !reachable(panes[e.tty]) {
				continue
			}
			under[pl.path] = append(under[pl.path], projectRow{
				path: pl.path, pid: e.pid, tty: e.tty, kind: e.kind,
				doing: activityOf(e), waiting: e.status == statusWaiting,
			})
		}
	}
	out := make([]projectRow, 0, len(ps))
	listed := map[string]bool{}
	for _, p := range ps {
		out = append(out, p)
		listed[p.path] = true
		nest := 1
		if p.grouped {
			nest = 2
		}
		for _, r := range under[p.path] {
			r.nest = nest
			out = append(out, r)
		}
	}
	var elsewhere []projectRow
	for _, pl := range projects {
		if listed[pl.path] || len(under[pl.path]) == 0 {
			continue
		}
		name := projectName(pl.path, roots, home)
		if name == "" {
			name = "NO PROJECT"
		}
		elsewhere = append(elsewhere, projectRow{name: name, path: pl.path})
	}
	slices.SortFunc(elsewhere, byName)
	for _, p := range elsewhere {
		out = append(out, p)
		for _, r := range under[p.path] {
			r.nest = 1
			out = append(out, r)
		}
	}
	return out
}

// The list is the projects as a page: a line to type into, and under it
// every project, each group with its repositories beneath it and the
// processes running in each beneath that. What is typed narrows the
// rows — a project answers by its own name and by the name of the
// folder that groups it, which is where the work is often called by,
// and a process by what it is and what it is doing — and the cursor is
// on one row, which enter opens a shell at, or goes into where the row
// is a process. The list is drawn on the same measure as the processes
// view, in the panel beside the bay.

// The list's words as things stand.
type projectsReport struct {
	filter   string
	rows     []projectRow // the rows the filter left, in the order they draw
	total    int          // how many there are before it
	roots    []string     // where conn looked, from ~, for when it found nothing
	scanning bool
	err      string
}

// composeProjects words the list: the filter's rows out of the whole,
// and the count of both.
//
// The count is of rows and not of projects. The header answers how much
// of the list is in front of you and how much the typing cut, and a
// process running in a project is as much a row to be found here as the
// project is — counting only the projects would have the number
// disagreeing with what the operator can see.
func composeProjects(ps []projectRow, filter string, roots []string, home string, scanning bool, err string) projectsReport {
	b := projectsReport{filter: filter, rows: matching(ps, filter), total: len(ps), scanning: scanning, err: err}
	for _, root := range roots {
		b.roots = append(b.roots, tilde(root, home))
	}
	return b
}

// matching is what a filter leaves of the list. Nothing is ever listed
// without the project it is in above it, and a row that answers carries
// down everything under it: a group is what a piece of work is often
// called by, and asking for it is asking for its repositories and for
// what is running in them. A group's count is what is under it, so the
// number says what is drawn.
func matching(ps []projectRow, filter string) []projectRow {
	f := strings.ToLower(strings.TrimSpace(filter))
	if f == "" {
		return ps
	}
	var out []projectRow
	for _, u := range units(ps) {
		out = append(out, leftOf(u, f)...)
	}
	return out
}

// units slices the list into what belongs together: each row at the
// margin — a project that stands alone, or a group — with everything
// listed under it.
func units(ps []projectRow) [][]projectRow {
	var out [][]projectRow
	for i := 0; i < len(ps); {
		j := i + 1
		for ; j < len(ps) && (ps[j].grouped || ps[j].pid != 0); j++ {
		}
		out = append(out, ps[i:j])
		i = j
	}
	return out
}

// leftOf is what a filter leaves of one unit: the whole of it where the
// row at the margin answers, and otherwise that row over whatever under
// it answered — each repository in turn on the same terms, so a
// process brings its repository up with it and a repository brings its
// processes down.
func leftOf(u []projectRow, f string) []projectRow {
	hit := func(p projectRow) bool { return strings.Contains(strings.ToLower(p.words()), f) }
	head := u[0]
	if hit(head) {
		return u
	}
	var kept []projectRow
	for i := 1; i < len(u); {
		if u[i].pid != 0 { // a process of the row at the margin's own
			if hit(u[i]) {
				kept = append(kept, u[i])
			}
			i++
			continue
		}
		j := i + 1 // a repository and the processes under it
		for ; j < len(u) && u[j].pid != 0; j++ {
		}
		switch sub := u[i:j]; {
		case hit(sub[0]):
			kept = append(kept, sub...)
		default:
			var procs []projectRow
			for _, r := range sub[1:] {
				if hit(r) {
					procs = append(procs, r)
				}
			}
			if len(procs) > 0 {
				kept = append(kept, sub[0])
				kept = append(kept, procs...)
			}
		}
		i = j
	}
	if len(kept) == 0 {
		return nil
	}
	head.repos = 0
	for _, r := range kept {
		if r.grouped {
			head.repos++
		}
	}
	return append([]projectRow{head}, kept...)
}

// sessionDirs is the directories a project's sessions could be filed
// under: its own, and for a group each repository beneath it in turn —
// a transcript is filed by the exact directory it was had in, which for
// a group is one of its repositories, not the folder that names them.
func sessionDirs(all []projectRow, p projectRow) []string {
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
	width = max(width, panelMinCols)
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
			// project is in the processes view; a repository that stands alone
			// is a row at the margin like any other.
			switch {
			case pr.pid != 0:
				// A process under its project, in the columns the processes
				// view puts it in. WAITING is stamped where a group's count
				// goes: it is the one thing that would have you come to this
				// list to get somewhere rather than to start something, and
				// the block is what conn stamps everywhere else it means you.
				// It does not blink here — the blink is the station saying so
				// while you are looking elsewhere, and in the list you are
				// looking for it.
				in := pr.nest * nestW
				doingW := measure - in - kindW
				if pr.waiting {
					doingW -= len(statusWaiting) + 3
				}
				l.to(in)
				l.add(p.gray, fit(pr.kind, kindW-1, false))
				l.to(in + kindW)
				l.add(p.ink, fit(pr.doing, doingW, false))
				if pr.waiting {
					l.to(measure - len(statusWaiting) - 2)
					l.add(p.chip, " "+statusWaiting+" ")
				}
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

	// The ground fills what the rows do not, as in the processes view.
	if height > 0 {
		for len(c.rows) < height {
			c.blank(0)
		}
	}
	return c.rows
}
