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
// after it starts. A run that ends in an error within quickWait has a
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
// the project on the screen when it started, the name and the command
// it runs under, and when.
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

// run starts what a role means and watches it, and reads the panes
// again once the watch is over, so a run still going is let go of even
// where nothing else changes. The head turns while the starts are
// made, and the first pane started takes the cursor and stands in the
// workspace, so the stack is seen coming up.
func (m model) run(list []stacked, as role, record bool) (model, tea.Cmd) {
	now := time.Now()
	for _, s := range list {
		m.watching = append(m.watching, watched{project: m.cur.Root, stacked: s, as: as, at: now})
	}
	m.starting++
	m.note = ""
	return m, tea.Batch(m.startRoles(list, as, record), tea.Tick(quickWait+time.Second, func(time.Time) tea.Msg { return changed{} }))
}

// watch reads what the watched runs have done by a read: each that
// failed at once is a failure, each that ended well or is still going
// past quickWait is let go of, the rest are still watched. Once nothing
// of the project on the screen is watched, its failures are asked about.
func (m model) watch(at time.Time) (model, tea.Cmd) {
	if m.screen != onProject || len(m.watching) == 0 {
		return m, nil
	}
	var still []watched
	var failed []failure
	for _, w := range m.watching {
		if w.project != m.cur.Root {
			still = append(still, w)
			continue
		}
		p, ok := m.runOf(w)
		switch {
		case ok && p.Exit != "" && p.Exit != "0" && p.Ended.Sub(w.at) <= quickWait:
			failed = append(failed, failure{stacked: w.stacked, pane: p.ID, exit: p.Exit, as: w.as})
		case ok && p.Exit != "":
		case at.Sub(w.at) > quickWait:
		default:
			still = append(still, w)
		}
	}
	m.watching = still
	m.failed = append(m.failed, failed...)
	for _, w := range still {
		if w.project == m.cur.Root {
			return m, nil
		}
	}
	if len(m.failed) == 0 || m.question != "" {
		return m, nil
	}
	// One role is asked about at a time; the other's wait for the next
	// read after the question.
	as := m.failed[0].as
	failed, rest := []failure{}, []failure{}
	for _, f := range m.failed {
		if f.as == as {
			failed = append(failed, f)
		} else {
			rest = append(rest, f)
		}
	}
	m.failed = rest
	m.question, m.offer, m.offerErr, m.again = as, nil, "", failed
	return m, askAgain(m.srv, m.cur, as, failed)
}

// runOf is the pane a watched run stands in: the one under its name
// that ended no sooner than it started, where it has ended, or the
// one still running. A pane by the name that ended before is the run
// it replaced, which goes once the new one is up.
func (m model) runOf(w watched) (pane, bool) {
	for _, r := range m.rows {
		if r.pane == nil || r.pane.Name != w.name {
			continue
		}
		if r.pane.Exit == "" || !r.pane.Ended.Before(w.at.Truncate(time.Second)) {
			return *r.pane, true
		}
	}
	return pane{}, false
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
