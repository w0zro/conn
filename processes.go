package main

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/w0zro/conn/internal/console"
	"github.com/w0zro/conn/internal/draw"
	"github.com/w0zro/conn/internal/room"
	"github.com/w0zro/conn/internal/work"
	"github.com/w0zro/conn/internal/work/claude"
	"github.com/w0zro/conn/internal/work/declared"

	"github.com/w0zro/conn/internal/config"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// The processes view: what is running, by project. Each project work
// is happening in is a block — its path as a title, and under it a row
// for each process that stands for its own work there, nested under
// whatever runs it the way the processes actually are: its kind, what
// it is doing, its terminal, how long it has stood as it does, and the
// word for how it stands. What is under a name indents two columns
// from it: a block's rows under its title, a row under the row that
// runs it, its kind and command shifted in together and the rest of
// its columns staying where they are. A project that holds other
// projects, as a folder of checkouts does, is a block with theirs
// nested under it by name, the way the list groups them, and the
// folder's heading is drawn even where nothing runs in the folder
// itself, so that its repositories have something to sit under. The
// projects are in the order of their paths, as the list has them.
// Within one, everything sits where it started and stays there for as
// long as it lives, oldest first, so what is new goes on the end and
// nothing above it moves. A cursor is on one row, which is
// drawn on a raised ground from edge to edge, and the rows scroll to
// keep it in view. What conn holds — a process in a pane of the server,
// which can be reached — is written in the ink; work conn can only
// report is dimmed a rank. In the panel, which is narrower than the
// console, the terminal column is left off and the rest close up; the
// row on the right, in the bay, is in orange.

// The processes view's words, composed from the projects as of a
// moment.
type processesReport struct {
	projects []projectBlock
	spin     int    // the spinner's frame, turned by the readings; see state.go
	err      string // why the table could not be read, when it could not
	stalled  bool   // docker went quiet; its rows are as last seen
	notice   string // what the server would not do, said under the rows
	inside   bool   // conn is in its server, and rows can be reached
	filed    bool   // the panel's drawing, folded and under eyebrows; z draws the tree
	lit      bool   // the annunciators' lit half; see the waiting word below
	digits   bool   // the contacts' digits are keys now: the processes view has the keys, and nothing is asked
}

type projectBlock struct {
	path string // the title: the project's name, or under a holding project its own
	nest int    // how many projects hold it, each a level in from the margin
	rows []processRow
	note string // what is wrong with the project's .conn, where something is
}

type processRow struct {
	pid                               int
	kind, command, tty, since, status string
	fault                             bool
	reach                             string   // the pane that holds it, in conn's server
	shown                             bool     // it is in the bay, on the right
	depth                             int      // how deep under its project's own root
	over                              bool     // a declared process whose pane holds only its last output
	name                              string   // the declared name, where the row is a declaration's
	age                               string   // how long a waiting row has waited, as the panel says it
	stands                            string   // the kind the panel marks it as; a folded shell's is not its own
	ports                             []string // the ports it listens on or publishes, said at the right of a panel row and after the command in the tree
	num                               string   // the digit that goes to it, on the first ten contacts as drawn; see numbered
	carried                           string   // what a contact carries, as the panel says it at the right
	heavy                             string   // what a contact past HeavyContext carries, as the row stamps it
}

// headOf is the first row of a terminal in the projects as read: the
// process its pane was opened on, which everything else in that pane
// hangs under. It is what the bay's mark goes on and what the cursor
// belongs on once the pane is reached, and both ask here so that the
// two can never disagree about which row the pane is. It answers the
// row's place in the reading too, for the cursor to hold.
func headOf(projects []work.Project, tty string) (pid, at int, ok bool) {
	if tty == "" {
		return 0, 0, false
	}
	// The head is the row that stands shallowest in the tree the
	// terminal was read as: a contact under the shell that runs it is
	// not the head of their terminal, the shell is.
	i, best, bestAt, bestDepth := 0, 0, 0, -1
	for _, pl := range projects {
		for _, e := range pl.Entries {
			if e.TTY == tty && (bestDepth < 0 || e.Depth < bestDepth) {
				best, bestAt, bestDepth = e.PID, i, e.Depth
			}
			i++
		}
	}
	return best, bestAt, bestDepth >= 0
}

// composeProcesses words the projects; panes says which terminals are
// the server's, bay which of them is on the right, and isProject which
// directories are projects, for the blocks to nest by.
//
// A pane holds a whole tree, and all of it is equally in the bay, but
// saying so on every row of it paints a block rather than a mark. Only
// the head of that tree is marked shown. What hangs under it reads as
// what it is: in a pane conn holds, like any other row conn can reach.
// worked is the projects something is up in. A project's rows can all
// be down — everything its .conn declares, every service a compose
// would bring up and every service brew holds, with nothing of it
// running — and rows like that are a reading of the file rather than of
// the machine: a list of what could be started, which is what the file
// already is. The project is left out. Opening anything in it brings it
// back, with what it declares under it, down, which is where a
// declaration is worth seeing: beside work already happening.
//
// A project whose .conn would not read stays whatever its rows are.
// That is a fault, and a fault is to be answered.
func worked(projects []work.Project) []work.Project {
	out := make([]work.Project, 0, len(projects))
	for _, pl := range projects {
		up := slices.ContainsFunc(pl.Entries, func(e work.Entry) bool { return e.Status != work.StatusDown })
		if up || pl.Note != "" {
			out = append(out, pl)
		}
	}
	return out
}

