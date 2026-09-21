package main

import (
	"errors"
	"fmt"
	"hash/fnv"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
)

// A project's .conn: the processes it is worked by, written down once
// so that conn can say which of them is not running and bring them up.
// One process a line, name [dir]: command — the name is one word, the
// dir is under the project and is where the pane opens, and the
// command is a shell line, handed to sh as written. A line beginning
// with # is a comment.
//
// A declared process that is not running is a row of the project's
// block in the processes view, dimmed and worded DOWN, at the foot of
// the block in the order of the file — where the project has a block,
// which is where work is happening in it. A project nothing is running
// in shows nothing of its file: the processes view is what is running,
// and a list of everything every project could run is not. Brought up, it runs in a pane of
// conn's server marked as the declaration, and the pane's head row
// carries the declared name; when it ends, the pane holds its output
// and the row reads ENDED, or EXIT n as a fault. A process started by
// hand with the declaration's command, in its directory, is the
// declaration too, wherever it was started: the row carries the name,
// and the declaration is not brought up a second time beside it. The
// mark is how a pane conn opened says whose it is, and the command and
// the directory are how any other row says the same; a row at the
// wrong directory, or with a word of difference, is a plain row.

const declaredName = ".conn"

// A declaration is one line of the file.
type declaration struct {
	name    string
	dir     string // under the project; "" is the project itself
	command string
	line    int // where in the file, for an error found after parsing
}

// A name is one word of letters, digits and the three marks a file
// name is allowed, so it can stand in a tmux option and on a shell line
// without quoting.
var declaredNameOK = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// parseDeclared reads the file's text. A line that will not parse is an
// error naming the line, in the voice conn's own configuration is read
// in: the file is there, somebody meant it to be read, and the reader's
// next move is to open it at that line.
func parseDeclared(text string) ([]declaration, error) {
	var out []declaration
	seen := map[string]bool{}
	for i, line := range strings.Split(text, "\n") {
		n := i + 1
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		head, cmd, ok := strings.Cut(line, ":")
		words := strings.Fields(head)
		if !ok || len(words) == 0 || len(words) > 2 {
			return nil, fmt.Errorf("line %d: want name [dir]: command", n)
		}
		d := declaration{name: words[0], command: strings.TrimSpace(cmd), line: n}
		if !declaredNameOK.MatchString(d.name) {
			return nil, fmt.Errorf("line %d: %q is not a name", n, d.name)
		}
		if len(words) == 2 {
			d.dir = filepath.Clean(words[1])
			if filepath.IsAbs(d.dir) || d.dir == ".." || strings.HasPrefix(d.dir, "../") {
				return nil, fmt.Errorf("line %d: %s is not under the project", n, words[1])
			}
			if d.dir == "." {
				d.dir = ""
			}
		}
		if d.command == "" {
			return nil, fmt.Errorf("line %d: %s runs nothing", n, d.name)
		}
		if seen[d.name] {
			return nil, fmt.Errorf("line %d: %s is declared twice", n, d.name)
		}
		seen[d.name] = true
		out = append(out, d)
	}
	return out, nil
}

// readDeclared reads a project's file. No file is nothing declared and
// no error. A directory a line names that is not there is an error
// like any other line's: tmux told to open a pane at a directory that
// does not exist opens it somewhere else and says nothing, and conn
// will not do that quietly on the operator's behalf.
func readDeclared(project string) ([]declaration, error) {
	path := filepath.Join(project, declaredName)
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	list, err := parseDeclared(string(b))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", declaredName, err)
	}
	for _, d := range list {
		if d.dir == "" {
			continue
		}
		if info, err := os.Stat(filepath.Join(project, d.dir)); err != nil || !info.IsDir() {
			return nil, fmt.Errorf("%s: line %d: %s is not a directory", declaredName, d.line, d.dir)
		}
	}
	return list, nil
}

// at is where a declaration's pane opens.
func (d declaration) at(project string) string {
	return filepath.Join(project, d.dir)
}

// label is how the row names a declaration: the name, and the command
// it stands for.
func (d declaration) label() string {
	return d.name + " · " + d.command
}

