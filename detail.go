package main

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// field is one line in the detail pane. Most are a label and a value, but a
// pane also needs to say what it is about and to group what it says: a flat
// list gives the same weight to what a session is doing as to its memory
// share, and leaves the reader to find the difference.
type field struct {
	label string
	value string
	kind  fieldKind
	tone  tone // how the value reads; the zero value is plain

	// lead is the part of the value picked out ahead of it, in a tone of
	// its own: a commit's hash before its subject, a plan entry's mark and
	// name before its command. Empty for most fields.
	lead     string
	leadTone tone
}

type fieldKind int

const (
	pairField    fieldKind = iota // a label and a value
	headingField                  // what the pane is about
	noteField                     // a quieter line under the heading
	gapField                      // a break between groups
	textField                     // a line of a transcript, as it was shown
)

func heading(s string) field { return field{value: s, kind: headingField} }
func note(s string) field    { return field{value: s, kind: noteField} }
func gap() field             { return field{kind: gapField} }
func text(s string) field    { return field{value: s, kind: textField} }

// transcriptLines is how much of a held shell's transcript an inspection reads:
// more than a pane shows, so a tall pane has lines to fill itself with; the
// pane draws the last of them that fit.
const transcriptLines = 60

// detailMsg carries the inspection of whatever the cursor was on when it was
// requested. The key identifies the subject so a slow lookup that lands after
// the cursor has moved on can be discarded.
type detailMsg struct {
	key    string
	fields []field
}

// detailKey identifies the subject of a row, so details can be cached and
// stale results dropped.
func detailKey(r navRow) string {
	if r.kind == rowProc {
		return "proc:" + strconv.Itoa(r.node.PID)
	}
	return "repo:" + r.project.Path
}

// entryState is what a plan entry — or the test run, or a row's own
// shell — is doing: "up" while its command runs, the exit status once it
// ended, and empty for one with no shell. At is when it ended, when the
// pane recorded that; zero otherwise.
type entryState struct {
	State string
	At    time.Time
}

// loadDetail inspects the selected row off the render path. Git and ps are
// fast, but they are still processes, and the UI should not wait on them.
// states is what the place's plan entries are doing, by name; tail reads
// the transcript of the shell a process row is in, when conn holds one,
// nil when it does not; ended is how and when that shell's command ended,
// when it has and the row is the shell at its prompt.
func loadDetail(r navRow, procCount, repoCount int, ag agent, states map[string]entryState, tail func() []string, ended entryState) tea.Cmd {
	key := detailKey(r)
	p := r.project
	switch r.kind {
	case rowProc:
		node, run := r.node, r.run
		return func() tea.Msg {
			fs := procFields(node, run, ag)
			if ended.State != "" {
				fs = append(fs, exitField(ended))
			}
			if tail != nil {
				fs = append(fs, transcript(tail())...)
			}
			return detailMsg{key: key, fields: fs}
		}
	case rowGroup:
		return func() tea.Msg {
			return detailMsg{key: key, fields: groupFields(p, repoCount, procCount, states)}
		}
	}
	return func() tea.Msg {
		return detailMsg{key: key, fields: repoFields(p, procCount, states)}
	}
}

// exitField says how the command a shell was started with ended: well in
// green, and any other way in red, with the status — and how long ago,
// when that was recorded.
func exitField(e entryState) field {
	t := toneBad
	if e.State == "0" {
		t = toneGood
	}
	return field{label: "exited", lead: e.State, leadTone: t, value: ago(e.At), tone: toneQuiet}
}

// ago says how long ago a moment was, or nothing for a moment unknown.
func ago(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return shortAge(at) + " ago"
}

// groupFields describes a group of repositories: where it is, what it holds,
// and the plan its folder carries, if it carries one. Git has nothing to say
// here — the folder is not a repository, which is the point of it.
func groupFields(p Project, repoCount, procCount int, states map[string]entryState) []field {
	fs := []field{
		heading(p.Name),
		note(p.Path),
		gap(),
		{label: "holds", value: plural(repoCount, "repository", "repositories"), tone: toneCount},
		runningField(procCount),
	}
	fs = append(fs, planFields(p.Path, states)...)
	return append(fs, verbFields(p.Path, states)...)
}

// runningField counts what is alive in a place: green when something is,
// receding when nothing is.
func runningField(procCount int) field {
	t := toneGood
	if procCount == 0 {
		t = toneQuiet
	}
	return field{label: "running", value: plural(procCount, "process", "processes"), tone: t}
}