// heavy is the figure a contact's row is stamped with when what it
// carries is past claude.HeavyContext, and nothing otherwise.
func heavy(e work.Entry) string {
	if e.Kind != work.KindContact || e.Carried <= claude.HeavyContext {
		return ""
	}
	return tokens(e.Carried)
}

// carriedWord is the figure a contact carries, as the panel says it at
// a row's right, and nothing for any other row or a contact with no
// turn read yet.
func carriedWord(e work.Entry) string {
	if e.Kind != work.KindContact || e.Carried <= 0 {
		return ""
	}
	return tokens(e.Carried)
}

func composeProcesses(projects []work.Project, panes map[string]room.Pane, bay string, roots []string, isProject func(string) bool, home string, now time.Time, err string, stalled bool, filed bool) processesReport {
	b := processesReport{err: err, stalled: stalled, filed: filed}
	head, _, marked := headOf(projects, bay)
	for _, pl := range projects {
		bp := projectBlock{path: pl.Path, note: pl.Note}
		for _, e := range pl.Entries {
			bp.rows = append(bp.rows, processRow{
				pid: e.PID, kind: e.Kind, command: activityOf(e), tty: e.TTY, since: work.SinceWord(e.Since, now),
				status: e.Status, fault: e.Fault, reach: panes[e.TTY].ID,
				shown: marked && e.PID == head, depth: e.Depth,
				over:    e.Declared != "" && panes[e.TTY].Exit != "",
				name:    rowName(e),
				age:     waitedFor(e, now),
				stands:  panelKind(e),
				ports:   e.Ports,
				heavy:   heavy(e),
				carried: carriedWord(e),
			})
		}
		b.projects = append(b.projects, bp)
	}
	if filed {
		b.projects = flat(b.projects, roots, home)
	} else {
		b.projects = nested(b.projects, isProject, roots, home)
	}
	numbered(b.projects)
	return b
}

// numbered gives the first ten contacts their digits, 0 to 9, counted
// down the blocks as they will be drawn, which is not the order of the
// reading: the panel sorts its projects by path, and the tree nests
// them. The digit on a row and the key that goes to it are read off
// the same count, so the two cannot disagree.
func numbered(blocks []projectBlock) {
	n := 0
	for i := range blocks {
		for j := range blocks[i].rows {
			r := &blocks[i].rows[j]
			if r.kind != work.KindContact || n > 9 {
				continue
			}
			r.num = strconv.Itoa(n)
			n++
		}
	}
}

// flat is the blocks as the panel has them: the projects alone, in the
// order of their paths, each named by its own directory. A block with
// no rows is dropped — a project is on the panel because work is
// happening in it, and a heading with nothing under it is a name for
// nothing.
//
// The tree nests a project under the project that holds it, as a folder
// of checkouts holds its repositories, and the panel does not. The
// folder is not where any work is happening; it is a place on a disk
// two of the projects share, and standing it over them cost a row and
// an indent to say so. The panel is the projects work is happening in,
// each at the margin, and the tree on z is where what holds what is
// read.
func flat(blocks []projectBlock, roots []string, home string) []projectBlock {
	var out []projectBlock
	for _, bp := range blocks {
		if len(bp.rows) > 0 {
			out = append(out, bp)
		}
	}
	slices.SortStableFunc(out, func(a, b projectBlock) int { return strings.Compare(a.path, b.path) })
	names := leafNames(out, roots, home)
	for i := range out {
		if out[i].path == "" {
			out[i].path = "NO PROJECT"
			continue
		}
		out[i].path = names[out[i].path]
	}
	return out
}

// leafNames names each project on the panel by its own directory's
// name, the leaf, which is what a project is called; the folder above
// it is put before it only where two projects on the panel share the
// name, and one more above that where they still do, which is as much
// of the path as it takes to tell them apart and no more. A name that
// would take the whole path is written as the tree titles a block,
// from a root or from ~; a root itself is its leaf.
func leafNames(blocks []projectBlock, roots []string, home string) map[string]string {
	segs := map[string]int{}
	for _, bp := range blocks {
		if bp.path != "" {
			segs[bp.path] = 1
		}
	}
	name := func(path string, n int) (string, bool) {
		parts := strings.Split(strings.Trim(filepath.ToSlash(path), "/"), "/")
		if n >= len(parts) {
			if slices.Contains(roots, path) {
				return filepath.Base(path), false
			}
			return projectName(path, roots, home), false
		}
		return strings.Join(parts[len(parts)-n:], "/"), true
	}
	names := map[string]string{}
	for {
		byName := map[string][]string{}
		for path, n := range segs {
			names[path], _ = name(path, n)
			byName[names[path]] = append(byName[names[path]], path)
		}
		grown := false
		for _, paths := range byName {
			if len(paths) < 2 {
				continue
			}
			for _, path := range paths {
				if _, more := name(path, segs[path]); more {
					segs[path]++
					grown = true
				}
			}
		}
		if !grown {
			return names
		}
	}
}

