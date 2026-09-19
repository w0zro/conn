package main

import (
	"slices"
	"strings"
	"testing"
	"time"
)

// The reading is filed by state: what wants you first and oldest wait
// first, then what is working, what is open, and what is not running.
// A filed row remembers where it was read, what conn does at a project
// is done at the row's own, and the head of a terminal is still the
// shell that ran the contact.
func TestThePanelIsFiledByState(t *testing.T) {
	now := processesNow
	in := []project{
		{path: "/w/a", entries: []entry{
			{pid: 1, kind: kindShell, command: "zsh", tty: "ttys001", status: statusActive},
			{pid: 2, kind: kindContact, command: "claude", tty: "ttys001", status: statusWaiting, depth: 1, since: now.Add(-2 * time.Minute)},
			{pid: 4, kind: kindEditor, command: "vim", tty: "ttys001", status: statusStopped, depth: 1, fault: true},
			{pid: 3, kind: kindRun, command: "node vite", tty: "ttys004", status: statusActive, ports: []string{"5173"}},
			{pid: 6, kind: kindRun, command: "worker", tty: "", status: statusDown, depth: 1},
		}},
		{path: "/w/b", entries: []entry{
			{pid: 5, kind: kindContact, command: "claude", tty: "ttys002", status: statusWaiting, since: now.Add(-9 * time.Minute)},
			{pid: 7, kind: kindShell, command: "zsh", tty: "ttys003", status: statusWorking, under: "go test ./..."},
		}},
	}
	out := byState(in)
	var got []string
	for _, pl := range out {
		var pids []string
		for _, e := range pl.entries {
			pids = append(pids, string(rune('0'+e.pid)))
		}
		got = append(got, groupTitle(pl.path)+":"+strings.Join(pids, ""))
	}
	if want := "WAITING FOR YOU:52 WORKING:7 SERVING:3 IDLE:14 NOT RUNNING:6"; strings.Join(got, " ") != want {
		t.Errorf("filed as %v, want %s", got, want)
	}
	if e := out[0].entries[1]; !e.filed || e.from != "/w/a" || e.fromDepth != 1 || e.depth != 0 {
		t.Errorf("a filed row does not remember where it was read: %+v", e)
	}
	if rowsBlock(out, out[0].entries[1], out[0]).path != "/w/a" {
		t.Error("a filed row's block is not the one it was read in")
	}
	if pid, _, ok := headOf(out, "ttys001"); !ok || pid != 1 {
		t.Errorf("the head of the terminal is %d, not the shell that ran the contact", pid)
	}
	if pid, _, ok := headOf(out, "ttys002"); !ok || pid != 5 {
		t.Errorf("a contact that is its terminal's head is not found: %d", pid)
	}
	if quiet := byState(nil); len(quiet) != 0 {
		t.Errorf("nothing filed grew a group: %+v", quiet)
	}
	// The reading by project again, for whatever asks about projects
	// rather than rows.
	if back := unfiled(out); len(back) != 2 || back[0].path != "/w/b" || len(back[0].entries) != 2 || back[1].path != "/w/a" || len(back[1].entries) != 5 {
		t.Errorf("unfiled: %+v", back)
	}

	// The page of a filed row is its project's, and still says what
	// runs it and what it runs.
	s, ok := subjectOf(2, out, nil)
	if !ok || s.project.path != "/w/a" || s.parent.pid != 1 {
		t.Errorf("the filed row's page: project %q, parent %d", s.project.path, s.parent.pid)
	}
	s, _ = subjectOf(1, out, nil)
	if len(s.children) != 2 {
		t.Errorf("the shell's page runs %d rows, not the contact and the editor", len(s.children))
	}

	// Drawn: each group under its eyebrow with its count, a row's
	// project at its right in the faint or a wait's age in the accent,
	// and a fault stamped. The file of record is the panel's own width.
	b := composeProcesses(out, map[string]pane{"ttys001": {id: "%1"}}, "ttys001", testProjRoots, testIsProject, "/Users/w0zro", now, "", false)
	b.lit = true
	rows := drawProcesses(b, 5, panelWidth, 30, plain)
	text := texts(rows)
	golden(t, "processes-state-44x30.txt", text)
	for _, want := range []string{"WAITING FOR YOU ─", "─ 2", "WORKING ─", "SERVING ─", "IDLE ─", "NOT RUNNING ─",
		"●  claude", "9 min", "2 min", "●  go test ./... ⣾", "●  :5173  node vite", "○  zsh", "◌  worker", " Stopped"} {
		if !strings.Contains(text, want) {
			t.Errorf("the panel lacks %q:\n%s", want, text)
		}
	}
	for _, r := range rows {
		if n := len([]rune(r.text)); n > panelWidth {
			t.Errorf("a row is %d wide in %d: %q", n, panelWidth, r.text)
		}
	}
	// What is not running is struck through, in color.
	if lit := texts(drawProcesses(b, 5, panelWidth, 30, colored())); !strings.Contains(lit, "\x1b[9m") {
		t.Errorf("what is not running is not struck through:\n%s", lit)
	}
}