// repoFields describes a repository: where it is, what state its checkout is
// in, and what is running in it.
func repoFields(p Project, procCount int, states map[string]entryState) []field {
	fs := []field{
		heading(p.Name),
		note(p.Path),
		gap(),
	}

	// "branch --show-current" rather than "rev-parse HEAD": a repository with
	// no commits yet still has a branch, and rev-parse fails on it.
	branch, err := git(p.Path, "branch", "--show-current")
	switch {
	case err != nil:
		fs = append(fs, field{label: "git", value: "unavailable: " + err.Error(), tone: toneQuiet})
	case branch == "":
		if sha, e := git(p.Path, "rev-parse", "--short", "HEAD"); e == nil {
			fs = append(fs, field{label: "branch", value: "detached at " + sha, tone: toneAttn})
		}
	default:
		fs = append(fs, field{label: "branch", value: branch, tone: toneAccent})
	}

	if status, err := git(p.Path, "status", "--porcelain"); err == nil {
		// A clean tree is well; changes are the fact worth a glance.
		s := describeStatus(status)
		t := toneAttn
		if s == "clean" {
			t = toneGood
		}
		fs = append(fs, field{label: "status", value: s, tone: t})
	}
	if upstream, err := git(p.Path, "rev-parse", "--abbrev-ref", "@{upstream}"); err == nil {
		// The branch's name is picked out; its standing against the
		// upstream is well when even, and worth a glance when not.
		divergence := describeAheadBehind(p.Path)
		t := toneGood
		if divergence != "" && divergence != "  (in sync)" {
			t = toneAttn
		}
		fs = append(fs, field{label: "upstream", lead: upstream, leadTone: toneAccent,
			value: strings.TrimSpace(divergence), tone: t})
	}
	fs = append(fs, gap())
	if last, e := git(p.Path, "log", "-1", "--format=%h%n%s"); e == nil {
		sha, subject, _ := strings.Cut(last, "\n")
		fs = append(fs, field{label: "last commit", lead: sha, leadTone: toneAccent, value: subject})
		if when, e := git(p.Path, "log", "-1", "--format=%cr  by %an"); e == nil {
			fs = append(fs, field{label: "", value: when, tone: toneQuiet})
		}
	} else if err == nil {
		// A repository git could read, with a branch but nothing on it yet.
		fs = append(fs, field{label: "last commit", value: "none yet", tone: toneQuiet})
	}
	if origin, err := git(p.Path, "remote", "get-url", "origin"); err == nil {
		fs = append(fs, field{label: "origin", value: origin, tone: toneQuiet})
	}

	fs = append(fs, gap(), runningField(procCount))
	fs = append(fs, planFields(p.Path, states)...)
	return append(fs, verbFields(p.Path, states)...)
}

// verbFields is how a place runs each task it says how to run — its
// tests, its build, its lint, as t, b and l would run them — and how the
// last run went: running, ended well, or failed and how, and how long
// ago — read off the task's shell like a plan entry's. A place that says
// nothing of a task has no line for it.
func verbFields(path string, states map[string]entryState) []field {
	var fs []field
	for _, v := range verbs {
		run, _, ok := v.command(path)
		if !ok {
			continue
		}
		mark, word, t := glyphOff, v.idle, toneQuiet
		switch st := states[v.name]; {
		case st.State == "up":
			mark, word, t = glyphOn, "running", toneGood
		case st.State == "0":
			mark, word, t = glyphDone, v.done, toneGood
		case st.State != "":
			mark, word, t = glyphFailed, "failed  exit "+st.State, toneBad
		}
		if when := ago(states[v.name].At); when != "" {
			word += "  " + when
		}
		if len(fs) == 0 {
			fs = append(fs, gap())
		}
		fs = append(fs, field{label: v.label, lead: mark + " " + word, leadTone: t, value: run})
	}
	return fs
}