// waitedFor is how long a waiting row has waited, for the panel to say
// at its right; nothing for a row that is not waiting, or whose moment
// is not known.
func waitedFor(e work.Entry, now time.Time) string {
	if e.Status != work.StatusWaiting || e.Since.IsZero() {
		return ""
	}
	return work.Minutes(now.Sub(e.Since))
}

// nested is the blocks as they draw: each under the project that holds
// its directory, where one does, with a heading made for a holding
// project nothing runs in. A conn that has not been told where the
// work is has no projects to nest by, and its blocks stand at the
// margin. A holding project is one the directory
// above the block's is, which a folder of checkouts under a root is
// and a root is not, so the nesting stops where the list's grouping
// does. The blocks came in the order of their paths, and a block goes
// under its holder in that order; a path that sorts between a holder
// and what it holds, as a hyphen does against a slash, follows the
// whole of them rather than splitting them. A block at the margin is
// titled by its name from the root, and a nested one by its path from
// the block above it, the rest being said there.
//
// A heading made for a holder nothing runs in is drawn only where it
// groups: two or more blocks under it. One block alone under such a
// folder stands where the heading would have, titled by its whole name
// from the root — w0zro/conn, not w0zro over conn — since a heading
// over one thing says nothing the thing's own name does not.
func nested(blocks []projectBlock, isProject func(string) bool, roots []string, home string) []projectBlock {
	at := map[string]int{}
	for i, bp := range blocks {
		at[bp.path] = i
	}
	made := len(blocks) // headings made below have no rows of their own
	holder := func(path string) string {
		if path == "" || isProject == nil {
			return ""
		}
		if dir := filepath.Dir(path); isProject(dir) {
			return dir
		}
		return ""
	}
	// Headings for the holders that have no block of their own, on up
	// to the margin.
	for i := 0; i < len(blocks); i++ {
		for h := holder(blocks[i].path); h != ""; h = holder(h) {
			if _, ok := at[h]; ok {
				break
			}
			at[h] = len(blocks)
			blocks = append(blocks, projectBlock{path: h})
		}
	}
	under := map[string][]int{}
	var top []int
	for i, bp := range blocks {
		if h := holder(bp.path); h != "" {
			under[h] = append(under[h], i)
		} else {
			top = append(top, i)
		}
	}
	// Made headings went on the end; each level draws in path order.
	// above is the block drawn over this level, which a nested block is
	// titled from.
	byPath := func(a, b int) int { return strings.Compare(blocks[a].path, blocks[b].path) }
	out := make([]projectBlock, 0, len(blocks))
	var walk func(idx []int, nest int, above string)
	walk = func(idx []int, nest int, above string) {
		slices.SortFunc(idx, byPath)
		for _, i := range idx {
			bp := blocks[i]
			held := under[bp.path]
			if i >= made && len(held) < 2 {
				// A heading over one thing is not drawn: the thing takes
				// its place, at this level, under what is above.
				walk(held, nest, above)
				continue
			}
			bp.nest = nest
			switch {
			case above != "":
				bp.path = relName(above, bp.path)
			case bp.path == "":
				bp.path = "NO PROJECT"
			default:
				bp.path = projectName(bp.path, roots, home)
			}
			out = append(out, bp)
			walk(held, nest+1, blocks[i].path)
		}
	}
	walk(top, 0, "")
	return out
}

// activityOf is what a row's middle column says: for a working contact
// the tool it has in flight, for a shell whose rows are folded what it
// runs, and for anything else its command as typed, whose arguments
// are what it is doing. A contact with nothing
// in flight says its command, which reads as the intelligence
// composing.
func activityOf(e work.Entry) string {
	if e.Doing != "" {
		return e.Doing
	}
	if e.Under != "" {
		return e.Under
	}
	return e.AsTyped()
}

// rowName is the name a row goes by in place of its command, where it
// has one: a declaration's declared name, and a contact's session
// title while the contact is not working. A working contact's row
// says what it is doing, which is the one thing about it that changes.
func rowName(e work.Entry) string {
	if e.Kind == work.KindContact && e.Doing == "" {
		return e.Title
	}
	return declaredNameOf(e)
}

// rowLabel is what the panel calls a row: its name where it has one,
// and what it is doing where it has not.
func rowLabel(e work.Entry) string {
	if name := rowName(e); name != "" {
		return name
	}
	return activityOf(e)
}

// declaredNameOf is the name a declared row goes by, where it is one:
// what the panel calls it, the command being in the file and on the
// page. Anything else has none.
func declaredNameOf(e work.Entry) string {
	if _, name, ok := declared.Unmark(e.Declared); ok {
		return name
	}
	return ""
}

