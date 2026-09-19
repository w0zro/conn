package main

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// readoutSubj is a waiting contact with everything the page has to say
// about one: a long command, a status it has stood in for a while, a
// directory of its own under its project, a pane conn holds, what it
// runs and what runs it, a session, and a project with a git status.
func readoutSubj() readoutSubject {
	e := entry{
		pid: 49212, kind: kindContact,
		command: "claude --resume d81d7536-e545-4881-8daa-f1d291a03be1",
		tty:     "ttys003", started: processesNow.Add(-92 * time.Minute),
		status: statusWaiting, since: processesNow.Add(-7 * time.Minute),
		cwd: "/Users/w0zro/projects/w0zro/conn/tools", asking: "input needed",
		depth: 1,
	}
	return readoutSubject{
		entry: e,
		proc: record{pid: e.pid, state: 'S', foreground: true,
			cpu: 2*time.Minute + 14*time.Second},
		project: project{path: "/Users/w0zro/projects/w0zro/conn"},
		parent: entry{pid: 49200, kind: kindShell, command: "zsh",
			tty: "ttys003", status: statusActive},
		children: []entry{{pid: 49300, kind: kindRun, command: "caffeinate -i -t 300",
			tty: "ttys003", status: statusActive, depth: 2}},
		pane:   pane{id: "%2"},
		inside: true,
		sess: sessionFile{SessionID: "d81d7536-e545-4881-8daa-f1d291a03be1",
			Name: "conn-2d", Version: "2.1.267", Kind: "interactive"},
		carried: session{Branch: "main", Prompt: "i want the info to use the pane on the right",
			Model: "claude-opus-5", Carried: 571_592,
			Ask: ask{Tool: "AskUserQuestion", Detail: "Does the status line still say PROCS while this question waits?"}},
		git: gitStatus{repo: true, branch: "main", dirty: 3,
			commit: "263cf91", subject: "The readout: what conn knows of a row",
			when: processesNow.Add(-3 * time.Hour), upstream: "origin/main", ahead: 142},
	}
}

// The readout at both widths is the file of record: a contact's sheet,
// and the page of groups every other row gets, here a shell's.
func TestTheReadoutIsWhatItWas(t *testing.T) {
	b := composeReadout(readoutSubj(), "/Users/w0zro", processesNow)
	golden(t, "readout-120x40.txt", texts(drawReadout(b, 120, 40, plain)))
	golden(t, "readout-narrow-60x40.txt", texts(drawReadout(b, 60, 40, plain)))
	s := readoutSubj()
	s.entry.kind, s.entry.command, s.entry.typed, s.entry.status, s.entry.asking = kindShell, "zsh", "", statusActive, ""
	s.entry.sockets = []socket{{"TCP", "*:5173", "LISTEN"}}
	s.sess, s.carried = sessionFile{}, session{}
	golden(t, "readout-shell-100x40.txt", texts(drawReadout(composeReadout(s, "/Users/w0zro", processesNow), 100, 40, plain)))
}