// A declared is a project's file as last read: its stamp, so that a
// reading costs one stat and reads again only what changed, and what
// it said — the declarations, or why it could not be read.
type declared struct {
	mod  time.Time
	size int64
	list []declaration
	err  string
	// What each declaration that runs compose would bring up, by the
	// declaration's name, as compose itself says; see composeServices.
	// And the stamp of the compose files it was read from, so that it
	// is asked again only when they change.
	services map[string][]string
	stamps   map[string]string
}

// composeArgs reads a declared command that runs compose: the words
// between compose and up, which are compose's own — a file, a project
// name — and the services named after up, if any. A command that is
// not a compose up is not one.
func composeArgs(command string) (pre, named []string, ok bool) {
	fields := strings.Fields(command)
	if len(fields) < 2 {
		return nil, nil, false
	}
	name := strings.TrimPrefix(fields[0], "docker-")
	if name != "docker" && name != "compose" {
		return nil, nil, false
	}
	i := slices.Index(fields, "up")
	if i < 0 {
		return nil, nil, false
	}
	start := 1
	if name == "docker" && len(fields) > 1 && fields[1] == "compose" {
		start = 2
	}
	if start > i {
		return nil, nil, false
	}
	pre = fields[start:i]
	for _, f := range fields[i+1:] {
		if !strings.HasPrefix(f, "-") {
			named = append(named, f)
		}
	}
	return pre, named, true
}

// composeFilesStamp is the compose files a declaration reads, as they
// stand: the ones compose looks for on its own, and any it was told
// of. A stamp that has not moved is a file that has not changed, and
// compose is not asked again about it.
func composeFilesStamp(dir string, pre []string) string {
	files := []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml",
		"compose.override.yaml", "compose.override.yml", "docker-compose.override.yaml", "docker-compose.override.yml"}
	for i, a := range pre {
		switch {
		case (a == "-f" || a == "--file") && i+1 < len(pre):
			files = append(files, pre[i+1])
		case strings.HasPrefix(a, "--file="):
			files = append(files, strings.TrimPrefix(a, "--file="))
		}
	}
	var b strings.Builder
	for _, f := range files {
		if !filepath.IsAbs(f) {
			f = filepath.Join(dir, f)
		}
		if info, err := os.Stat(f); err == nil {
			fmt.Fprintf(&b, "%s:%d:%d;", f, info.ModTime().UnixNano(), info.Size())
		}
	}
	return b.String()
}

// composeServices is what a compose up would bring up: the services it
// names, or, naming none, every service compose finds in its files —
// asked of compose itself, with the same words the declaration gives
// it, so that every file, override and profile compose would read is
// read the way compose reads it, and conn parses no YAML of its own.
// Without docker there is nothing to ask, and nothing is said.
func composeServices(dir string, pre, named []string) []string {
	if len(named) > 0 {
		return named
	}
	if dockerPath == "" {
		return nil
	}
	args := append([]string{"compose"}, pre...)
	out, err := dockerSaysIn(dir, dockerWait, append(args, "config", "--services")...)
	if err != nil {
		return nil
	}
	var services []string
	for line := range strings.SplitSeq(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			services = append(services, line)
		}
	}
	return services
}

// composeServicesOf fills in what each compose declaration would bring
// up, asking compose only where the files have changed since it was
// last asked.
func composeServicesOf(path string, list []declaration, was declared) (map[string][]string, map[string]string) {
	services, stamps := map[string][]string{}, map[string]string{}
	for _, d := range list {
		pre, named, ok := composeArgs(d.command)
		if !ok {
			continue
		}
		stamp := d.command + "\x00" + composeFilesStamp(d.at(path), pre)
		if s, ok := was.services[d.name]; ok && was.stamps[d.name] == stamp {
			services[d.name], stamps[d.name] = s, stamp
			continue
		}
		services[d.name], stamps[d.name] = composeServices(d.at(path), pre, named), stamp
	}
	return services, stamps
}

