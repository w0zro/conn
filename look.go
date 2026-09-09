package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The look page: what conn knows about a row, read without entering it.
// i at conn opens it on the row under the cursor in the everything view,
// or on the shown buffer's; ctrl-space i from a buffer does the same for
// that buffer. It is a popup over the window, like the environment page,
// and it says of a process what it is, where it runs, the binary behind
// its name, its arguments, the telling part of its environment, its
// uptime and share of the machine, its ports and children, and the last
// of what its shell has shown; of an agent, what it is doing and what it
// was last asked; of a place, its branch, its standing, its last commit,
// and its plan as a checklist; of a container, its image, its status and
// the tail of its logs. conn reads all of it off the render path, writes
// it beside the socket, and the page draws it.

// field is one line of the page. Most are a label and a value, but a
// page also needs to say what it is about and to group what it says: a
// flat list gives the same weight to what a session is doing as to its
// memory share, and leaves the reader to find the difference.
type field struct {
	Label string    `json:"label,omitempty"`
	Value string    `json:"value,omitempty"`
	Kind  fieldKind `json:"kind,omitempty"`
	Tone  tone      `json:"tone,omitempty"` // how the value reads; the zero value is plain

	// Lead is the part of the value picked out ahead of it, in a tone of
	// its own: a commit's hash before its subject, a plan entry's mark and
	// name before its command. Empty for most fields.
	Lead     string `json:"lead,omitempty"`
	LeadTone tone   `json:"leadTone,omitempty"`
}

type fieldKind int

const (
	pairField    fieldKind = iota // a label and a value
	headingField                  // what the page is about
	noteField                     // a quieter line under the heading
	gapField                      // a break between groups
	textField                     // a line of a transcript, as it was shown
)

func heading(s string) field { return field{Value: s, Kind: headingField} }
func note(s string) field    { return field{Value: s, Kind: noteField} }
func gap() field             { return field{Kind: gapField} }
func text(s string) field    { return field{Value: s, Kind: textField} }

// openLook opens the look page on the row at hand: the cursor's row in
// the everything view, else the shown buffer's. What the page says is
// read off the render path — git, ps, docker and a transcript are each
// a process — then written for the page, which opens on it.
func (m *model) openLook() tea.Cmd {
	if m.server == nil {
		m.status, m.statusErr = "no server to show it on: "+m.serverErr, true
		return nil
	}
	r, ok := m.lookRow()
	if !ok {
		m.status, m.statusErr = "nothing to look at", false
		return nil
	}
	name := ""
	if r.kind == rowProc {
		name = m.rowName(r)
	}
	var agentLines []field
	if ag := m.agentFor(r); ag != nil {
		agentLines = m.agentFields(r, ag)
	}
	load := loadLook(r, name, len(m.placeTrees(r)), len(m.grouped[r.project.Path]),
		agentLines, m.entryStates(r.project.Path), m.tailOf(r), m.ending(r))
	s := m.server
	return func() tea.Msg {
		if err := s.showLook(load()); err != nil {
			return serverErrorMsg{err: err}
		}
		return nil
	}
}

// lookRow is the row i looks at: the cursor's while the everything view
// is up, else the row of the shown buffer.
func (m model) lookRow() (navRow, bool) {
	if m.viewingAll() {
		return m.selected()
	}
	t := m.terms[m.shown]
	if t == nil {
		return navRow{}, false
	}
	for _, r := range m.rows {
		if r.kind == rowProc && m.owningTerm(r.node.PID) == t {
			return r, true
		}
	}
	return navRow{}, false
}

// placeTrees is the process trees a place's row stands for: a group's
// repositories', a repository's own and its sub-projects', a
// sub-project's own.
func (m model) placeTrees(r navRow) []*ProcNode {
	switch r.kind {
	case rowGroup:
		return m.groupTrees(r.project.Path)
	case rowProject:
		return m.repoTrees(r.project.Path)
	case rowSub:
		return m.byPlace[r.project.Path]
	}
	return nil
}