// projectName is what the processes view writes over a block: what is
// left of the path once the root the checkouts are kept under is taken
// off it. ~/projects/w0zro/conn is w0zro/conn. The root is the same for
// every project shown and says nothing that tells one from another, and
// it is said at the head of every block — the panel is forty-four
// columns wide, and the part that tells them apart is the part that
// should have them.
//
// A project outside every root is written from ~ and whole: there is
// nothing shared to take off it, and where it is is the only thing the
// line has to say. A root itself is written the same way, since what is
// left of it after itself is nothing.
func projectName(path string, roots []string, home string) string {
	for _, root := range roots {
		if path != root && work.Within(path, root) {
			return relName(root, path)
		}
	}
	return config.Tilde(path, home)
}

// The processes view's columns, from the right: the status flush with
// the measure, the time in that status and the terminal before it, and
// the command taking what is left after the kind. Under minCols the
// view is a panel, and the command is the column that tells rows
// apart, so the panel gives it what the others can spare: the terminal
// and the time go, since the page beside the panel carries both; the
// status column is as wide as the widest word on it rather than the
// console's widest, with a floor at WORKING so the column holds still
// as words come and go; and a declared row says its name, the command
// being in the file and on the page.
const (
	kindW        = 8
	ttyW         = 10
	sinceW       = 5
	panelKindW   = 8
	panelStatusW = 7 // WORKING, WAITING, STOPPED: the floor the column holds at
	treeIndent   = 2 // columns a row gives up per level under its root
)

// panelStatusWidth is the status column as the panel sizes it: the
// widest word on it, a chip's two cells of padding counted, and never
// under the floor.
func panelStatusWidth(b processesReport) int {
	w := panelStatusW
	for _, bp := range b.projects {
		for _, r := range bp.rows {
			n := ansi.StringWidth(r.status)
			if r.fault || r.status == work.StatusWaiting {
				n += 2
			}
			if r.heavy != "" {
				n = max(n, ansi.StringWidth(r.heavy)+2)
			}
			w = max(w, n)
		}
	}
	return w
}