// refreshDeclared is the files as they stand now, from what they were:
// a stat per project, a read where the stamp moved, and nothing at all
// for a project with no file, which is not in the answer. A file that
// would not read is read again every time, since what was wrong may be
// beside the file — a directory it named — rather than in it. It
// builds a new map rather than writing into the one the model holds,
// since it runs off the loop.
func refreshDeclared(was map[string]declared, paths []string) map[string]declared {
	out := map[string]declared{}
	for _, path := range paths {
		info, err := os.Stat(filepath.Join(path, declaredName))
		if err != nil {
			continue
		}
		d, kept := was[path]
		stale := !kept || d.err != "" || !d.mod.Equal(info.ModTime()) || d.size != info.Size()
		if stale {
			d = declared{mod: info.ModTime(), size: info.Size()}
			list, err := readDeclared(path)
			if err != nil {
				d.err = err.Error()
			} else {
				d.list = list
			}
		}
		// The compose files are stamped every time, the .conn kept or
		// read: a service added to a compose file is a row to show,
		// and nothing about the .conn says the compose file moved.
		d.services, d.stamps = composeServicesOf(path, d.list, was[path])
		out[path] = d
	}
	return out
}

// declaredPaths is every project worth asking for a file: the projects
// the reading has blocks for, kept to those that are projects, and
// every project conn has already read a file for. A project keeps its
// block once its file has been read, whether or not the table still
// shows work in it: the file says what works the project, and that
// none of it is up is the fact the block stands to say. was is the
// declarations as of the last reading.
func declaredPaths(projects []project, was map[string]declared, isProject func(string) bool) []string {
	seen := map[string]bool{}
	var out []string
	take := func(path string) {
		if path != "" && !seen[path] && isProject(path) {
			seen[path] = true
			out = append(out, path)
		}
	}
	for _, pl := range projects {
		take(pl.path)
	}
	for path := range was {
		take(path)
	}
	sort.Strings(out)
	return out
}

// markDeclared is how a pane says which declaration it was opened for:
// the name, and the project escaped so that the mark carries no space,
// since a pane's fields are told apart by spaces when they are read.
func markDeclared(project, name string) string {
	return name + "@" + url.PathEscape(project)
}

// unmarkDeclared reads a mark back. A name cannot hold @, so the first
// one is the seam.
func unmarkDeclared(mark string) (project, name string, ok bool) {
	name, escaped, ok := strings.Cut(mark, "@")
	if !ok {
		return "", "", false
	}
	project, err := url.PathUnescape(escaped)
	if err != nil {
		return "", "", false
	}
	return project, name, true
}

// declaredPID is the pid a down row has, since the cursor is a pid and
// a row has to have one: below zero, where no process is, below every
// container's, and the same for the declaration on every reading so
// the cursor holds its row. The project is in it, so two projects that
// each declare a web are two rows.
func declaredPID(project, name string) int {
	h := fnv.New32a()
	h.Write([]byte(project))
	h.Write([]byte{0})
	h.Write([]byte(name))
	return -(1<<24 + int(h.Sum32()&0xffffff)) - 2
}

// declaredLine is what runs in a declaration's pane: the command as
// written, then the pane told how it ended, a word of that for the
// operator, and the hold, so the last output stays up to be read. On
// lines of their own, not after semicolons: a comment or an & on the
// end of the operator's line would otherwise take the rest with it.
// tmux tells a pane its own id in TMUX_PANE, and is told it back: a
// client with no terminal is otherwise pointed at whichever pane the
// server counts as current, which is not this one.
func declaredLine(d declaration, tmux string) string {
	return d.command + "\n" +
		shellQuote(tmux) + " set-option -p -t \"$TMUX_PANE\" @conn_exit \"$?\"\n" +
		"printf '\\n[" + d.name + " exited]\\n'\n" +
		holdOpen
}

// exitStatus is the word for a declared process that ended, from what
// its pane recorded: ENDED for a clean end, which is no fault, and the
// code otherwise, which is — the same words a container's end gets.
func exitStatus(code string) (string, bool) {
	if code == "0" {
		return statusEnded, false
	}
	return exitWord + code, true
}

// exitWord is what a status that ended with a code begins with.
const exitWord = "EXIT "