// agentFields is what an agent is doing, read here where the instance
// is: working, blocked on an ask and which, done and waiting, or idle;
// and how long it has waited. What it says of itself deeper — its branch,
// its context — the command reads.
func (m model) agentFields(r navRow, ag agent) []field {
	var fs []field
	if ask, ok := ag.blocked(); ok {
		fs = append(fs, field{Label: "agent", Lead: glyphAsk + " blocked", LeadTone: toneUrgent, Value: ask})
	} else if ag.working() {
		fs = append(fs, field{Label: "agent", Value: "working", Tone: toneGood})
	} else if m.awaiting(r) != nil {
		fs = append(fs, field{Label: "agent", Value: "done; waiting for the next ask", Tone: toneAttn})
	} else {
		fs = append(fs, field{Label: "agent", Value: "idle since it started", Tone: toneQuiet})
	}
	if w, ok := ag.(waited); ok && m.awaiting(r) != nil && w.since() > 0 {
		fs = append(fs, field{Label: "waiting", Value: shortFor(w.since()), Tone: toneAttn})
	}
	return fs
}

// tailOf reads the transcript of the shell a process row is in — the one
// conn holds around it, which is where what the row is named for is
// drawing — or is nothing for a row with no such shell.
func (m model) tailOf(r navRow) func() []string {
	if r.kind != rowProc || m.server == nil {
		return nil
	}
	t := m.owningTerm(r.node.PID)
	if t == nil {
		return nil
	}
	srv, pid := m.server, t.pid
	return func() []string { return srv.tail(pid, transcriptLines) }
}

// loadLook is the page's lines, read when called: git and ps are fast,
// but they are still processes, and the window should not wait on them.
// agentLines is what the model read of an agent; states is what the
// place's plan entries are doing, by name; tail reads the transcript of
// the shell a process row is in, nil when conn holds none; ended is how
// and when that shell's command ended, when it has.
func loadLook(r navRow, name string, procCount, repoCount int, agentLines []field, states map[string]entryState, tail func() []string, ended entryState) func() []field {
	p := r.project
	switch r.kind {
	case rowProc:
		node, run := r.node, r.run
		if node.Container != nil {
			return func() []field { return containerFields(node) }
		}
		return func() []field {
			fs := procFields(node, name, run, agentLines)
			if ended.State != "" {
				fs = append(fs, exitField(ended))
			}
			if tail != nil {
				fs = append(fs, transcript(tail())...)
			}
			return fs
		}
	case rowGroup:
		return func() []field { return groupFields(p, repoCount, procCount, states) }
	case rowRest:
		c := r.rest
		return func() []field { return restFields(c) }
	}
	return func() []field { return repoFields(p, procCount, states) }
}

// restFields is the page for a conversation at rest: the kind that had
// it and how long ago it last moved, the branch, the last thing asked of
// it and what it said it was doing, where it was had, and what enter
// runs to pick it back up.
func restFields(c conversation) []field {
	fs := []field{
		heading(c.Kind + " " + glyphDot + " suspended"),
		note(c.Dir),
		gap(),
		{Label: "suspended", Value: ago(c.When), Tone: toneQuiet},
	}
	if c.Branch != "" {
		fs = append(fs, field{Label: "branch", Value: c.Branch, Tone: toneAccent})
	}
	if c.Prompt != "" {
		fs = append(fs, field{Label: "asked", Value: c.Prompt})
	}
	if c.Summary != "" {
		fs = append(fs, field{Label: "said", Value: c.Summary})
	}
	if run := resumeCommand(c); run != "" {
		fs = append(fs, field{Label: "enter", Value: "continues it: " + run, Tone: toneQuiet})
	}
	return fs
}

// exitField says how the command a shell was started with ended: well in
// green, and any other way in the bad tone, with the status — what the
// transcript said of it, and how long ago, when those are known.
func exitField(e entryState) field {
	t := toneBad
	if e.State == "0" {
		t = toneGood
	}
	return field{Label: "exited", Lead: e.State, LeadTone: t, Value: joinWords(exitWord(e.State), e.Summary, ago(e.At)), Tone: toneQuiet}
}

// groupFields describes a group of repositories: where it is, what it
// holds, and the plan its folder carries, if it carries one. Git has
// nothing to say here — a group's folder is not a repository.
func groupFields(p Project, repoCount, procCount int, states map[string]entryState) []field {
	if p.Path == globalPlace {
		return globalFields(procCount)
	}
	fs := []field{
		heading(p.Name),
		note(p.Path),
		gap(),
		{Label: "holds", Value: plural(repoCount, "repository", "repositories"), Tone: toneCount},
		runningField(procCount),
	}
	return append(fs, planFields(p.Path, states)...)
}

