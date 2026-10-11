package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/w0zro/conn/internal/config"
	"github.com/w0zro/conn/internal/room"
	"github.com/w0zro/conn/internal/work"
	"github.com/w0zro/conn/internal/work/brew"
	"github.com/w0zro/conn/internal/work/declared"
	"github.com/w0zro/conn/internal/work/docker"
)

// conn ls is the processes of the project the working directory is in,
// as the panel reads them, one a line: the grammar's first word for the
// agents that work a project, which had no address for its processes
// and rebuilt one every session out of ps, lsof and tmux. It reads the
// machine once and starts nothing, so it answers with no server up.

// runLs reads the machine and writes the project of dir.
func runLs(w io.Writer, home, dir string) int {
	configured, err := config.Roots(home)
	if err != nil {
		fmt.Fprintf(os.Stderr, "conn ls: %v\n", err)
		return 1
	}
	in := sources{uid: os.Getuid(), home: home, configured: configured, containers: docker.Read(), self: os.Getpid()}
	if srv := room.Find(home); srv != nil {
		if _, err := srv.Panes(); err == nil {
			in.srv = srv
		}
	}
	r, err := readMachine(in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "conn ls: the process table could not be read: %v\n", err)
		return 1
	}
	// brew is asked only where a project declares a service of it, as
	// the panel asks it: a question to brew is a second or so.
	if brew.Declared(r.declared) {
		if services, err := brew.Read(); err == nil {
			in.brews, in.declared = services, r.declared
			if again, err := readMachine(in); err == nil {
				r = again
			}
		}
	}
	root := r.rooting.rootOf(realDir(dir))
	if !r.rooting.isProject(root) {
		fmt.Fprintf(os.Stderr, "conn ls: %s is in no project under the roots in config.json\n", config.Tilde(dir, home))
		return 1
	}
	var mine []work.Project
	for _, pl := range r.projects {
		if work.Within(pl.Path, root) {
			mine = append(mine, pl)
		}
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