// upAndHeld is what a project already has panes for, by mark: the
// declarations that are up, which a raise passes over, and the panes
// holding one that ended, by the mark, which a raise replaces. It reads
// the rows rather than the panes, since a pane whose rows are not yet
// read is not yet anything.
func upAndHeld(projects []project, panes map[string]pane, path string) (up map[string]bool, held map[string]string) {
	up, held = map[string]bool{}, map[string]string{}
	for _, pl := range projects {
		for _, e := range pl.entries {
			if e.declared == "" {
				continue
			}
			if project, _, ok := unmarkDeclared(e.declared); !ok || project != path {
				continue
			}
			// A brew service is up by brew's word, not by a pane.
			if e.brew != "" {
				if e.status == statusActive {
					up[e.declared] = true
				}
				continue
			}
			// A row with no terminal is a down row, or a declaration
			// started by hand somewhere conn cannot see a terminal for.
			if e.tty == "" {
				if e.status != statusDown {
					up[e.declared] = true
				}
				continue
			}
			if p := panes[e.tty]; p.exit != "" {
				held[e.declared] = p.id
			} else {
				up[e.declared] = true
			}
		}
	}
	return up, held
}

// attachDeclared puts the declarations among the rows. A declaration
// with a pane marked as its own is that pane's head row, relabelled
// with the name and, once the pane has recorded an end, worded by it;
// one without is a down row at the foot of its project's block. A file
// that would not read is the block's note. A project with no block —
// nothing running in it — is given one, holding what it declares,
// down.
func attachDeclared(projects []project, declared map[string]declared, panes map[string]pane) []project {
	if len(declared) == 0 {
		return projects
	}
	byMark := map[string]pane{}
	for _, p := range panes {
		if p.declared == "" {
			continue
		}
		// Two panes for one declaration is a raise that overlapped a
		// reading; the one still running is the one that counts.
		if was, ok := byMark[p.declared]; ok && was.exit == "" {
			continue
		}
		byMark[p.declared] = p
	}
	out := make([]project, len(projects))
	copy(out, projects)
	paths := make([]string, 0, len(declared))
	for path := range declared {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		d := declared[path]
		i := blockOf(out, path)
		if i < 0 {
			// Nothing of the project is running, and it declares what
			// should be: the block stands empty and takes the down rows
			// below. Whether it is listed is not settled here — a
			// project whose every row is down is dropped, and one whose
			// declaration brew holds up for it is not; see worked. What
			// is settled here is that the file has been read against the
			// machine, which is what brew is then asked about. A file
			// that declares nothing and read clean has nothing to stand
			// for.
			if d.err == "" && len(d.list) == 0 {
				continue
			}
			out = append(out, project{path: path})
			i = len(out) - 1
		}
		if d.err != "" {
			out[i].note = d.err
			continue
		}
		for _, decl := range d.list {
			// A brew service is a row of its own kind, as brew reports
			// it, not a pane running the command; see attachBrew.
			if _, ok := brewArgs(decl.command); ok {
				continue
			}
			mark := markDeclared(path, decl.name)
			if p, ok := byMark[mark]; ok {
				// The pane is up. Its head is relabelled wherever the
				// table filed it; a pane whose rows the table has not
				// got yet has no row this beat, rather than a down row
				// that is not true. A service compose would bring up
				// that has no container yet is down under it.
				if pid, ok := headPID(out, p.tty); ok {
					relabel(out, pid, decl, mark, p.exit)
					servicesUnder(out, pid, path, decl, d.services[decl.name])
				}
				continue
			}
			// No pane of conn's is marked as it, but a row running its
			// command at its directory is the declaration started by
			// hand, and the row is relabelled the same way.
			if pid, ok := startedByHand(out[i], path, decl); ok {
				relabel(out, pid, decl, mark, "")
				servicesUnder(out, pid, path, decl, d.services[decl.name])
				continue
			}
			out[i].entries = append(out[i].entries, entry{
				pid: declaredPID(path, decl.name), kind: kindRun,
				command: decl.label(), typed: decl.label(),
				status: statusDown, cwd: decl.at(path), declared: mark,
			})
			// What the declaration would bring up, each a row of its
			// own under it, down: a stack is its services, and a row
			// per service says what conn will hold once it is up and
			// which of them is not, the way the services stand under
			// the stack while it runs.
			for _, svc := range d.services[decl.name] {
				out[i].entries = append(out[i].entries, downService(path, decl, svc, 1))
			}
		}
	}
	return out
}