// drawProcesses renders the processes view for a terminal of the given
// size, with the cursor on the row of the given pid.
func drawProcesses(b processesReport, cursor int, width, height int, p draw.Palette) []draw.Row {
	// Nothing to list is drawn the same way whichever drawing is up,
	// and so is drawn before the choice between them: the fold and the
	// tree differ in how rows are arranged under their projects, and
	// here there are no rows. The panel — the fold, which is what conn
	// shows — drew nothing at all in this case, so a machine with
	// nothing running on it and a panel that had failed to draw looked
	// alike, and a process table that could not be read said so only in
	// the tree, which is the drawing nobody is on when it happens.
	if b.err != "" || len(b.projects) == 0 {
		return drawNoRows(b, width, height, p)
	}
	if b.filed {
		return drawFiled(b, cursor, width, height, p)
	}
	panel := width < draw.MinCols
	width = max(width, draw.PanelMinCols)
	measure := draw.MeasureAt(width)
	c := draw.Canvas{P: p, Width: width}
	statusCol := measure - console.StatusW
	sinceCol := statusCol - 1 - sinceW
	ttyCol := sinceCol - 1 - ttyW
	commandW := ttyCol - 1 - kindW
	kindCol := kindW
	if panel {
		statusCol = measure - panelStatusWidth(b)
		ttyCol, sinceCol = -1, -1
		kindCol = panelKindW
		commandW = statusCol - 1 - kindCol
	}

	// No rule and no column heads. The view goes unlabeled — it is what
	// conn is when it is up — and its columns were labeled anyway, which
	// is the same furniture one level down. Four heads named a table the
	// eye learns in a glance: a kind word, what the process is doing, how
	// long, and how it stands. ACTIVITY was eight columns of label over
	// fourteen columns of column, which is a sign the label is the thing
	// being read. They cost three rows off the top of a panel that has
	// forty, and the first project's name says more than all four.
	//
	// What is left at the top is the first project's name, a row down, and
	// that row is the point: the heads and their rule were furniture and
	// sat flush against the pane's edge, where furniture belongs. A name
	// is the first thing there is to read, and reading does not start
	// hard against an edge.

	// The projects, in the order of their paths; or the reason there
	// are none.
	room := height
	if height == 0 {
		room = 1 << 30
	}
	var body []draw.Row
	cursorRow := -1
	project := func(bp projectBlock) {
		d := draw.Canvas{P: p, Width: width}
		// A row of air before each block at the margin, the first
		// included. Furniture can sit on the edge of a pane — a rule is
		// an edge, and the head row that used to be here was flush for
		// that reason. A project's name is not furniture, it is the
		// first thing there is to read, and a thing to be read does not
		// start hard against the top of the pane. It is the only air
		// there is: a name is followed straight by what is under it, and
		// a blank row means a new thing begins. A block nested under
		// another is not a new thing but part of the one above it, and
		// its indent already sets it apart; air before each of them
		// spread a folder of three repositories over half a panel.
		if bp.nest == 0 {
			d.Blank(0)
		}
		// The title alone. It carried a count of its rows on the right,
		// which was the kernel's word for them and a figure the operator
		// never asks for: the rows are right there under it. Every title
		// is in the parchment and bold, at the margin or nested: a
		// nested one was in the ink, the indent alone telling it from a
		// row, and a folder of repositories read as a wall of rows with
		// nothing to catch the eye between one block and the next. The
		// indent says what is under what; the weight says what is a name.
		l := d.Line()
		titleIn := 0
		if bp.nest > 0 {
			titleIn = min(bp.nest*treeIndent, max(commandW-4, 0))
		}
		l.To(titleIn)
		l.Add(p.Parchment+p.Bold, draw.Fit(bp.path, measure-titleIn, true))
		d.Emit(l, 0, false)
		for _, r := range bp.rows {
			l := d.Line()
			l.PID = r.pid
			cursored := r.pid == cursor
			// What conn can do with a row is said two ways, and they are
			// not the same kind of saying. Dimming is a rank the whole
			// row drops: what conn can only report — a terminal it did
			// not open, and cannot attach to — goes faint in every
			// column, since the rest of them are the quiet gray already
			// and dimming one of six says nothing. Outside its server
			// conn holds nothing, so nothing is dimmed: the distinction
			// would be every row.
			//
			// The bay is a mark, and one cell of one row is all a mark
			// needs: the kind of the head of what is in the bay, in the
			// orange, which is "you, here" everywhere else in conn. A
			// row is a lot of orange, and the status column especially
			// is not the orange's to take — WAITING is already a color
			// close to it, and the two together say neither.
			kind, command, ttyColor, sinceColor, word := p.Gray, p.Ink, p.Gray, p.Gray, p.Gray
			switch {
			case r.shown:
				kind = p.Orange + p.Bold
			case b.inside && (r.reach == "" || r.over):
				// A row conn can only report, or a declared process
				// that has ended and holds its pane for its output:
				// reachable, and over.
				dim := p.Faint
				if cursored {
					// Faint on the raised ground is barely there. The row
					// under the cursor is the one being read, so it gives
					// up a rank of the dimming rather than the reading.
					dim = p.Gray
				}
				kind, command, ttyColor, sinceColor, word = dim, dim, dim, dim, dim
			}
			if cursored {
				// The row under the cursor is the one on the raised ground,
				// edge to edge; where there is no color to raise it, it takes
				// a mark in the margin instead.
				l.P = p.Chosen()
				if p.Plain {
					l.Mark = "▸"
				}
				command += p.Bold
				cursorRow = len(body) + len(d.Rows)
			}
			// A row indents under its block's title, and under the row
			// that runs it, kind and command shifted in together; the
			// command gives up what the indent takes; a tree too deep for
			// the room there is stops taking more.
			indent := min((bp.nest+1+r.depth)*treeIndent, max(commandW-4, 0))
			// The digit that goes to a contact stands in the indent, a
			// column of air before the kind word, which every row has.
			if b.digits && r.num != "" && indent >= 2 {
				l.To(indent - 2)
				l.Add(p.Faint+p.Dim, r.num)
			}
			l.To(indent)
			l.Add(kind, draw.Fit(r.kind, kindCol-1, false))
			l.To(kindCol + indent)
			activity := r.command
			if panel && r.name != "" {
				activity = r.name
			}
			l.Activity(command, p.Gray, activity, r.ports, commandW-indent)
			if !panel {
				l.To(ttyCol)
				l.Add(ttyColor, draw.Fit(strings.ToUpper(r.tty), ttyW, false))
				l.To(sinceCol)
				l.Add(sinceColor, r.since)
			}
			switch {
			case r.fault:
				l.To(measure - ansi.StringWidth(r.status) - 2)
				l.Add(p.Chip, " "+r.status+" ")
			case r.status == work.StatusWaiting:
				// The one word here that asks something of you, and the
				// only one worth finding without looking. It is stamped
				// the way the console stamps a fault and the status line
				// stamps the keys: a block of the orange with the word
				// knocked out of it. A block is not read but seen, and
				// the thing that wants you should be seen before it is
				// read.
				//
				// And it blinks, on the console's own cadence and off
				// the same turn, which is the one thing on a screen that
				// reaches the corner of an eye. Reading down a list of
				// rows that all say something, the row that wants you is
				// the row that moves. A fault beside it wears the same
				// stamp and holds still, which is the difference between
				// a thing to look at and a thing to answer.
				//
				// On the dark half the cells are the ground and nothing
				// around them moves, the way the console's verdict goes
				// dark: a word that jumped its neighbours about would be
				// worse than one that never blinked.
				if b.lit {
					l.To(measure - ansi.StringWidth(r.status) - 2)
					l.Add(p.Chip, " "+r.status+" ")
				}
			// A contact carrying more than is good for it is stamped with
			// the figure where a fault's word goes, and blinks with the
			// waiting word: it is something to do something about, a
			// session to end or compact, and a figure that held still all
			// afternoon would stop being seen. Waiting outranks it, being
			// a question, and the figure is back when the question is
			// answered.
			case r.heavy != "":
				if b.lit {
					l.To(measure - ansi.StringWidth(r.heavy) - 2)
					l.Add(p.Chip, " "+r.heavy+" ")
				}
			default:
				l.To(measure - ansi.StringWidth(r.status))
				l.Add(word, r.status)
			}
			d.Emit(l, 0, false)
		}
		// What is wrong with the project's .conn, under its rows, as a
		// fault is stamped: the file was written to be read, and a
		// project that shows none of what it declares should say why.
		if bp.note != "" {
			l := d.Line()
			in := min((bp.nest+1)*treeIndent, max(commandW-4, 0))
			l.To(in)
			l.Add(p.Chip, " "+draw.Fit(strings.ToUpper(bp.note), measure-in-2, false)+" ")
			d.Emit(l, 0, false)
		}
		body = append(body, d.Rows...)
	}
	for _, bp := range b.projects {
		project(bp)
	}
	c.Rows = append(c.Rows, draw.Scrolled(body, cursorRow, room-len(c.Rows), width, p)...)

	c.Rows = append(c.Rows, notes(b, width, measure, p)...)

	// The ground fills what the rows do not: the keys are learned once,
	// and a legend on every row of every reading is a thing to read
	// past forever.
	if height > 0 {
		for len(c.Rows) < height {
			c.Blank(0)
		}
	}
	return c.Rows
}