// The waited word is in minutes under an hour, and spelled above it.
func TestMinutes(t *testing.T) {
	for d, want := range map[time.Duration]string{0: "0 min", 22 * time.Minute: "22 min", 59*time.Minute + 59*time.Second: "59 min", 85 * time.Minute: "1h 25m"} {
		if got := minutes(d); got != want {
			t.Errorf("minutes(%v) = %q, want %q", d, got, want)
		}
	}
}

// The key bar says only the keys the cursor's row can take: enter where
// there is somewhere to go, x where there is something to end, tab
// where anything waits, u where the project has anything down, and the
// keys that act at a project only inside the server. While a process
// has the keys, it says the chords instead.
func TestTheBarSaysWhatTheRowCanTake(t *testing.T) {
	m := model{view: viewProcesses, inside: true, focused: true, panes: map[string]pane{"ttys001": {id: "%1"}}}
	m.projects = byState([]project{{path: "/w", entries: []entry{
		{pid: 1, kind: kindShell, command: "zsh", tty: "ttys001", status: statusActive},
		{pid: 2, kind: kindContact, command: "claude", tty: "ttys002", status: statusWaiting},
		{pid: -9, kind: kindRun, command: "worker", status: statusDown, declared: "worker@/w"},
	}}})
	// The words are written in the lower case, whatever the hint says.
	has := func(bar, key, does string) bool {
		return strings.Contains(bar, key+" #[nobold fg="+grayHex+"]"+strings.ToLower(does))
	}
	m.cursor = 1
	bar := m.bar()
	for _, want := range [][2]string{{"j k", "Move"}, {"enter", "Open"}, {"tab", "Next waiting"}, {"x", "End it"}, {"s", "Shell"}, {"U", "Bring up all"}, {"?", "Help"}} {
		if !has(bar, want[0], want[1]) {
			t.Errorf("on the shell the bar lacks %s %s:\n%s", want[0], want[1], bar)
		}
	}
	m.cursor = 2 // a contact in no pane conn holds: nowhere to go into
	if bar := m.bar(); has(bar, "enter", "Open") || !has(bar, "x", "End it") {
		t.Errorf("on an unreachable contact the bar offers %s", bar)
	}
	if bar := m.bar(); has(bar, "u", "Bring it up") {
		t.Errorf("on a shell the bar offers u:\n%s", bar)
	}
	m.cursor = -9 // declared and down: brought up, not ended
	if bar := m.bar(); !has(bar, "enter", "Bring it up, go in") || !has(bar, "u", "Bring it up") || has(bar, "x", "End it") {
		t.Errorf("on a down declaration the bar offers %s", bar)
	}
	all := unfiled(m.projects)
	for i := range all[0].entries {
		if all[0].entries[i].pid == 2 {
			all[0].entries[i].status = statusIdle
		}
	}
	m.projects = byState(all)
	if bar := m.bar(); has(bar, "tab", "Next waiting") {
		t.Errorf("with nothing waiting the bar offers tab:\n%s", bar)
	}
	m.inside = false
	if bar := m.bar(); has(bar, "s", "Shell") || has(bar, "U", "Bring up all") {
		t.Errorf("outside the server the bar offers keys that need it:\n%s", bar)
	}
	// With nothing running there is no row and so no project to act at:
	// the list and the manual are what is left.
	empty := model{view: viewProcesses, inside: true, focused: true}
	if bar := empty.bar(); has(bar, "s", "Shell") || has(bar, "a", "New contact") || !has(bar, "p", "Projects") || !has(bar, "?", "Help") {
		t.Errorf("with no rows the bar offers %s", bar)
	}
	m.inside, m.focused = true, false
	if bar := m.bar(); !has(bar, "^space", "Panel") || !has(bar, "^space ?", "Help") || has(bar, "x", "End it") {
		t.Errorf("with the keys in a process the bar offers %s", bar)
	}
	m.focused = true
	m.kill = &pendingKill{prompt: "kill -TERM 1 · zsh?"}
	if bar := m.bar(); strings.Contains(bar, "CONFIRM") || !strings.Contains(bar, "kill -TERM 1 · zsh?") || !has(bar, "y", "Yes") {
		t.Errorf("a question armed puts %s on the bar", bar)
	}
	// The clock at the right edge of the band, as a mission clock reads.
	m.up, m.now = processesNow.Add(-(5*24*time.Hour + 2*time.Hour + 14*time.Minute)), processesNow
	if up := m.upWord(); !strings.Contains(up, "T+ 5d 02h 14m ") {
		t.Errorf("the clock reads %q", up)
	}
}