// downService is a service of a compose declaration as a row, down:
// named by the service alone, at a depth under the declaration's row,
// with a pid of its own to hold the cursor with. It carries no mark
// and no container, since there is nothing to signal, stop or enter;
// bringing the declaration up is what brings it up.
func downService(path string, decl declaration, svc string, depth int) entry {
	return entry{
		pid: declaredPID(path, decl.name+"/"+svc), kind: kindService,
		command: svc, typed: svc, status: statusDown, cwd: decl.at(path), depth: depth,
	}
}

// startedByHand is the row in a project's block that is a declaration
// started by hand: one running the declared command, word for word,
// at the declared directory, and not already a declaration's. The
// shallowest such row is the one, since a command that runs itself
// again under itself is one process to the operator.
func startedByHand(pl project, path string, decl declaration) (pid int, ok bool) {
	command := strings.Join(strings.Fields(decl.command), " ")
	at := filepath.Clean(decl.at(path))
	depth := -1
	for _, e := range pl.entries {
		if e.declared != "" || e.pid <= 0 || e.status == statusDown {
			continue
		}
		if strings.Join(strings.Fields(e.asTyped()), " ") != command || filepath.Clean(e.cwd) != at {
			continue
		}
		if depth < 0 || e.depth < depth {
			pid, depth = e.pid, e.depth
		}
	}
	return pid, depth >= 0
}

// headPID is the pid of the head row of a pane's tree: the first row
// with its terminal, the rows standing in tree order.
func headPID(out []project, tty string) (int, bool) {
	for _, pl := range out {
		for _, e := range pl.entries {
			if e.tty == tty {
				return e.pid, true
			}
		}
	}
	return 0, false
}

// servicesUnder puts a down row under a running declaration's row
// for each service compose would bring up that has no container among
// the rows there: a service that has not started, or that compose
// has not got to yet. The rows are copied before they are written,
// since the projects given may be the model's own.
func servicesUnder(out []project, pid int, path string, decl declaration, services []string) {
	if len(services) == 0 {
		return
	}
	for i, pl := range out {
		for j, e := range pl.entries {
			if e.pid != pid {
				continue
			}
			// The head's subtree, and the services with a row in it.
			end := j + 1
			present := map[string]bool{}
			for end < len(pl.entries) && pl.entries[end].depth > e.depth {
				if pl.entries[end].kind == kindService {
					name, _, _ := strings.Cut(pl.entries[end].command, " · ")
					present[name] = true
				}
				end++
			}
			var missing []entry
			for _, svc := range services {
				if !present[svc] {
					missing = append(missing, downService(path, decl, svc, e.depth+1))
				}
			}
			if len(missing) == 0 {
				return
			}
			rows := make([]entry, 0, len(pl.entries)+len(missing))
			rows = append(rows, pl.entries[:end]...)
			rows = append(rows, missing...)
			rows = append(rows, pl.entries[end:]...)
			out[i].entries = rows
			return
		}
	}
}

// blockOf is the index of a project's block, or below zero where the
// project has none.
func blockOf(out []project, path string) int {
	for i, pl := range out {
		if pl.path == path {
			return i
		}
	}
	return -1
}

// relabel makes a declaration's row say what it is: the declared name
// and command in place of the sh tmux started or the line typed, its
// kind a run, its mark the declaration's, and its word the exit the
// pane recorded, where it has one. The rows are copied before they
// are written, since the projects given may be the model's own.
func relabel(out []project, pid int, decl declaration, mark, exit string) {
	for i, pl := range out {
		for j, e := range pl.entries {
			if e.pid != pid {
				continue
			}
			rows := make([]entry, len(pl.entries))
			copy(rows, pl.entries)
			e.kind, e.command, e.typed, e.declared = kindRun, decl.label(), decl.label(), mark
			if exit != "" {
				e.status, e.fault = exitStatus(exit)
			}
			rows[j] = e
			out[i].entries = rows
			return
		}
	}
}
