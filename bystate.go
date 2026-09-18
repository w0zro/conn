package main

import "sort"

// The panel is filed by what a thing is doing, not by which project it
// is in. What wants you comes first, under its own eyebrow; then what
// is working, what is open and quiet, and what is not running; and the
// project drops to the right of the row, in the faint, where it is read
// second. A panel by project put the one row that had stopped for the
// operator wherever its project happened to sort, under whatever shell
// happened to run it; a panel by state puts it at the top.
//
// The tree is still there: z shows it, by project, with everything
// under everything.

// The groups, in the order they stand, each a path no project can have
// and sorting as they stand, since the blocks are drawn in path order.
const (
	groupWaiting    = "\x001 waiting"
	groupWorking    = "\x002 working"
	groupOpen       = "\x003 open"
	groupNotRunning = "\x004 not running"
)

var groupOrder = []string{groupWaiting, groupWorking, groupOpen, groupNotRunning}

// groupTitle is a group's eyebrow.
func groupTitle(path string) string {
	switch path {
	case groupWaiting:
		return "WAITING FOR YOU"
	case groupWorking:
		return "WORKING"
	case groupOpen:
		return "OPEN"
	case groupNotRunning:
		return "NOT RUNNING"
	}
	return ""
}

// isGroup says whether a block's path is a group's rather than a
// project's.
func isGroup(path string) bool {
	return len(path) > 0 && path[0] == 0
}

// stateOf is the group a row files under.
func stateOf(e entry) string {
	switch e.status {
	case statusWaiting:
		return groupWaiting
	case statusWorking:
		return groupWorking
	case statusDown, statusEnded:
		return groupNotRunning
	}
	return groupOpen
}

// byState files every row of the reading under its group, the groups
// in their order and the waiting rows oldest first. A filed row stands
// alone, at the margin,
// and remembers the project it was read in and the depth it stood at
// there, so that what conn does at a project is done at the row's own,
// and the page still finds what runs it.
func byState(projects []project) []project {
	groups := map[string][]entry{}
	for _, pl := range projects {
		for _, e := range pl.entries {
			g := stateOf(e)
			e.filed, e.from, e.fromDepth, e.depth = true, pl.path, e.depth, 0
			groups[g] = append(groups[g], e)
		}
	}
	waiting := groups[groupWaiting]
	sort.SliceStable(waiting, func(i, j int) bool {
		a, b := waiting[i].since, waiting[j].since
		if a.IsZero() || b.IsZero() {
			return !a.IsZero() && b.IsZero()
		}
		return a.Before(b)
	})
	var out []project
	for _, g := range groupOrder {
		if len(groups[g]) > 0 {
			out = append(out, project{path: g, entries: groups[g]})
		}
	}
	return out
}

// unfiled is the reading by project again: every filed row back under
// the path it was read in, in the order the projects first appear, for
// whatever asks about projects rather than rows. A group is no place
// to open a shell.
func unfiled(projects []project) []project {
	var out []project
	at := map[string]int{}
	for _, pl := range projects {
		if !isGroup(pl.path) {
			out = append(out, pl)
			continue
		}
		for _, e := range pl.entries {
			i, ok := at[e.from]
			if !ok {
				i = len(out)
				at[e.from] = i
				out = append(out, project{path: e.from})
			}
			e.filed, e.from, e.depth, e.fromDepth = false, "", e.fromDepth, 0
			out[i].entries = append(out[i].entries, e)
		}
	}
	return out
}

// rowsBlock is the project a row belongs to: the block it stands in,
// or the one it was read in before it was filed by state.
func rowsBlock(projects []project, e entry, in project) project {
	if !e.filed {
		return in
	}
	for _, pl := range projects {
		if pl.path == e.from {
			return pl
		}
	}
	return project{path: e.from}
}

// depthOf is how deep a row stood in the tree it was read from, whether
// or not it has since been filed by state.
func depthOf(e entry) int {
	if e.filed {
		return e.fromDepth
	}
	return e.depth
}
