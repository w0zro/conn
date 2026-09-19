package main

import (
	"sort"
	"strings"
)

// The panel is filed by what a thing is doing, not by which project it
// is in. What wants you comes first, under its own eyebrow; then what
// is working, what is serving, what is idle, and what is not running;
// and the
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
	groupServing    = "\x003 serving"
	groupIdle       = "\x004 idle"
	groupNotRunning = "\x005 not running"
)

var groupOrder = []string{groupWaiting, groupWorking, groupServing, groupIdle, groupNotRunning}

// groupTitle is a group's eyebrow.
func groupTitle(path string) string {
	switch path {
	case groupWaiting:
		return "WAITING FOR YOU"
	case groupWorking:
		return "WORKING"
	case groupServing:
		return "SERVING"
	case groupIdle:
		return "IDLE"
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

// stateOf is the group a row files under. What is alive with nothing
// to report — a shell at its prompt, an editor, a process up and not
// doing anything, a contact at rest, a stopped row with its stamp — is
// idle: at rest, nothing pending, yours when you want it, which is
// the word its rows already use. A stopped row is alive, and fg
// brings it back; one that ended with a code has ended, the same as
// one that ended clean, and is not running, with its stamp saying
// how it went.
func stateOf(e entry) string {
	switch e.status {
	case statusWaiting:
		return groupWaiting
	case statusWorking:
		return groupWorking
	case statusDown, statusEnded:
		return groupNotRunning
	}
	if strings.HasPrefix(e.status, exitWord) {
		return groupNotRunning
	}
	if serving(e) {
		return groupServing
	}
	return groupIdle
}

// serving says whether a row is a thing to reach: it is alive and has
// a port, one it listens on or one its container publishes. A server
// is not idle, it is at its work, and a port is what tells a
// node that serves from a node that builds, both of which the process
// table calls the same. A contact is filed by what it asks of you and
// never by what it has open, and what is not running serves nothing.
func serving(e entry) bool {
	switch e.status {
	case statusDown, statusEnded:
		return false
	}
	return e.kind != kindContact && len(e.ports) > 0
}

// portsWord is how a row says its ports after its command: web · :8438,
// or every port it has, lowest first. Nothing for a row with none.
func portsWord(ports []string) string {
	if len(ports) == 0 {
		return ""
	}
	return " · " + portsColumn(ports)
}

// portsColumn is the ports as the filed panel's column has them, ahead
// of the command: :8438, or every port lowest first. Nothing for none.
func portsColumn(ports []string) string {
	if len(ports) == 0 {
		return ""
	}
	return ":" + strings.Join(ports, " :")
}

// byState files every row of the reading under its group, the groups
// in their order and the waiting rows oldest first. Every group stands
// whether it has rows or not, with its count, so the panel keeps one
// shape as rows come and go: a group that came and went with its rows
// moved everything under it a reading at a time, and the eye lost its
// place. Only a reading with nothing in it at all has no groups, and
// says so instead. A filed row stands alone, at the margin, and
// remembers the project it was read in and the depth it stood at
// there, so that what conn does at a project is done at the row's own,
// and the page still finds what runs it.
func byState(projects []project) []project {
	groups := map[string][]entry{}
	filed := 0
	for _, pl := range projects {
		for _, e := range pl.entries {
			g := stateOf(e)
			e.filed, e.from, e.fromDepth, e.depth = true, pl.path, e.depth, 0
			groups[g] = append(groups[g], e)
			filed++
		}
	}
	if filed == 0 {
		return nil
	}
	// A brew service two projects declare is one service, and stands
	// once, under the first project that declares it; the row counts
	// the projects and says * for them, see filedFrom.
	for g, rows := range groups {
		seen := map[string]bool{}
		kept := rows[:0]
		for _, e := range rows {
			if e.brew != "" {
				if seen[e.brew] {
					continue
				}
				seen[e.brew] = true
			}
			kept = append(kept, e)
		}
		groups[g] = kept
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
		out = append(out, project{path: g, entries: groups[g]})
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