// globalFields is the page for what runs outside every project.
func globalFields(procCount int) []field {
	return []field{
		heading(globalGroup.Name),
		note("what the machine runs outside every project: containers, and what listens"),
		gap(),
		runningField(procCount),
	}
}

// runningField counts what is alive in a place: green when something is,
// receding when nothing is.
func runningField(procCount int) field {
	t := toneGood
	if procCount == 0 {
		t = toneQuiet
	}
	return field{Label: "running", Value: plural(procCount, "process", "processes"), Tone: t}
}

// repoFields describes a repository: where it is, what state its checkout
// is in, and what is running in it.
func repoFields(p Project, procCount int, states map[string]entryState) []field {
	fs := []field{
		heading(p.Name),
		note(p.Path),
		gap(),
	}

	// "branch --show-current" rather than "rev-parse HEAD": a repository
	// with no commits yet still has a branch, and rev-parse fails on it.
	branch, err := git(p.Path, "branch", "--show-current")
	switch {
	case err != nil:
		fs = append(fs, field{Label: "git", Value: "unavailable: " + err.Error(), Tone: toneQuiet})
	case branch == "":
		if sha, e := git(p.Path, "rev-parse", "--short", "HEAD"); e == nil {
			fs = append(fs, field{Label: "branch", Value: "detached at " + sha, Tone: toneAttn})
		}
	default:
		fs = append(fs, field{Label: "branch", Value: branch, Tone: toneAccent})
	}

	if status, err := git(p.Path, "status", "--porcelain"); err == nil {
		// A clean tree is well; changes are the fact worth a glance.
		s := describeStatus(status)
		t := toneAttn
		if s == "clean" {
			t = toneGood
		}
		fs = append(fs, field{Label: "status", Value: s, Tone: t})
	}
	if upstream, err := git(p.Path, "rev-parse", "--abbrev-ref", "@{upstream}"); err == nil {
		// The branch's name is picked out; its standing against the
		// upstream is well when even, and worth a glance when not.
		divergence := describeAheadBehind(p.Path)
		t := toneGood
		if divergence != "" && divergence != "  (in sync)" {
			t = toneAttn
		}
		fs = append(fs, field{Label: "upstream", Lead: upstream, LeadTone: toneAccent,
			Value: strings.TrimSpace(divergence), Tone: t})
	}
	fs = append(fs, gap())
	if last, e := git(p.Path, "log", "-1", "--format=%h%n%s"); e == nil {
		sha, subject, _ := strings.Cut(last, "\n")
		fs = append(fs, field{Label: "last commit", Lead: sha, LeadTone: toneAccent, Value: subject})
		if when, e := git(p.Path, "log", "-1", "--format=%cr  by %an"); e == nil {
			fs = append(fs, field{Label: "", Value: when, Tone: toneQuiet})
		}
	} else if err == nil {
		// A repository git could read, with a branch but nothing on it yet.
		fs = append(fs, field{Label: "last commit", Value: "none yet", Tone: toneQuiet})
	}
	if origin, err := git(p.Path, "remote", "get-url", "origin"); err == nil {
		fs = append(fs, field{Label: "origin", Value: origin, Tone: toneQuiet})
	}

	fs = append(fs, gap(), runningField(procCount))
	return append(fs, planFields(p.Path, states)...)
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
		// An entry that is up glows in the color of its mark in the
		// view; one that is down recedes with its hollow mark; one whose
		// command ended shows the check in green or the cross in the bad
		// tone, with how it ended. The command reads in ink otherwise: it
		// is what r would run.
		mark, t, vt, value := glyphOff+" ", toneQuiet, tonePlain, e.Run
		switch st := states[e.Name]; {
		case st.State == "up":
			// Up, and where: an entry listening says its ports, and a
			// server that is up and listening nowhere is a server that
			// is not ready yet, or never will be.
			mark, t = glyphOn+" ", toneGood
			if len(st.Ports) > 0 {
				value += "   :" + strings.Join(st.Ports, " :")
			}
		case st.State == "0":
			mark, t = glyphDone+" ", toneGood
			value += "   exited 0"
		case st.State != "":
			mark, t, vt = glyphFailed+" ", toneBad, toneBad
			value += "   exited " + st.State
		}
		if st := states[e.Name]; st.State != "up" && st.State != "" {
			if rest := joinWords(exitWord(st.State), st.Summary, ago(st.At)); rest != "" {
				value += ", " + rest
			}
		}
		fs = append(fs, field{Label: label, Lead: mark + e.Name, LeadTone: t, Value: value, Tone: vt})
		fs = append(fs, runsField(path, e.Name, e.Run)...)
	}
	return append(fs, field{Label: "from", Value: plan.Source, Tone: toneQuiet})
}