// A row is serving when it is alive and has a port: one it listens on,
// or one its container publishes. A server is not idle, and
// a port is what tells a node that serves from a node that builds. A
// contact is filed by what it asks, never by what it has open; what is
// not running serves nothing; and a port under a shell is the shell's
// to say, the fold carrying it up onto the row that stays.
func TestAServingRowIsFiledByItsPort(t *testing.T) {
	listens := []socket{{proto: "TCP", addr: "127.0.0.1:5173", state: "LISTEN"}, {proto: "TCP", addr: "[::1]:5173", state: "LISTEN"},
		{proto: "TCP", addr: "127.0.0.1:5173->127.0.0.1:50122", state: "ESTABLISHED"}, {proto: "UDP", addr: "*:5353"},
		{proto: "unix", addr: "/tmp/vite.sock"}, {proto: "TCP", addr: "*:24678", state: "LISTEN"}}
	if got := strings.Join(listeningPorts(listens), " "); got != "5173 24678" {
		t.Errorf("the ports are %q, want the TCP listeners once each, lowest first", got)
	}
	for _, c := range []struct {
		e    entry
		want string
	}{
		{entry{kind: kindRun, command: "node", status: statusActive, ports: []string{"5173"}}, groupServing},
		{entry{kind: kindService, command: "web", status: statusActive, ports: []string{"8438"}}, groupServing},
		{entry{kind: kindService, command: "db", status: "UNHEALTHY", fault: true, ports: []string{"5432"}}, groupServing},
		{entry{kind: kindRun, command: "node", status: statusActive}, groupIdle},
		{entry{kind: kindService, command: "web", status: statusDown, ports: []string{"8438"}}, groupNotRunning},
		{entry{kind: kindContact, command: "claude", status: statusIdle, ports: []string{"41231"}}, groupIdle},
		{entry{kind: kindContact, command: "claude", status: statusWaiting, ports: []string{"41231"}}, groupWaiting},
	} {
		if got := stateOf(c.e); got != c.want {
			t.Errorf("%s %s with ports %v files under %q, want %q", c.e.kind, c.e.command, c.e.ports, groupTitle(got), groupTitle(c.want))
		}
	}
	folded := fold([]project{{path: "/w", entries: []entry{
		{pid: 1, kind: kindShell, command: "zsh", typed: "zsh", tty: "ttys001", status: statusActive},
		{pid: 2, kind: kindRun, command: "npm run dev", typed: "npm run dev", tty: "ttys001", status: statusActive, depth: 1, ports: []string{"24678"}},
		{pid: 3, kind: kindRun, command: "node vite", typed: "node vite", tty: "ttys001", status: statusActive, depth: 2, ports: []string{"5173"}},
		{pid: 4, kind: kindContact, command: "claude", typed: "claude", tty: "ttys002", status: statusWorking},
		{pid: 5, kind: kindRun, command: "python -m http.server", typed: "python -m http.server", tty: "ttys002", status: statusActive, depth: 1, ports: []string{"8000"}},
	}}})
	var rows []string
	for _, e := range folded[0].entries {
		rows = append(rows, strings.Repeat(" ", e.depth)+activityOf(e)+portsWord(e.ports))
	}
	// A process that listens stands as a row of its own under whatever
	// started it, with its own command and its own port: the row to
	// reach a port from is the process that holds it. The shell that
	// ran it carries no port, and files by what it is, as a shell whose
	// child is a contact does.
	if want := []string{"zsh", " npm run dev · :24678", "  node vite · :5173", "claude", " python -m http.server · :8000"}; !slices.Equal(rows, want) {
		t.Errorf("the fold kept %q, want %q", rows, want)
	}
	if got := stateOf(folded[0].entries[1]); got != groupServing {
		t.Errorf("the server files under %q", groupTitle(got))
	}
	if got := stateOf(folded[0].entries[0]); got != groupIdle {
		t.Errorf("the shell that ran the server files under %q", groupTitle(got))
	}
	// Drawn narrow, the port is the last thing to go: in eight cells
	// it stands alone, without the dot that joined it to the command,
	// and only a width the port itself does not fit gives the cells
	// to the command.
	l := (&canvas{p: plain, width: 20}).line()
	l.activity(plain.ink, plain.gray, "node vite", []string{"5173"}, 8)
	if l.b.String() != ":5173" {
		t.Errorf("in eight cells the row says %q", l.b.String())
	}
	l = (&canvas{p: plain, width: 20}).line()
	l.activity(plain.ink, plain.gray, "node vite", []string{"5173"}, 12)
	if l.b.String() != "nod… · :5173" {
		t.Errorf("in twelve cells the row says %q", l.b.String())
	}
	l = (&canvas{p: plain, width: 20}).line()
	l.activity(plain.ink, plain.gray, "node vite", []string{"5173"}, 4)
	if l.b.String() != "nod…" {
		t.Errorf("in four cells the row says %q", l.b.String())
	}
}

