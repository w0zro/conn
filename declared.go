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
// the block in the order of the file. Brought up, it runs in a pane of
// conn's server marked as the declaration, and the pane's head row
// carries the declared name; when it ends, the pane holds its output
// and the row reads ENDED, or EXIT n as a fault. A process started by
// hand with the same command is a plain row: the mark is what says a
// pane is the declaration's, and nothing else is guessed at.

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
		if d, ok := was[path]; ok && d.err == "" && d.mod.Equal(info.ModTime()) && d.size == info.Size() {
			out[path] = d
			continue
		}
		d := declared{mod: info.ModTime(), size: info.Size()}
		list, err := readDeclared(path)
		if err != nil {
			d.err = err.Error()
		} else {
			d.list = list
		}
		out[path] = d
	}
	return out
}

// declaredPaths is every project worth asking for a file: the projects
// the reading has blocks for, kept to those that are projects, and
// every project the walk found — so a project nothing is running in
// still gets its block and its down rows.
func declaredPaths(projects []project, walked []projectRow, isProject func(string) bool) []string {
	seen := map[string]bool{}
	var out []string
	add := func(path string) {
		if path != "" && !seen[path] && isProject(path) {
			seen[path] = true
			out = append(out, path)
		}
	}
	for _, pl := range projects {
		add(pl.path)
	}
	for _, r := range walked {
		if r.pid == 0 {
			add(r.path)
		}
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
// tmux is on $TMUX inside the pane, so the option needs no target.
func declaredLine(d declaration, tmux string) string {
	return d.command + "\n" +
		shellQuote(tmux) + " set-option -p @conn_exit \"$?\"\n" +
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
	return "EXIT " + code, true
}

// attachDeclared puts the declarations among the rows. A declaration
// with a pane marked as its own is that pane's head row, relabelled
// with the name and, once the pane has recorded an end, worded by it;
// one without is a down row at the foot of its project's block. A
// project with a file and no block gets one, in its place by path. A
// file that would not read is the block's note.
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
		i := blockAt(&out, path)
		d := declared[path]
		if d.err != "" {
			out[i].note = d.err
			continue
		}
		for _, decl := range d.list {
			mark := markDeclared(path, decl.name)
			if p, ok := byMark[mark]; ok {
				// The pane is up. Its head is relabelled wherever the
				// table filed it; a pane whose rows the table has not
				// got yet has no row this beat, rather than a down row
				// that is not true.
				relabel(out, p, decl)
				continue
			}
			out[i].entries = append(out[i].entries, entry{
				pid: declaredPID(path, decl.name), kind: kindRun,
				command: decl.label(), typed: decl.label(),
				status: statusDown, cwd: decl.at(path), declared: mark,
			})
		}
	}
	return out
}

// blockAt is the index of a project's block, made where there is none:
// in its place among the others by path, which is the order they are
// in, so a block that is only declarations is found where a block with
// work would be.
func blockAt(out *[]project, path string) int {
	for i, pl := range *out {
		if pl.path == path {
			return i
		}
	}
	at := len(*out)
	for i, pl := range *out {
		if pl.path > path {
			at = i
			break
		}
	}
	*out = append(*out, project{})
	copy((*out)[at+1:], (*out)[at:])
	(*out)[at] = project{path: path}
	return at
}

// relabel makes the head row of a declaration's pane say what it is:
// the declared name and command in place of the sh tmux started, its
// kind a run, and its word the exit the pane recorded, where it has.
// The rows are copied before they are written, since the projects
// given may be the model's own.
func relabel(out []project, p pane, decl declaration) {
	for i, pl := range out {
		for j, e := range pl.entries {
			if e.tty != p.tty {
				continue
			}
			rows := make([]entry, len(pl.entries))
			copy(rows, pl.entries)
			e.kind, e.command, e.typed, e.declared = kindRun, decl.label(), decl.label(), p.declared
			if p.exit != "" {
				e.status, e.fault = exitStatus(p.exit)
			}
			rows[j] = e
			out[i].entries = rows
			return
		}
	}
}