// runsField is the line under an entry saying how the last runs of its
// command went and how long each took, newest first, for one that has
// run before; nothing for one that has not.
func runsField(path, name, command string) []field {
	runs := pastRuns(path, name, command, runsShown)
	if len(runs) == 0 {
		return nil
	}
	return []field{{Label: "", Value: "runs  " + describeRuns(runs), Tone: toneQuiet}}
}

// describeStatus turns porcelain output into a count of what changed.
func describeStatus(porcelain string) string {
	// Only trailing newlines may go: the first two columns are the status
	// itself, and a leading space is what distinguishes " M" (modified in
	// the worktree) from "M " (staged).
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

// procFields describes a running process: what it is, where it runs, and
// how long it has been going. agentLines is what the model read of an
// agent, when the process is one.
func procFields(n *ProcNode, name string, run []*ProcNode, agentLines []field) []field {
	// Headed with the row's own name — app, npm run dev, claude · opus —
	// and the number after: the page is about what the row says it is.
	fs := []field{
		heading(name + " " + nodeID(n)),
		note(n.Dir),
	}

	// What an agent is doing outranks the process table: it is the reason
	// the process is worth looking at, so it comes first.
	if len(agentLines) > 0 {
		fs = append(fs, gap())
		fs = append(fs, agentLines...)
	}

	fs = append(fs, gap())
	// The view folds a run that never branches into the one row, so the
	// shell that started this and anything between them is not on screen
	// anywhere else. This is where it is said.
	if len(run) > 1 {
		fs = append(fs, field{Label: "run", Value: describeRun(run), Tone: toneAccent})
	}
	// The parent, and the group the process was started in — the job,
	// which is what a tree kill covers. One init took in when its parent
	// died is said to be orphaned: it is running on with nothing above it.
	parent, t := strconv.Itoa(n.PPID), toneQuiet
	if n.PGID != 0 {
		parent += " " + glyphDot + " group " + strconv.Itoa(n.PGID)
	}
	if orphaned(n.Proc) {
		parent, t = parent+" "+glyphDot+" orphaned", toneAttn
	}
	fs = append(fs, field{Label: "parent", Value: parent, Tone: t})

	if argv, err := ps(n.PID, "command="); err == nil && argv != "" {
		fs = append(fs, field{Label: "argv", Value: argv, Tone: toneName})
	}
	// Which node, which python: the binary behind the name, where the
	// name alone is the usual confusion. And the environment it was
	// started with, the part that says which environment that is and
	// what it talks to (tellingEnv).
	if bin := binaryOf(n.PID); bin != "" {
		fs = append(fs, field{Label: "binary", Value: homely(bin), Tone: toneQuiet})
	}
	if env := tellingEnv(environOf(n.PID)); len(env) > 0 {
		fs = append(fs, gap())
		for i, kv := range env {
			label := "env"
			if i > 0 {
				label = ""
			}
			fs = append(fs, field{Label: label, Value: kv, Tone: tonePlain})
		}
	}
	fs = append(fs, gap())
	if stats, err := ps(n.PID, "etime=,%cpu=,%mem="); err == nil {
		if f := strings.Fields(stats); len(f) == 3 {
			// Alive reads green; a share of the machine worth a glance
			// reads amber, and an ordinary one as the measure it is.
			fs = append(fs,
				field{Label: "uptime", Value: f[0], Tone: toneGood},
				field{Label: "cpu", Value: f[1] + "%", Tone: shareTone(f[1], 50)},
				field{Label: "memory", Value: f[2] + "%", Tone: shareTone(f[2], 20)},
			)
		}
	}
	if started, err := ps(n.PID, "lstart="); err == nil && started != "" {
		fs = append(fs, field{Label: "started", Value: started, Tone: toneQuiet})
	}
	if state, err := ps(n.PID, "stat="); err == nil && state != "" {
		fs = append(fs, field{Label: "state", Value: describeState(state), Tone: stateTone(state)})
	}

	// Where it is, for the ones that are anywhere: a dev server's row
	// says what it is, and this says what to open. The whole run is
	// asked, not just the process the row is named for: a dev server is
	// a shell running an npm running a node, and it is the node at the
	// bottom that holds the port.
	if ports := runPorts(run, n); len(ports) > 0 {
		fs = append(fs, field{Label: "listening", Value: strings.Join(ports, ", "), Tone: toneAccent})
	}

	if kids := countTree(n) - 1; kids > 0 {
		fs = append(fs, field{Label: "children", Value: plural(kids, "process", "processes"), Tone: toneCount})
	}
	return fs
}

// containerFields describes a container: what it is, where it belongs,
// and what docker says of it, then the last of its logs.
func containerFields(n *ProcNode) []field {
	c := n.Container
	statusTone := toneGood
	if containerWrong([]*ProcNode{n}) {
		statusTone = toneBad
	} else if !c.running() {
		statusTone = toneQuiet
	}
	where := n.Dir
	if where == globalPlace {
		where = "started outside every project"
	}
	fs := []field{
		heading(procLabel(n)),
		note(where),
		gap(),
		{Label: "container", Value: c.Name, Tone: toneName},
		{Label: "image", Value: c.Image, Tone: toneQuiet},
		{Label: "status", Value: c.Status, Tone: statusTone},
	}
	if len(n.Ports) > 0 {
		fs = append(fs, field{Label: "publishes", Value: strings.Join(n.Ports, ", "), Tone: toneAccent})
	}
	return append(fs, transcript(readContainerLogs(c.ID, transcriptLines))...)
}

// readContainerLogs is the last lines of a container's logs, as docker
// prints them; nothing without docker, or with nothing logged.
func readContainerLogs(id string, lines int) []string {
	if dockerPath == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
	defer cancel()
	out, _ := exec.CommandContext(ctx, dockerPath, "logs", "--tail", strconv.Itoa(lines), id).CombinedOutput()
	said := strings.TrimRight(string(out), "\n")
	if said == "" {
		return nil
	}
	return strings.Split(said, "\n")
}

// transcript is the last of what a held shell has shown, as a block of
// the page: what the process is doing, read without entering it. A shell
// that has shown nothing yet is not a block.
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

// describeRun names every process in a folded run, oldest first, so the
// shell the row was started from is the first thing in it.
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

// describeState expands the leading character of a ps state code, which
// is the part that says whether the process is doing anything.
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

// The page.

// lookPath is where the page's lines are kept, beside the socket.
func lookPath() string {
	return filepath.Join(filepath.Dir(socketPath()), "look.json")
}

// writeLook writes the lines for the page.
func writeLook(fields []field) error {
	b, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	path := lookPath()
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// readLook reads them.
func readLook() ([]field, error) {
	b, err := os.ReadFile(lookPath())
	if err != nil {
		return nil, err
	}
	var fs []field
	err = json.Unmarshal(b, &fs)
	return fs, err
}

// lookPopupWidth and lookPopupHeight are the room the page asks for; a
// smaller client cuts it to the client.
const lookPopupWidth, lookPopupHeight = 110, 40

// showLook keeps the page's lines for the page and opens it over the
// client that spoke last.
func (s *session) showLook(fields []field) error {
	if err := writeLook(fields); err != nil {
		return err
	}
	return popup(s.run, "", "", lookPopupWidth, lookPopupHeight, shellQuote(connExe())+" page look")
}

// lookModel is the page as a program: the lines, drawn in blocks, the
// window over them scrolling with j, k and the arrows, gone on q or esc.
type lookModel struct {
	fields []field
	err    error
	loaded bool
	offset int
	width  int
	height int
}

type lookReadMsg struct {
	fields []field
	err    error
}

func newLookModel() lookModel {
	return lookModel{width: lookPopupWidth, height: lookPopupHeight}
}

func (m lookModel) Init() tea.Cmd {
	return func() tea.Msg {
		fs, err := readLook()
		return lookReadMsg{fields: fs, err: err}
	}
}

func (m lookModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case lookReadMsg:
		m.fields, m.err, m.loaded = msg.fields, msg.err, true
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c", "i":
			return m, tea.Quit
		case "down", "j":
			m.offset++
		case "up", "k":
			m.offset--
		case "space", "pgdown", "ctrl+d":
			m.offset += max(m.height-2, 1)
		case "pgup", "ctrl+u":
			m.offset -= max(m.height-2, 1)
		case "g":
			m.offset = 0
		case "G":
			m.offset = len(m.lines())
		}
		m.offset = max(min(m.offset, max(len(m.lines())-m.height, 0)), 0)
	}
	return m, nil
}