// drawNoRows is the view where there is nothing to list: why the table
// could not be read, or, where it read and held nothing, that nothing
// is running. Either is a line at the middle of the panel, with
// whatever the notes have to say under it.
func drawNoRows(b processesReport, width, height int, p draw.Palette) []draw.Row {
	width = max(width, draw.PanelMinCols)
	c := draw.Canvas{P: p, Width: width}
	c.Blank(0)
	l := c.Line()
	if b.err != "" {
		l.Add(p.Chip, " "+strings.ToUpper(b.err)+" ")
	} else {
		l.Add(p.Gray, "NO PROCESSES")
	}
	c.Emit(l, 0, true)
	c.Rows = append(c.Rows, notes(b, width, draw.MeasureAt(width), p)...)
	if height > 0 {
		for len(c.Rows) < height {
			c.Blank(0)
		}
	}
	return c.Rows
}

// notes is what is said under the rows of either drawing of the view:
// docker having gone quiet, and what the server would not do.
func notes(b processesReport, width, measure int, p draw.Palette) []draw.Row {
	c := draw.Canvas{P: p, Width: width}
	// Docker having gone quiet is said under the rows it is about. The
	// services are still listed — what docker last said stands, which is
	// better than dropping them — but a row that may be minutes stale
	// reading as though it were this second is the one thing the view
	// must not do. It is a note and not a fault: nothing is wrong, and
	// there is nothing to answer.
	//
	// It goes here rather than on the status line. The right of that line
	// is empty on purpose, and what was taken off it was a second copy of
	// this list; a word about how these rows were come by belongs beside
	// them, where the eye already is.
	if b.stalled {
		d := draw.Canvas{P: p, Width: width}
		d.Blank(0)
		l := d.Line()
		// Short enough for the panel's own measure, which is what this
		// view is usually read at: a note cut off mid-word says less
		// than no note.
		l.Add(p.Faint, draw.Fit("DOCKER NOT ANSWERING · AS LAST SEEN", measure, false))
		d.Emit(l, 0, false)
		c.Rows = append(c.Rows, d.Rows...)
	}
	// What the server would not do is a fault to be looked at, and is
	// stamped like one, in the server's own words: the tmux command
	// that failed and what it said, which is what the operator would
	// see had they typed it. The processes view is where a key was
	// pressed for it, so it is said here, under the rows.
	// It is wrapped rather than cut: an error cut mid-word is an error
	// nobody can act on, and the panel is narrow.
	if b.notice != "" {
		d := draw.Canvas{P: p, Width: width}
		d.Blank(0)
		for _, part := range draw.WrapValue(strings.ToUpper(b.notice), measure-2) {
			l := d.Line()
			l.Add(p.Chip, " "+part+" ")
			d.Emit(l, 0, false)
		}
		c.Rows = append(c.Rows, d.Rows...)
	}

	return c.Rows
}