// The page says the things the processes view's columns have no room
// for, and says them whole. A contact's page is the sheet: who it is,
// the question in a card, what to do, what to be careful of, how the
// work has gone, and the specifications beside.
func TestTheReadoutSaysWhatTheRowCannot(t *testing.T) {
	// Tall enough for the whole page, and narrow enough for one column,
	// so a value stands whole on its own line rather than wrapped beside
	// the sheet.
	text := texts(drawReadout(composeReadout(readoutSubj(), "/Users/w0zro", processesNow), 100, 60, plain))

	// The command as it was written, not in conn's own upper case: it is
	// a thing somebody might retype.
	if !strings.Contains(text, "claude --resume d81d7536-e545-4881-8daa-f1d291a03be1") {
		t.Errorf("the whole command is not on the page:\n%s", text)
	}
	// The question is the reason to open the page on a waiting row at
	// all, so it comes before what to do about it, in a card, in the
	// contact's own words.
	if strings.Index(text, "WAITING FOR YOU") > strings.Index(text, "PROCEDURE") {
		t.Errorf("the question is not the first thing on the page:\n%s", text)
	}
	// How long it has waited is said in the card and among the figures,
	// and nowhere else.
	if n := strings.Count(text, "7 min"); n != 2 {
		t.Errorf("how long it has waited is on the page %d times:\n%s", n, text)
	}
	for what, want := range map[string]string{
		"what it stopped for":      "WAITING FOR YOU",
		"how long, at the right":   "  7 min\n",
		"what it is asking, whole": "“Does the status line still say PROCS while this question waits?”",
		"with what, and when":      "Asked with AskUserQuestion at",
		"what to do":               "1    enter    Go in and answer it",
		"what else":                "2    tab      Leave it, take the next thing waiting",
		"how long it has waited":   "Waiting ...... 7 min",
		"its own directory":        "Working in ... ~/projects/w0zro/conn/tools",
		"what the table says":      "It has the terminal and is sleeping.",
		"what x reaches":           "keeps going.",
		"what it has spent":        "Processor .... 2m 14s",
		"which session":            "d81d7536-e545-4881-8daa-f1d291a03be1",
		"what it goes by":          "CONN-2D",
		"the last thing it asked":  "You last gave it “i want the",
		"the story":                "It came up at 18:28 and worked for 1h 25m, then stopped to ask.",
		"the bar's caption":        "worked 1h 25m · waiting 7m",
		"the branch and the tree":  "Branch ....... main · 3 changed",
		"the commit":               "Commit ....... 263cf91",
		"what it is tracking":      "Tracking ..... origin/main · 142 ahead",
		"what runs it":             "Under ........ Shell zsh · 49200",
		"what it runs":             "Runs ......... Run caffeinate · 49300",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("%s (%q) is not on the page:\n%s", what, want, text)
		}
	}
}

// A row with nothing more to say says nothing more: a shell has no ask
// and no session, so it gets neither group, and a heading over nothing
// is not drawn. A process sitting in its tree's own project is not told
// it is there twice.
func TestTheReadoutLeavesOutWhatThereIsNoneOf(t *testing.T) {
	s := readoutSubject{
		entry: entry{pid: 88, kind: kindShell, command: "zsh", tty: "ttys009",
			started: processesNow.Add(-time.Hour), status: statusIdle,
			cwd: "/Users/w0zro/projects/w0zro/conn"},
		proc:    record{pid: 88, state: 'S'},
		project: project{path: "/Users/w0zro/projects/w0zro/conn"},
		inside:  true,
	}
	text := texts(drawReadout(composeReadout(s, "/Users/w0zro", processesNow), 120, 40, plain))

	for _, gone := range []string{"WAITING", "ASKS", "SAID", "CONTACT", "PROJECT\n", "TREE", "CWD", "CPU"} {
		if strings.Contains(text, gone) {
			t.Errorf("%q is on a page that has nothing to put under it:\n%s", gone, text)
		}
	}
	// A terminal conn did not open says so, since inside the server that
	// is a fact about the row rather than about conn.
	if !strings.Contains(text, "conn did not open it") {
		t.Errorf("a terminal conn does not hold is not said:\n%s", text)
	}
	// A status with no moment behind it gets no clause rather than a
	// made-up one.
	if strings.Contains(text, "Idle · for") {
		t.Errorf("a status with no moment was dated anyway:\n%s", text)
	}
}

// Outside the server conn holds no panes at all, so saying a row is in
// none of them says nothing about the row.
func TestTheReadoutSaysNothingOfPanesOutsideTheServer(t *testing.T) {
	s := readoutSubj()
	s.inside, s.pane = false, pane{}
	text := texts(drawReadout(composeReadout(s, "/Users/w0zro", processesNow), 120, 40, plain))
	if strings.Contains(text, "Pane") {
		t.Errorf("outside the server the page still spoke of panes:\n%s", text)
	}
}