func (m lookModel) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

// lines is the page as drawn: a blank, then each block under a blank.
func (m lookModel) lines() []string {
	switch {
	case !m.loaded:
		return []string{"", gutter + noteStyle.Render("looking…")}
	case m.err != nil:
		return []string{"", gutter + errStyle.Render(m.err.Error())}
	}
	lines := []string{""}
	for _, block := range blocks(m.fields) {
		drawn := renderBlock(block, m.width)
		if len(drawn) == 0 {
			continue
		}
		if len(lines) > 1 {
			lines = append(lines, "")
		}
		lines = append(lines, drawn...)
	}
	return lines
}

// render is the window over the lines, on the wash, padded to the popup.
func (m lookModel) render() string {
	wash := lipgloss.NewStyle().Background(lipgloss.Color(colorWash))
	lines := m.lines()
	top := max(min(m.offset, max(len(lines)-m.height, 0)), 0)
	var out []string
	for i := top; i < len(lines) && len(out) < m.height; i++ {
		out = append(out, wash.Render(pad(truncateStyled(lines[i], m.width, false), m.width))+ansi.ResetStyle)
	}
	for len(out) < m.height {
		out = append(out, wash.Render(strings.Repeat(" ", m.width)))
	}
	return strings.Join(out, "\n")
}