// processesKey answers a key in the processes view: q and ctrl+c
// detach in the server and close conn outside it; c brings the console
// back over the whole window, enter reaches the cursor's process, esc
// goes back into the last process the workspace held, s opens a shell
// at its project, a opens claude there instead, and A opens the
// sessions view over what claude left suspended there. tab goes to what
// is waiting on you, longest first, and round again. ? puts the manual
// in the workspace. x asks to end the cursor's process, and arms the
// question rather than the ending: the next key answers it. gg and G
// are the ends of the list, where j and k are its steps: a table long
// enough to scroll is not walked to its end.
//
// came is the pane the panel key just brought the keys out of, for the
// keys that begin a detour.
func (m model) processesKey(k, came string) (model, tea.Cmd) {
	switch k {
	case "ctrl+c", "q":
		return m.leave()
	case "c":
		// The blink is not started here: what annunciates is decided in
		// one place, and the tick follows the view on its own.
		m.view = viewConsole
		if m.inside {
			return m, m.serverCmd(func() error { return m.srv.Wide() })
		}
	case "j", "down":
		m = m.onRow(0, ring(m.cursorAt+1, rowsIn(m.projects)))
	case "k", "up":
		m = m.onRow(0, ring(m.cursorAt-1, rowsIn(m.projects)))
	case "g":
		// Nothing yet: g is the half of a motion, and what it means is
		// decided by the key after it.
		m.firstG = true
	case "G":
		m = m.onRow(0, rowsIn(m.projects)-1)
	case "z":
		// The whole tree, or the fold of it again. The rows are re-made
		// from the reading held, so the change is at once; the cursor
		// keeps its pid where the pid is still shown, and its row
		// otherwise.
		m.full = !m.full
		if len(m.tree) > 0 {
			m.projects = m.tree
			if !m.full {
				m.projects = fold(m.tree)
			}
			m = m.onRow(m.cursor, m.cursorAt)
		}
	case "enter":
		if e, _, ok := m.under(); ok {
			_, cmd := m.enterOn(e)
			return m, cmd
		}
	case "esc":
		return m.backIn()
	case "x":
		if e, _, ok := m.under(); ok {
			m.kill = m.endOn(e)
		}
	case "s":
		// A shell here. On a container, here is inside it: the row stands
		// for a machine of its own, and the directory it was started for
		// is not where its work is going on.
		if e, pl, ok := m.under(); m.inside && ok {
			if e.Container != "" {
				return m, m.shellInContainer(e)
			}
			if pl.Path != "" {
				return m, m.openShell(pl.Path)
			}
		}
	case "S":
		// A session with the server the row is, by its own client: psql
		// on postgres. s beside it is a shell near the server; this is
		// the server itself, talked to.
		if e, pl, ok := m.under(); m.inside && ok {
			if p := m.programUnder(e); p != nil {
				return m, m.openClient(e, p, pl.Path)
			}
		}
	case "o":
		// The row's port in the browser. It asks nothing of the server,
		// so it works whether or not conn holds one: the port is on the
		// row either way, and so is the machine the browser is on.
		if e, _, ok := m.under(); ok && serving(e) {
			return m, m.openServing(e)
		}
	case "a":
		if _, pl, ok := m.under(); m.inside && ok && pl.Path != "" {
			return m, m.startContact(pl.Path)
		}
	case "A":
		// The sessions at the project: the capital of the contact's
		// key, a session being a contact's to pick back up.
		return m.openAt("alt+shift+a", came)
	case "/":
		// What the row's project has written, searched: the slash is
		// the search key everywhere a hand has pressed one. It ends
		// where the keys were before it, as the other detours do.
		if _, pl, ok := m.under(); ok && pl.Path != "" {
			return m.openOutput(pl.Path, came)
		}
	case "tab":
		return m.toWaiting()
	case "1", "2", "3", "4", "5", "6", "7", "8", "9", "0":
		return m.toContact(k)
	case "p":
		// A detour: it ends where the keys were before it, which is the
		// pane the panel key just brought them out of, or nowhere.
		m.from = came
		return m.toProjects()
	case ",":
		// conn's own configuration, in the workspace. The comma is what
		// a program of this shape is settled in everywhere, and it is
		// not a letter the processes view wanted for anything.
		return m.openDetour(toSettings, came)
	case "?":
		return m.openDetour(toManual, came)
	}
	return m, nil
}

// processesReport is the processes view's words as things stand.
func (m model) processesReport() processesReport {
	w := composeProcesses(m.projects, m.panes, m.bay.tty, m.roots.real, m.roots.isProject, m.head.Login.Home, m.now, m.processesErr, m.dockerStalled, !m.full)
	w.inside, w.lit, w.notice = m.inside, m.lit, m.notice
	// A digit is drawn only where pressing it goes somewhere: the keys
	// on the panel, in this view, with no question armed to take them.
	// While the keys are in a process the digit is two keys away, and
	// the rows say nothing a press there would do.
	w.digits = m.view == viewProcesses && m.focused && m.kill == nil
	w.spin = int(m.now.UnixMilli()/spinEvery.Milliseconds()) % len(draw.Spinner)
	return w
}

// click is the mouse pressed on the panel. tmux has the mouse, and
// hands a press in conn's pane on to conn since conn asks for it. A
// press on a row of the processes view puts the cursor on the row and
// goes in, the way enter does; a row with nothing to go into puts its
// readout in the workspace instead. The rows are drawn again to find
// which row was under the press, since the view is drawn from the
// model and the model keeps no picture of it. Anywhere else, and any
// other button, is nothing yet.
func (m model) click(msg tea.MouseClickMsg) (model, tea.Cmd) {
	if m.view != viewProcesses || msg.Button != tea.MouseLeft {
		return m, nil
	}
	rows := drawProcesses(m.processesReport(), m.cursor, m.cols(), m.height, m.p)
	if msg.Y < 0 || msg.Y >= len(rows) || rows[msg.Y].PID == 0 {
		return m, nil
	}
	pid := rows[msg.Y].PID
	m = m.onRow(pid, m.cursorAt)
	if e, _, ok := m.under(); ok {
		if _, cmd := m.enterOn(e); cmd != nil {
			return m, cmd
		}
	}
	return m.keepingPage()
}