// planFields is the checklist of what a place says it needs, and which of
// those are up, down, or ended — and how. It is the list r works from, so
// showing it is showing what r would do.
func planFields(path string, states map[string]entryState) []field {
	plan := readPlan(path)
	if len(plan.Entries) == 0 {
		return nil
	}
	fs := []field{gap()}
	for i, e := range plan.Entries {
		label := "needs"
		if i > 0 {
			label = "" // the rest line up under the first
		}
		// An entry that is up glows the way its mark does in the navigator;
		// one that is down recedes with its hollow mark; one whose command
		// ended wears the check in green or the cross, red through, with
		// how it ended. The command reads in ink otherwise: it is what r
		// would run.
		mark, t, vt, value := glyphOff+" ", toneQuiet, tonePlain, e.Run
		switch st := states[e.Name]; {
		case st.State == "up":
			mark, t = glyphOn+" ", toneGood
		case st.State == "0":
			mark, t = glyphDone+" ", toneGood
			value += "   exited 0"
		case st.State != "":
			mark, t, vt = glyphFailed+" ", toneBad, toneBad
			value += "   exited " + st.State
		}
		if when := ago(states[e.Name].At); when != "" && states[e.Name].State != "up" {
			value += ", " + when
		}
		fs = append(fs, field{label: label, lead: mark + e.Name, leadTone: t, value: value, tone: vt})
	}
	return append(fs, field{label: "from", value: plan.Source, tone: toneQuiet})
}

