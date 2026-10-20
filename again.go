package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/w0zro/conn/internal/tmux"
)

// What a role means in a project is used while it works. What the keys
// start for one — the stack brought up, the checks run, as conn kept
// them or as Claude Code just named them — is watched for a moment
// after it starts, by the question standing in the workspace (see
// offer.go). A run that ends in an error within quickWait has a
// command that does not work here, a script that is gone or a tool not
// on the PATH, more often than a project that is broken; Claude Code is
// asked again, given what failed and what it said, and the answer is
// offered in place of the old, kept on enter under the same name. A
// command that is right and fails fast for the project's own reasons,
// a lint that finds something, is answered as kept, and nothing is
// offered.

// quickWait is how soon after its start a run that fails has failed at
// once.
const quickWait = 2 * time.Second

// A watched is a run the keys started for a role, while it is watched:
// the project it was started for, the name and the command it runs
// under, and when.
type watched struct {
	project string
	stacked
	as role
	at time.Time
}

// A failure is a watched run that failed at once: the pane it stands
// in, the status it ended with, and the role it was run for.
type failure struct {
	stacked
	pane, exit string
	as         role
}

// againPrompt is what Claude Code is asked when runs fail at once: the
// project, each run by its name and command, the status it ended with
// and its last lines, and for a command that works where one does not.
func againPrompt(root string, as role, failed []failure, said map[string][]string) string {
	var b strings.Builder
	b.WriteString(projectDescribed(root))
	what := "long-running processes a developer starts to work on it"
	if as == roleCheck {
		what = "checks a developer runs on it before calling work done"
	}
	fmt.Fprintf(&b, "\n\nThese %s were run at the project's directory and failed at once:", what)
	for _, f := range failed {
		fmt.Fprintf(&b, "\n\n%s: %s\nexited %s", f.name, f.command, f.exit)
		if lines := lastLines(said[f.pane], 40, 4<<10); len(lines) > 0 {
			b.WriteString(", its last lines:\n")
			b.WriteString(strings.Join(lines, "\n"))
		}
	}
	b.WriteString("\n\nFor each whose command is wrong for this project on this machine — a script that is not there, a tool that is not installed or not on the shell's PATH, the wrong directory — give the command that works, one a line, as NAME: COMMAND with the NAME as given, COMMAND as typed into a shell at the project's directory, through the project's own scripts where it has them. Leave out each whose command is right and whose failure is the project's own, as a failing test or a compile error. Nothing else, no code fence. If every command is right, reply KEEP.")
	return b.String()
}

// errKept is Claude Code's answer that every command that failed is
// right.
var errKept = errors.New("kept")

// parseAgain reads the answer to againPrompt: the lines that give a
// command for a name that failed, each under the name it failed as.
// KEEP, or nothing for any of them, is every command kept.
func parseAgain(out string, failed []failure) ([]stacked, error) {
	list, err := parseStack(out)
	if err != nil {
		return nil, errKept
	}
	var fixed []stacked
	for _, s := range list {
		for _, f := range failed {
			if s.name == f.name && s.command != f.command {
				fixed = append(fixed, stacked{name: f.name, command: s.command, root: f.root})
			}
		}
	}
	if len(fixed) == 0 {
		return nil, errKept
	}
	return fixed, nil
}

// askAgain asks about runs that failed at once, off the loop, with what
// each said.
func askAgain(srv *tmux.Server, p project, as role, failed []failure) tea.Cmd {
	return func() tea.Msg {
		lines := map[string][]string{}
		for _, f := range failed {
			lines[f.pane], _ = said(srv, f.pane)
		}
		out, err := askClaude(againPrompt(p.Root, as, failed, lines))
		if err == nil {
			var list []stacked
			list, err = parseAgain(out, failed)
			if err == nil {
				return roleAnswer{root: p.Root, as: as, list: list}
			}
			if errors.Is(err, errKept) {
				return roleAnswer{root: p.Root, as: as, kept: true}
			}
		}
		return roleAnswer{root: p.Root, as: as, err: err.Error()}
	}
}

// names is the names of runs, as a sentence says them.
func names(fs []failure) string {
	var ns []string
	for _, f := range fs {
		ns = append(ns, f.name)
	}
	switch len(ns) {
	case 0:
		return "it"
	case 1:
		return ns[0]
	}
	return strings.Join(ns[:len(ns)-1], ", ") + " and " + ns[len(ns)-1]
}