// A stopped process is not dated from a contact's own clock: the moment
// conn holds is when the contact last changed what it says of itself,
// which is not when anything stopped it.
func TestTheReadoutDoesNotDateAFaultFromTheContactsClock(t *testing.T) {
	s := readoutSubj()
	s.entry.status, s.entry.fault, s.entry.asking = statusStopped, true, ""
	text := texts(drawReadout(composeReadout(s, "/Users/w0zro", processesNow), 120, 40, plain))
	if strings.Contains(text, "Stopped · for") {
		t.Errorf("a stopped row was dated from the contact's clock:\n%s", text)
	}
}

// A branch with nothing to track is not behind by nothing — there is
// nothing for it to be behind — so it says neither.
func TestTheReadoutSaysNothingOfTrackingWithNoUpstream(t *testing.T) {
	s := readoutSubj()
	s.git.upstream, s.git.ahead, s.git.dirty = "", 0, 0
	text := texts(drawReadout(composeReadout(s, "/Users/w0zro", processesNow), 120, 40, plain))
	if strings.Contains(text, "Tracking") {
		t.Errorf("a branch with no upstream was given one:\n%s", text)
	}
	if !strings.Contains(text, "main · clean") {
		t.Errorf("a clean tree does not say so:\n%s", text)
	}
}

// subjectOf reads the line of descent off the tree the processes view
// wrote: the nearest row above at a shallower depth runs this one, and
// the rows below it one level deeper are what it runs. A grandchild is
// not a child, and the next row at the same depth is a sibling, not
// kin.
func TestSubjectOfReadsTheLineOfDescent(t *testing.T) {
	pl := project{path: "/w", entries: []entry{
		{pid: 1, kind: kindShell, command: "zsh", depth: 0},
		{pid: 2, kind: kindContact, command: "claude", depth: 1},
		{pid: 3, kind: kindRun, command: "go test", depth: 2},
		{pid: 4, kind: kindRun, command: "compile", depth: 3}, // a grandchild
		{pid: 5, kind: kindRun, command: "caffeinate", depth: 2},
		{pid: 6, kind: kindShell, command: "zsh", depth: 0}, // another tree
	}}
	s, ok := subjectOf(2, []project{pl}, nil)
	if !ok {
		t.Fatal("pid 2 was not found")
	}
	if s.parent.pid != 1 {
		t.Errorf("claude runs under pid %d, want 1", s.parent.pid)
	}
	var kids []int
	for _, k := range s.children {
		kids = append(kids, k.pid)
	}
	if len(kids) != 2 || kids[0] != 3 || kids[1] != 5 {
		t.Errorf("claude runs %v, want [3 5] — the grandchild is not a child", kids)
	}
	// A root has nothing above it, and the next tree is not its child.
	s, _ = subjectOf(6, []project{pl}, nil)
	if s.parent.pid != 0 || len(s.children) != 0 {
		t.Errorf("a bare root stands under %d with %d children", s.parent.pid, len(s.children))
	}
}

// A row that ends while its page is up says so rather than going blank
// or holding the last thing it read.
func TestTheReadoutSaysWhenItsRowIsGone(t *testing.T) {
	text := texts(drawReadout(readoutReport{pid: 49212, gone: true}, 120, 40, plain))
	if !strings.Contains(text, "no longer listed") {
		t.Errorf("a page whose row went says:\n%s", text)
	}
	if !strings.Contains(text, "PID 49212") {
		t.Errorf("it still says which row it was about:\n%s", text)
	}
}