// toWaiting goes to the process that has waited longest: the cursor to
// its row, its pane in the bay, and the keys in it, so one press has
// the operator answering. Pressed again from the panel it goes round
// the ring, longest first. It is what tab does in the processes view.
// From another view the processes view is put up on the way, since the
// answer is a pane, and from the console the bay is given its side
// back, a pane being unreachable with the console over the window.
//
// A process conn holds no pane for is still gone to, on the panel, and
// the keys stay where they are.
func (m model) toWaiting() (model, tea.Cmd) {
	round := work.WaitingRound(m.projects)
	if len(round) == 0 {
		return m, nil
	}
	next := round[0]
	for i, e := range round {
		if e.PID == m.cursor {
			next = round[(i+1)%len(round)]
			break
		}
	}
	return m.goTo(next)
}

// toContact goes to the contact whose row carries the digit pressed:
// the first ten contacts down the panel as it draws them, 0 the first,
// as tmux numbers its windows. From inside a process it is the panel
// key and the digit, which is how a window is picked by number in tmux.
// A digit no row carries is nothing.
func (m model) toContact(k string) (model, tea.Cmd) {
	pid := 0
	for _, bp := range m.processesReport().projects {
		for _, r := range bp.rows {
			if r.num == k {
				pid = r.pid
			}
		}
	}
	for _, pl := range m.projects {
		for _, e := range pl.Entries {
			if pid != 0 && e.PID == pid {
				return m.goTo(e)
			}
		}
	}
	return m, nil
}

// goTo puts the cursor on a row and the operator in front of it: the
// panel comes back to the processes view if it is somewhere else, and
// the row's pane goes into the bay with the keys, where conn holds
// one. A row conn only reports is gone to on the panel, and the keys
// stay where they are, there being nothing to put them in.
func (m model) goTo(next work.Entry) (model, tea.Cmd) {
	m = m.onRow(next.PID, m.cursorAt)
	var cmds []tea.Cmd
	if m.view != viewProcesses {
		console := m.view == viewConsole
		m.view, m.entering = viewProcesses, false
		m.processesGen++
		cmds = append(cmds, m.readProcesses())
		if console && m.inside {
			cmds = append(cmds, m.serverCmd(func() error { return m.srv.Narrow() }))
		}
	}
	if m.inside && room.Reachable(m.panes[next.TTY]) {
		cmds = append(cmds, m.reach(m.panes[next.TTY], next.TTY))
	}
	return m, tea.Batch(cmds...)
}

// raiseUnder is u: the row under the cursor brought up, as raiseOn
// says. From the list the row is a project, and the project is what is
// brought up.
func (m model) raiseUnder() (model, tea.Cmd) {
	if m.view != viewProcesses {
		return m.raiseAt()
	}
	if e, _, ok := m.under(); ok {
		return m, m.raiseOn(e)
	}
	return m, nil
}

// under is the entry and the project under the cursor. nextReachable is
// the first process at or after the cursor, round again from the top,
// that conn holds a live pane for: what the bay takes when what was in
// it ends. A hold, the readout and a pane that has died are not
// processes.
func (m model) nextReachable() (work.Entry, bool) {
	var all []work.Entry
	for _, pl := range m.projects {
		all = append(all, pl.Entries...)
	}
	start := 0
	for i, e := range all {
		if e.PID == m.cursor {
			start = i
			break
		}
	}
	for k := range all {
		e := all[(start+k)%len(all)]
		if p := m.panes[e.TTY]; room.Reachable(p) {
			return e, true
		}
	}
	return work.Entry{}, false
}

func (m model) under() (work.Entry, work.Project, bool) {
	for _, pl := range m.projects {
		for _, e := range pl.Entries {
			if e.PID == m.cursor {
				return e, pl, true
			}
		}
	}
	return work.Entry{}, work.Project{}, false
}

// onRow puts the cursor on the row of pid where it is listed, and on
// the row at otherwise, held within the rows there are; see follow.
func (m model) onRow(pid, at int) model {
	m.cursor, m.cursorAt = follow(m.projects, pid, at)
	return m
}

// follow finds the cursor after the rows change: the row of its pid,
// where that is still listed, else the row where it was, held within
// the rows there are. It answers the pid and the row.
func follow(projects []work.Project, pid, at int) (int, int) {
	var pids []int
	for _, pl := range projects {
		for _, e := range pl.Entries {
			pids = append(pids, e.PID)
		}
	}
	if len(pids) == 0 {
		return 0, 0
	}
	for i, p := range pids {
		if pid != 0 && p == pid {
			return p, i
		}
	}
	at = min(max(at, 0), len(pids)-1)
	return pids[at], at
}

// rowsIn is how many process rows the projects hold: what a motion to
// the end or the middle of them counts against. It counts the processes
// and not the titles above them, which is the list j and k walk.
func rowsIn(projects []work.Project) int {
	n := 0
	for _, pl := range projects {
		n += len(pl.Entries)
	}
	return n
}