// Every group stands whether it has rows or not, with its count, so
// the panel keeps one shape as rows come and go. Only a reading with
// nothing in it at all has no groups.
func TestEveryGroupStandsWithItsCount(t *testing.T) {
	out := byState([]project{{path: "/w", entries: []entry{
		{pid: 1, kind: kindShell, command: "zsh", tty: "ttys001", status: statusIdle},
	}}})
	if len(out) != len(groupOrder) {
		t.Fatalf("one idle shell stands under %d groups, want every one of the %d", len(out), len(groupOrder))
	}
	for i, g := range groupOrder {
		if out[i].path != g {
			t.Errorf("group %d is %q, want %q", i, groupTitle(out[i].path), groupTitle(g))
		}
	}
	b := composeProcesses(out, nil, "", testProjRoots, testIsProject, "/Users/w0zro", processesNow, "", false)
	b.lit = true
	text := texts(drawProcesses(b, 1, panelWidth, 30, plain))
	for _, want := range []string{"WAITING FOR YOU ───────────────────── 0", "WORKING ───────────────────────────── 0", "SERVING ───────────────────────────── 0", "IDLE ──────────────────────────────── 1", "NOT RUNNING ───────────────────────── 0"} {
		if !strings.Contains(text, want) {
			t.Errorf("the panel lacks %q:\n%s", want, text)
		}
	}
}

// A row keeps its port when the width is short: the port is where you
// would go, and the row is looked at for it. The command is elided
// beside it and the project at the right yields to it, elided from the
// left, rather than the port being cut off behind a long project name.
func TestAFiledRowKeepsItsPortBeforeItsProject(t *testing.T) {
	long := "/Volumes/work/some-organization-name/auditboard-backend-services"
	m := newModel(plain)
	m.view, m.inside, m.width, m.height = viewProcesses, true, panelWidth, 20
	m.projects = byState([]project{{path: long, entries: []entry{
		{pid: 5, kind: kindRun, command: "pnpm start", cwd: long, status: statusActive, ports: []string{"3000"}},
		{pid: 6, kind: kindRun, command: "node server.js", cwd: long, status: statusActive, ports: []string{"8080", "8081"}},
	}}})
	for _, width := range []int{panelWidth, 50, 40} {
		text := texts(drawProcesses(m.processesReport(), 5, width, 20, plain))
		if !strings.Contains(text, ":3000") || !strings.Contains(text, ":8080 :8081") {
			t.Errorf("at %d wide the rows lost their ports:\n%s", width, text)
		}
		if !strings.Contains(text, "pnpm") || !strings.Contains(text, "node") {
			t.Errorf("at %d wide the rows lost their commands:\n%s", width, text)
		}
		if !strings.Contains(text, "…") {
			t.Errorf("at %d wide nothing yielded to the port:\n%s", width, text)
		}
	}
}

// The column of ports is a group's own. A serving row's command starts
// after the widest port in its group, and a row filed elsewhere starts
// its command right after the dot: the groups are read one at a time,
// and a blank slot under IDLE is a gap, not a column.
func TestThePortsColumnIsTheGroupsOwn(t *testing.T) {
	m := newModel(plain)
	m.view, m.inside, m.width, m.height = viewProcesses, true, panelWidth, 20
	m.projects = byState([]project{{path: "/home/w0zro/projects/app", entries: []entry{
		{pid: 5, kind: kindRun, command: "node server.js", status: statusActive, ports: []string{"8080", "8081"}},
		{pid: 6, kind: kindShell, command: "zsh", status: statusIdle},
	}}})
	var serving, idle string
	for _, line := range strings.Split(texts(drawProcesses(m.processesReport(), 0, panelWidth, 20, plain)), "\n") {
		switch {
		case strings.Contains(line, "node"):
			serving = line
		case strings.Contains(line, "zsh"):
			idle = line
		}
	}
	if serving == "" || idle == "" {
		t.Fatalf("the rows are missing:\n%q\n%q", serving, idle)
	}
	if strings.Index(serving, "node") <= strings.Index(serving, ":8080 :8081") {
		t.Errorf("the serving row's command does not follow its ports: %q", serving)
	}
	if strings.Index(idle, "zsh") >= strings.Index(serving, "node") {
		t.Errorf("the idle row waits on a column its group does not have:\n%q\n%q", serving, idle)
	}
}