// blocks splits the fields at the breaks between groups. A group sets its
// own value column, so one long label does not indent a page that has
// nothing else like it in it.
func blocks(fields []field) [][]field {
	var out [][]field
	var cur []field
	for _, f := range fields {
		if f.Kind == gapField {
			out = append(out, cur)
			cur = nil
			continue
		}
		cur = append(cur, f)
	}
	return append(out, cur)
}

// renderBlock draws one group. A group with nothing in it draws nothing
// at all, so a page that skipped a whole group does not leave a hole
// where it would have been.
func renderBlock(block []field, width int) []string {
	if len(block) == 0 {
		return nil
	}
	// The widest label in this group sets its value column, so values
	// line up.
	labelW := 0
	for _, f := range block {
		if f.Kind == pairField {
			labelW = max(labelW, lipgloss.Width(f.Label))
		}
	}
	var lines []string
	for _, f := range block {
		switch f.Kind {
		case headingField:
			lines = append(lines, gutter+titleStyle.Render(f.Value))
		case noteField:
			for _, c := range wrapValue(f.Value, width-len(gutter)-1) {
				lines = append(lines, gutter+noteStyle.Render(c))
			}
		case textField:
			// As the shell showed it, in the colors it drew, cut to the
			// page: a transcript wrapped would be a different
			// transcript. Each line ends reset, so nothing the shell set
			// outlives it.
			lines = append(lines, gutter+truncateStyled(f.Value, width-len(gutter), false)+ansi.ResetStyle)
		default:
			lines = append(lines, wrapField(f, labelW, width)...)
		}
	}
	return lines
}

// wrapField draws one label and its value, wrapping a long value under
// the value column rather than letting it run off the page.
func wrapField(f field, labelW, width int) []string {
	label := pad(labelStyle.Render(f.Label), labelW)
	valueW := max(width-labelW-2*len(gutter), 8)

	// The lead stands ahead of the value on its first line, in its own
	// tone, and the value wraps in the room it leaves.
	lead := ""
	if f.Lead != "" {
		lead = toneStyles[f.LeadTone].Render(f.Lead)
		if f.Value != "" {
			lead += "  "
		}
		valueW = max(valueW-lipgloss.Width(lead), 8)
	}

	chunks := wrapValue(f.Value, valueW)
	if len(chunks) == 0 {
		chunks = []string{""}
	}

	style := toneStyles[f.Tone]
	lines := make([]string, 0, len(chunks))
	for i, c := range chunks {
		if i == 0 {
			lines = append(lines, gutter+label+gutter+lead+style.Render(c))
			continue
		}
		lines = append(lines, gutter+strings.Repeat(" ", labelW)+gutter+style.Render(c))
	}
	return lines
}

// runLookPage is `conn page look`: the page, in the popup.
func runLookPage() {
	if _, err := tea.NewProgram(newLookModel()).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "conn: %v\n", err)
		os.Exit(1)
	}
}
