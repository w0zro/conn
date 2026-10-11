package main

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/w0zro/conn/internal/config"
	"github.com/w0zro/conn/internal/work"
	"github.com/w0zro/conn/internal/work/declared"
)

// conn ls is the processes of the project the working directory is in,
// as the panel reads them, one a line: the grammar's first word for the
// agents that work a project, which had no address for its processes
// and rebuilt one every session out of ps, lsof and tmux. It reads the
// machine once and starts nothing, so it answers with no server up.

// runLs reads the machine and writes the project of dir.
func runLs(w io.Writer, home, dir string) int {
	h, err := readHere("ls", home, dir)
	if err != nil {
		return fail("ls", err)
	}
	mine := h.projects(h.root)
	// A project nothing runs in is still the answer: its path, and no
	// rows under it.
	if len(mine) == 0 {
		mine = []work.Project{{Path: h.root}}
	}
	writeLs(w, mine, home)
	return 0
}

// realDir is a directory as the process table names it, symlinks and
// all resolved, so it is compared with the rows' own.
func realDir(dir string) string {
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		return real
	}
	return dir
}

// writeLs writes the projects, each under its path, a row a line:
// the pid, the status, the kind, the name it is addressed by, its
// ports, and its command, indented under what runs it.
func writeLs(w io.Writer, projects []work.Project, home string) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for i, pl := range projects {
		if i > 0 {
			fmt.Fprintln(tw)
		}
		fmt.Fprintln(tw, config.Tilde(pl.Path, home))
		if pl.Note != "" {
			fmt.Fprintf(tw, "  %s\n", pl.Note)
		}
		for _, e := range pl.Entries {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s%s\n", pidWord(e), e.Status, strings.ToLower(e.Kind),
				nameOf(e), dash(strings.Join(portsOf(e), ",")), strings.Repeat("  ", e.Depth), e.AsTyped())
		}
	}
	tw.Flush()
}

// nameOf is the name a row is addressed by: the one it is declared
// under, or its program's own.
func nameOf(e work.Entry) string {
	if _, name, ok := declared.Unmark(e.Declared); ok {
		return name
	}
	if e.Brew != "" {
		return e.Brew
	}
	return work.Program(e.Command)
}

// portsOf is what a row listens on: a process's own sockets, or what a
// service publishes on the host.
func portsOf(e work.Entry) []string {
	if len(e.Ports) > 0 {
		return e.Ports
	}
	return work.ListeningPorts(e.Sockets)
}

// pidWord is a row's pid, or - for a row with none: a declaration that
// is down, a container.
func pidWord(e work.Entry) string {
	if e.PID <= 0 {
		return "-"
	}
	return fmt.Sprint(e.PID)
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