// Every label fits inside the leader's field. One that does not leaves
// a single dot and starts its value a column past every other value on
// the page, which is the one thing a column of facts must not do.
func TestEveryReadoutLabelFitsTheLeader(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range []readoutSubject{readoutSubj(), {entry: entry{pid: 1, kind: kindShell}, inside: true}} {
		for _, g := range composeReadout(s, "/Users/w0zro", processesNow).groups {
			for _, f := range g.facts {
				seen[f.label] = true
				if len(f.label) > labelW {
					t.Errorf("the label %q is %d wide, past the leader's %d", f.label, len(f.label), labelW)
				}
			}
		}
	}
	if len(seen) < 10 {
		t.Errorf("only %d labels were seen; the check is not reaching the page", len(seen))
	}
	// And the values do line up, which is what the width is for.
	text := texts(drawReadout(composeReadout(readoutSubj(), "/Users/w0zro", processesNow), 120, 40, plain))
	col := -1
	for _, line := range strings.Split(text, "\n") {
		i := strings.Index(line, ". ")
		if !strings.Contains(line, " ... ") || i < 0 {
			continue
		}
		if col == -1 {
			col = i
		} else if i != col {
			t.Errorf("a value starts at column %d where the rest start at %d: %q", i, col, line)
		}
	}
}

// The page is about one row, and says of that row what the processes
// view has no room for. What it does not say is everything conn
// already knows it put there itself, everything a neighbouring row
// would say on its own page, and anything the page has said once.
func TestTheReadoutSaysNothingTwiceAndNothingOfConnsOwn(t *testing.T) {
	s := readoutSubj()
	// A contact conn raised carries the note conn appends to it: a
	// thousand characters of conn's own prose, with newlines through
	// it. The processes view has always dropped it and so does this.
	s.entry.typed = s.entry.command
	s.entry.command = "claude --append-system-prompt " + insideNote("/tmp/sock") + " --resume d81d7536-e545-4881-8daa-f1d291a03be1"
	// A shell a contact runs carries the environment snapshot it was
	// started with, which is a screen of somebody else's quoting.
	s.children = append(s.children, entry{pid: 49301, kind: kindShell, tty: "ttys003",
		command: "zsh -c source /Users/w0zro/.claude/shell-snapshots/snapshot-zsh-1789.sh 2>/dev/null || true && eval 'go build'",
		status:  statusActive, depth: 2})
	text := texts(drawReadout(composeReadout(s, "/Users/w0zro", processesNow), 100, 60, plain))

	for what, gone := range map[string]string{
		"the note conn appended":      "running inside conn",
		"a child's whole line":        "shell-snapshots",
		"the session, said twice":     "Session ...",
		"the branch the project says": "Its branch ... main",
		"the ordinary kind":           "Running ...... interactive",
	} {
		if strings.Contains(text, gone) {
			t.Errorf("%s (%q) is on the page:\n%s", what, gone, text)
		}
	}
	// What is left of each is the part somebody would act on.
	for what, want := range map[string]string{
		"the command as typed": "Command ...... claude --resume d81d7536-e545-4881-8daa-f1d291a03be1",
		"the child by program": "Shell zsh · 49301",
		"the session, once":    "d81d7536-e545-4881-8daa-f1d291a03be1",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("%s (%q) is not on the page:\n%s", what, want, text)
		}
	}
	if n := strings.Count(text, "d81d7536-e545-4881-8daa-f1d291a03be1"); n != 1 {
		t.Errorf("the session id is on the page %d times:\n%s", n, text)
	}

	// A session running behind another is not the ordinary case and is
	// worth its line; so is a branch the project has since left.
	s.sess.Kind, s.carried.Branch = "bg-spare", "topic/resume"
	text = texts(drawReadout(composeReadout(s, "/Users/w0zro", processesNow), 100, 60, plain))
	if !strings.Contains(text, "Running ...... bg-spare") || !strings.Contains(text, "Its branch ... topic/resume") {
		t.Errorf("what is not ordinary went unsaid:\n%s", text)
	}
}