// describeStatus turns porcelain output into a count of what changed.
func describeStatus(porcelain string) string {
	// Only trailing newlines may go: the first two columns are the status
	// itself, and a leading space is what distinguishes " M" (modified in the
	// worktree) from "M " (staged).
	lines := strings.Split(strings.Trim(porcelain, "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return "clean"
	}
	var staged, unstaged, untracked int
	for _, ln := range lines {
		if len(ln) < 2 {
			continue
		}
		switch {
		case strings.HasPrefix(ln, "??"):
			untracked++
		default:
			if ln[0] != ' ' {
				staged++
			}
			if ln[1] != ' ' {
				unstaged++
			}
		}
	}
	var parts []string
	if staged > 0 {
		parts = append(parts, fmt.Sprintf("%d staged", staged))
	}
	if unstaged > 0 {
		parts = append(parts, fmt.Sprintf("%d modified", unstaged))
	}
	if untracked > 0 {
		parts = append(parts, fmt.Sprintf("%d untracked", untracked))
	}
	if len(parts) == 0 {
		return "clean"
	}
	return strings.Join(parts, ", ")
}

// describeAheadBehind reports divergence from the upstream branch, as a
// suffix so it reads as part of the upstream line.
func describeAheadBehind(path string) string {
	counts, err := git(path, "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
	if err != nil {
		return ""
	}
	f := strings.Fields(counts)
	if len(f) != 2 {
		return ""
	}
	ahead, behind := f[0], f[1]
	switch {
	case ahead == "0" && behind == "0":
		return "  (in sync)"
	case behind == "0":
		return "  (" + ahead + " ahead)"
	case ahead == "0":
		return "  (" + behind + " behind)"
	}
	return "  (" + ahead + " ahead, " + behind + " behind)"
}

// procFields describes a running process: what it is, where it runs, and how
// long it has been going.
func procFields(n *ProcNode, run []*ProcNode, ag agent) []field {
	fs := []field{
		heading(procLabel(n)),
		note(n.Dir),
	}

	// What an agent is doing outranks the process table: it is the reason
	// the process is worth looking at, so it comes first.
	if ag != nil {
		fs = append(fs, ag.describe()...)
	}

	fs = append(fs, gap())
	// The navigator folds a run that never branches into the one row, so the
	// shell that started this and anything between them is not on screen
	// anywhere else. This is where it is said.
	if len(run) > 1 {
		fs = append(fs, field{label: "run", value: describeRun(run), tone: toneAccent})
	}
	fs = append(fs, field{label: "parent", value: strconv.Itoa(n.PPID), tone: toneQuiet})

	if argv, err := ps(n.PID, "command="); err == nil && argv != "" {
		fs = append(fs, field{label: "argv", value: argv, tone: toneName})
	}
	fs = append(fs, gap())
	if stats, err := ps(n.PID, "etime=,%cpu=,%mem="); err == nil {
		if f := strings.Fields(stats); len(f) == 3 {
			// Alive reads green; a share of the machine worth a glance
			// reads amber, and an ordinary one as the measure it is.
			fs = append(fs,
				field{label: "uptime", value: f[0], tone: toneGood},
				field{label: "cpu", value: f[1] + "%", tone: shareTone(f[1], 50)},
				field{label: "memory", value: f[2] + "%", tone: shareTone(f[2], 20)},
			)
		}
	}
	if started, err := ps(n.PID, "lstart="); err == nil && started != "" {
		fs = append(fs, field{label: "started", value: started, tone: toneQuiet})
	}
	if state, err := ps(n.PID, "stat="); err == nil && state != "" {
		fs = append(fs, field{label: "state", value: describeState(state), tone: stateTone(state)})
	}

	// Where it is, for the ones that are anywhere: a dev server's row says
	// what it is, and this says what to open.
	//
	// The whole run is asked, not just the process the row is named for. A
	// dev server is a shell running an npm running a node, and it is the node
	// at the bottom that holds the port — the one the fold exists to hide. The
	// row stands for the run, so the run's ports are the row's.
	if ports := runPorts(run, n); len(ports) > 0 {
		fs = append(fs, field{label: "listening", value: strings.Join(ports, ", "), tone: toneAccent})
	}

	if kids := countTree(n) - 1; kids > 0 {
		fs = append(fs, field{label: "children", value: plural(kids, "process", "processes"), tone: toneCount})
	}
	return fs
}

// transcript is the last of what a held shell has shown, as a block of the
// pane: what the process is doing, read without entering it. A shell that
// has shown nothing yet is not a block.
func transcript(lines []string) []field {
	if len(lines) == 0 {
		return nil
	}
	fs := []field{gap(), heading("transcript")}
	for _, l := range lines {
		fs = append(fs, text(l))
	}
	return fs
}

// runPorts is everything the processes a row stands for are listening on,
// as the scan found them.
func runPorts(run []*ProcNode, n *ProcNode) []string {
	nodes := run
	if len(nodes) == 0 {
		nodes = []*ProcNode{n}
	}

	seen := map[string]bool{}
	var ports []string
	for _, node := range nodes {
		for _, p := range node.Ports {
			if !seen[p] {
				seen[p] = true
				ports = append(ports, p)
			}
		}
	}
	sortPorts(ports)
	return ports
}

// describeRun names every process in a folded run, oldest first, so the shell
// the row was started from is the first thing in it.
func describeRun(run []*ProcNode) string {
	parts := make([]string, 0, len(run))
	for _, n := range run {
		parts = append(parts, procLabel(n))
	}
	return strings.Join(parts, " "+glyphJoin+" ")
}

// shareTone is the color a share of the machine reads in: amber from the
// threshold up, where it is the fact worth a glance, and a measure below.
func shareTone(pct string, threshold float64) tone {
	v, err := strconv.ParseFloat(pct, 64)
	if err == nil && v >= threshold {
		return toneAttn
	}
	return toneCount
}

// stateTone is the color a process state reads in: running is alive, a
// zombie or a stop is wrong, and sleeping — most processes, most of the
// time — is the name of a state, and nothing more.
func stateTone(stat string) tone {
	if stat == "" {
		return tonePlain
	}
	switch stat[0] {
	case 'R':
		return toneGood
	case 'T', 'U', 'Z':
		return toneBad
	}
	return toneName
}

// describeState expands the leading character of a ps state code, which is the
// part that says whether the process is doing anything.
func describeState(stat string) string {
	if stat == "" {
		return stat
	}
	names := map[byte]string{
		'R': "running", 'S': "sleeping", 'I': "idle",
		'T': "stopped", 'U': "uninterruptible wait", 'Z': "zombie",
	}
	if name, ok := names[stat[0]]; ok {
		return name + "  (" + stat + ")"
	}
	return stat
}

// countTree counts a node and everything beneath it.
func countTree(n *ProcNode) int {
	total := 1
	for _, c := range n.Children {
		total += countTree(c)
	}
	return total
}

// git runs a git command, reporting what git printed rather than just an exit
// status: "not a git repository" is worth showing, "exit status 128" is not.
//
// Without optional locks: these run in the background on a cadence, and a
// status that took the index lock would collide with the git the user is
// running in the shell beside it. Bounded like the scans, because a git that
// hangs on a large or unhealthy checkout would otherwise pile up behind the
// refresh the same way a hung lsof did.
func git(dir string, args ...string) (string, error) {
	out, err := listing(scanTimeout, "git",
		append([]string{"--no-optional-locks", "-C", dir}, args...)...)
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			if msg := firstLine(string(ee.Stderr)); msg != "" {
				return "", errors.New(strings.TrimPrefix(msg, "fatal: "))
			}
		}
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// firstLine returns the first non-empty line of s.
func firstLine(s string) string {
	for ln := range strings.SplitSeq(s, "\n") {
		if ln = strings.TrimSpace(ln); ln != "" {
			return ln
		}
	}
	return ""
}

func ps(pid int, format string) (string, error) {
	out, err := listing(scanTimeout, "ps", "-p", strconv.Itoa(pid), "-o", format)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}