// Who the contact is with, what is answering, and what it is hauling.
// The station sees a process and a transcript; these say what is at the
// other end of it.
func TestTheReadoutSaysWhoTheContactIsWith(t *testing.T) {
	text := texts(drawReadout(composeReadout(readoutSubj(), "/Users/w0zro", processesNow), 100, 60, plain))
	for what, want := range map[string]string{
		"the agent and what it is": "Claude Code 2.1.267",
		"what is answering":        "Model ........ claude-opus-5",
		"what it is hauling":       "571k of context carried",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("%s (%q) is not on the page:\n%s", what, want, text)
		}
	}

	// A contact conn cannot read says none of it rather than guessing.
	s := readoutSubj()
	s.carried.Model, s.carried.Carried = "", 0
	quiet := texts(drawReadout(composeReadout(s, "/Users/w0zro", processesNow), 100, 60, plain))
	for _, gone := range []string{"Model ...", "context carried"} {
		if strings.Contains(quiet, gone) {
			t.Errorf("%q is on the page for a contact that said nothing:\n%s", gone, quiet)
		}
	}
	// Who it is with is known from the program's own name, which conn
	// has whether the contact answers for itself or not.
	if !strings.Contains(quiet, "Claude Code") {
		t.Errorf("who it is with went unsaid:\n%s", quiet)
	}

	// A maker conn is not sure of is left off; the agent's own name is
	// the part that answers the question.
	s = readoutSubj()
	s.entry.command, s.entry.typed = "aider --model sonnet", ""
	if got := texts(drawReadout(composeReadout(s, "/Users/w0zro", processesNow), 100, 60, plain)); !strings.Contains(got, "  Aider 2.1.267") {
		t.Errorf("an agent with no maker named:\n%s", got)
	}
}

// Tokens are said in the largest unit that keeps them short: a reader
// has no use for the last three digits of half a million.
func TestTokensAreShort(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1_000: "1K", 571_592: "571K", 1_000_000: "1.0M", 1_450_000: "1.4M"} {
		if got := tokens(n); got != want {
			t.Errorf("tokens(%d) = %q, want %q", n, got, want)
		}
	}
}

// The page is what the workspace holds while the keys are on the panel
// in the processes view. The keys arriving ask for it, going into a
// process takes them away and the process takes the workspace, and the
// keys coming back bring the page back — about the row the cursor is
// on, which after reaching something is that something.
func TestThePageFollowsTheKeys(t *testing.T) {
	panel := func() model {
		m := newModel(plain)
		m.inside, m.view, m.focused = true, viewProcesses, true
		m.srv = &server{tmux: "/nonexistent/tmux", socket: "/tmp/none"}
		m.projects = []project{{path: "/w", entries: []entry{
			{pid: 11, tty: "ttys001"}, {pid: 12, tty: "ttys002"},
		}}}
		m.panes = map[string]pane{"ttys002": {id: "%2", tty: "ttys002"}}
		m.cursor = 11
		return m
	}

	// A reading with the keys here and a row to be about asks for it.
	m := panel()
	next, cmd := m.Update(processesMsg{gen: m.processesGen, projects: m.projects})
	if m = next.(model); !m.looking || cmd == nil {
		t.Errorf("the page did not take the workspace: looking %v, cmd %v", m.looking, cmd != nil)
	}
	// And not twice: the reading after it finds the page already there.
	if next, _ = m.Update(processesMsg{gen: m.processesGen, projects: m.projects}); !next.(model).looking {
		t.Error("a second reading lost the page")
	}

	// Going into a process takes the keys, and the workspace is that
	// process: the next reading does not pull the page back over it.
	m = panel()
	m.looking = true
	next, _ = m.Update(reachedMsg{"ttys002"})
	m = next.(model)
	if m.focused {
		t.Error("reaching a process left the keys on the panel")
	}
	m.looking = false
	if next, _ = m.Update(processesMsg{gen: m.processesGen, projects: m.projects}); next.(model).looking {
		t.Error("the page took the workspace back from a process the operator is in")
	}

	// The keys coming back bring it back.
	next, cmd = m.Update(tea.FocusMsg{})
	if m = next.(model); !m.focused || !m.looking || cmd == nil {
		t.Errorf("the keys coming back did not bring the page: focused %v, looking %v", m.focused, m.looking)
	}

	// There is no key for the page and none is needed: i is a letter
	// like any other, and nothing the operator can press takes the page
	// away. What takes it away is going into a process, which is the
	// workspace holding that instead.
	m.cursor = 11
	next, _ = m.Update(tea.KeyPressMsg(tea.Key{Text: "i"}))
	if m = next.(model); !m.looking {
		t.Error("i took the page away")
	}
}

// A readout reads a channel, and says so when the channel reads
// nothing. A directory that is no repository has no state to report
// and the page says nothing of it, which is the page saying nothing of
// what there is none of; a git conn went to ask and could not is a
// reading conn tried for and did not get, and that is the page's to
// say. The two looked the same from here until git could tell them
// apart: both failed, both exited the same way.
func TestThePageSaysWhatItCouldNotRead(t *testing.T) {
	// A directory with no repository in it says nothing about one.
	s := readoutSubj()
	s.git = gitStatus{}
	quiet := texts(drawReadout(composeReadout(s, "/Users/w0zro", processesNow), 100, 60, plain))
	if strings.Contains(quiet, "Git ...") {
		t.Errorf("a directory that is no repository was reported as unread:\n%s", quiet)
	}

	// A git conn could not ask says which way it could not.
	for _, why := range []string{"NOT ON PATH", "NO ANSWER IN 2S"} {
		s = readoutSubj()
		s.git = gitStatus{problem: why}
		got := texts(drawReadout(composeReadout(s, "/Users/w0zro", processesNow), 100, 60, plain))
		if !strings.Contains(got, "Git .......... "+why) {
			t.Errorf("a git that could not be asked (%q) went unsaid:\n%s", why, got)
		}
	}

	// A contact that gives no account of itself says that, rather than
	// leaving a group with one row in it and no reason.
	s = readoutSubj()
	s.sess, s.carried = sessionFile{}, session{}
	got := texts(drawReadout(composeReadout(s, "/Users/w0zro", processesNow), 100, 60, plain))
	if !strings.Contains(got, "Says ......... Nothing conn can read") {
		t.Errorf("a contact conn cannot ask went unsaid:\n%s", got)
	}
	// And a row that is no contact at all has no such channel to read.
	s.entry.kind, s.entry.command, s.entry.typed = kindRun, "sleep 300", ""
	if got := texts(drawReadout(composeReadout(s, "/Users/w0zro", processesNow), 120, 60, plain)); strings.Contains(got, "CONTACT") {
		t.Errorf("a run was given a contact group:\n%s", got)
	}
}

// A row that is no process — a declaration down, a service down — is
// filed under a number of conn's own, and the page names the row
// rather than showing that number as a pid.
func TestAMadeUpPidIsNotShownAsOne(t *testing.T) {
	for _, e := range []entry{
		{pid: declaredPID("/w", "stack"), kind: kindRun, command: "stack · docker compose up", typed: "stack · docker compose up", status: statusDown, cwd: "/w"},
		{pid: declaredPID("/w", "stack/web"), kind: kindService, command: "web", typed: "web", status: statusDown, cwd: "/w", depth: 1},
	} {
		text := texts(drawReadout(composeReadout(readoutSubject{entry: e, project: project{path: "/w"}}, "/Users/w0zro", processesNow), 120, 20, plain))
		if strings.Contains(text, "PID -") {
			t.Errorf("the page shows conn's own number as a pid:\n%s", text)
		}
		if want := program(e.typed); !strings.Contains(text, want) {
			t.Errorf("the page does not name the row %q:\n%s", want, text)
		}
	}
}

// The sheet fits every width the pane can have, one column or two: a
// row past the edge is a row tmux cuts, and nothing on the sheet is
// worth less than seeing it end.
func TestTheSheetFitsEveryWidth(t *testing.T) {
	for _, w := range []int{panelMinCols, 60, twoColumns - 1, twoColumns, 120, 160} {
		for _, r := range drawReadout(composeReadout(readoutSubj(), "/Users/w0zro", processesNow), w, 0, plain) {
			if n := utf8.RuneCountInString(r.text); n > w {
				t.Errorf("at %d a row is %d wide: %q", w, n, r.text)
			}
		}
	}
}
