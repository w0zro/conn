package main

import (
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// The panel is filed by project: a block per project, its rows in the
// order they were read, and what conn does at a project done at the
// row's own. The head of a terminal is the shell that ran the contact.
func TestThePanelIsFiledByProject(t *testing.T) {
	now := processesNow
	out := []project{
		{path: "/Users/w0zro/projects/w0zro/conn", entries: []entry{
			{pid: 1, kind: kindShell, command: "zsh", tty: "ttys001", status: statusActive},
			{pid: 2, kind: kindContact, command: "claude", tty: "ttys001", status: statusWaiting, depth: 1, since: now.Add(-2 * time.Minute)},
			{pid: 4, kind: kindEditor, command: "vim", tty: "ttys001", status: statusStopped, depth: 1, fault: true},
			{pid: 3, kind: kindRun, command: "node vite", tty: "ttys004", status: statusActive, ports: []string{"5173"}},
			{pid: 6, kind: kindRun, command: "worker", tty: "", status: statusDown, depth: 1},
		}},
		{path: "/Users/w0zro/projects/w0zro/vim.pro/conjurer", entries: []entry{
			{pid: 5, kind: kindContact, command: "claude", tty: "ttys002", status: statusWaiting, since: now.Add(-9 * time.Minute)},
			{pid: 7, kind: kindShell, command: "zsh", tty: "ttys003", status: statusWorking, under: "go test ./..."},
		}},
	}
	if pid, _, ok := headOf(out, "ttys001"); !ok || pid != 1 {
		t.Errorf("the head of the terminal is %d, not the shell that ran the contact", pid)
	}
	if pid, _, ok := headOf(out, "ttys002"); !ok || pid != 5 {
		t.Errorf("a contact that is its terminal's head is not found: %d", pid)
	}
	// The page of a row is its project's, and says what runs it and
	// what it runs.
	s, ok := subjectOf(2, out, nil)
	if !ok || s.project.path != "/Users/w0zro/projects/w0zro/conn" || s.parent.pid != 1 {
		t.Errorf("the row's page: project %q, parent %d", s.project.path, s.parent.pid)
	}
	s, _ = subjectOf(1, out, nil)
	if len(s.children) != 2 {
		t.Errorf("the shell's page runs %d rows, not the contact and the editor", len(s.children))
	}

	// Drawn: each project under its eyebrow with what it wants of you
	// at the end of the rule, a wait stamped with its age, a fault
	// stamped with its word, and what is down saying so. The file of
	// record is the panel's own width.
	b := composeProcesses(out, map[string]pane{"ttys001": {id: "%1"}}, "ttys001", testProjRoots, testIsProject, "/Users/w0zro", now, "", false, true)
	b.lit = true
	rows := drawProcesses(b, 5, panelWidth, 30, plain)
	text := texts(rows)
	golden(t, "processes-filed-44x30.txt", text)
	for _, want := range []string{"conn ─", "conjurer ─", "─  WAITING", " 9 MIN", " 2 MIN",
		"⣾ ●  go test ./...", "●  node vite", ":5173", "○  zsh", "◌    worker", " STOPPED", " DOWN"} {
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
	m.projects = []project{{path: "/w", entries: []entry{
		{pid: 1, kind: kindShell, command: "zsh", tty: "ttys001", status: statusActive},
		{pid: 2, kind: kindContact, command: "claude", tty: "ttys002", status: statusWaiting},
		{pid: -9, kind: kindRun, command: "worker", status: statusDown, declared: "worker@/w"},
	}}}
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
	for i := range m.projects[0].entries {
		if m.projects[0].entries[i].pid == 2 {
			m.projects[0].entries[i].status = statusIdle
		}
	}
	if bar := m.bar(); has(bar, "tab", "Next waiting") {
		t.Errorf("with nothing waiting the bar offers tab:\n%s", bar)
	}
	m.inside = false
	if bar := m.bar(); has(bar, "s", "Shell") || has(bar, "U", "Bring up all") {
		t.Errorf("outside the server the bar offers keys that need it:\n%s", bar)
	}
	// With nothing running there is no row and so no project to act at:
	// the station's own keys are what is left.
	empty := model{view: viewProcesses, inside: true, focused: true}
	if bar := empty.bar(); has(bar, "s", "Shell") || has(bar, "a", "New contact") ||
		!has(bar, "p", "Projects") || !has(bar, ",", "Settings") || !has(bar, "?", "Help") {
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
// or one its container publishes. A server is not idle, and a port is
// what tells a node that serves from a node that builds. A contact
// stands by what it asks, never by what it has open; what is not
// running serves nothing; and a port under a shell is the shell's to
// say, the fold carrying it up onto the row that stays.
func TestAServingRowIsKnownByItsPort(t *testing.T) {
	listens := []socket{{proto: "TCP", addr: "127.0.0.1:5173", state: "LISTEN"}, {proto: "TCP", addr: "[::1]:5173", state: "LISTEN"},
		{proto: "TCP", addr: "127.0.0.1:5173->127.0.0.1:50122", state: "ESTABLISHED"}, {proto: "UDP", addr: "*:5353"},
		{proto: "unix", addr: "/tmp/vite.sock"}, {proto: "TCP", addr: "*:24678", state: "LISTEN"}}
	if got := strings.Join(listeningPorts(listens), " "); got != "5173 24678" {
		t.Errorf("the ports are %q, want the TCP listeners once each, lowest first", got)
	}
	for _, c := range []struct {
		e    entry
		want bool
	}{
		{entry{kind: kindRun, command: "node", status: statusActive, ports: []string{"5173"}}, true},
		{entry{kind: kindService, command: "web", status: statusActive, ports: []string{"8438"}}, true},
		{entry{kind: kindService, command: "db", status: "UNHEALTHY", fault: true, ports: []string{"5432"}}, true},
		{entry{kind: kindRun, command: "node", status: statusActive}, false},
		{entry{kind: kindService, command: "web", status: statusDown, ports: []string{"8438"}}, false},
		{entry{kind: kindRun, command: "web · npm run dev", status: statusEnded, ports: []string{"8438"}}, false},
		{entry{kind: kindContact, command: "claude", status: statusIdle, ports: []string{"41231"}}, false},
	} {
		if got := serving(c.e); got != c.want {
			t.Errorf("%s %s with ports %v serves %v, want %v", c.e.kind, c.e.command, c.e.ports, got, c.want)
		}
	}
	// How a row stands, which is the mark it takes and what its block
	// says of it. A wait comes before a fault, a fault before a down
	// row; a stopped row is a fault and is alive, and one that ended
	// with a code is a fault and is over.
	for _, c := range []struct {
		status string
		fault  bool
		want   int
	}{
		{statusWaiting, false, standWaiting},
		{statusStopped, true, standFault},
		{"UNHEALTHY", true, standFault},
		{"EXIT 3", true, standFault},
		{statusDown, false, standDown},
		{statusEnded, false, standOver},
		{"EXIT 3", false, standOver},
		{statusWorking, false, standWorking},
		{statusActive, false, standRests},
		{statusIdle, false, standRests},
	} {
		if got := stateOf(c.status, c.fault); got != c.want {
			t.Errorf("%s (fault %v) stands %d, want %d", c.status, c.fault, got, c.want)
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
	if !serving(folded[0].entries[1]) {
		t.Error("the server does not read as one")
	}
	if serving(folded[0].entries[0]) {
		t.Error("the shell that ran the server reads as a server itself")
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

// The eyebrow says what the project wants of you: a wait before a
// fault, a fault before a down row, counted where there is more than
// one of it, and nothing where the project wants nothing. It is the
// whole of the triage in one line a project rather than one a process,
// and it holds while the rows it is about are scrolled away.
func TestTheEyebrowSaysWhatTheProjectWants(t *testing.T) {
	row := func(status string, fault bool) processRow {
		return processRow{status: status, fault: fault}
	}
	for _, c := range []struct {
		rows    []processRow
		want    string
		stamped bool
	}{
		{[]processRow{row(statusIdle, false), row(statusWorking, false)}, "", false},
		{[]processRow{row(statusDown, false)}, "DOWN", false},
		{[]processRow{row(statusDown, false), row(statusDown, false)}, "2 DOWN", false},
		{[]processRow{row("EXIT 1", true), row(statusDown, false)}, "EXIT 1", true},
		{[]processRow{row("EXIT 1", true), row(statusStopped, true)}, "2 FAULTS", true},
		{[]processRow{row(statusWaiting, false), row("EXIT 1", true)}, "WAITING", true},
		{[]processRow{row(statusWaiting, false), row(statusWaiting, false)}, "2 WAITING", true},
	} {
		got, stamped, _ := verdict(c.rows)
		if got != c.want || stamped != c.stamped {
			t.Errorf("%d rows say %q (stamped %v), want %q (%v)", len(c.rows), got, stamped, c.want, c.stamped)
		}
	}
	// Drawn at the end of the rule, and a wait's blinks with the rows.
	m := newModel(plain)
	m.view, m.inside, m.width, m.height, m.now = viewProcesses, true, panelWidth, 20, processesNow
	m.projects = []project{
		{path: "/Users/w0zro/projects/w0zro/conn", entries: []entry{
			{pid: 1, kind: kindShell, command: "zsh", status: statusIdle},
		}},
		{path: "/Users/w0zro/projects/w0zro/vim.pro/conjurer", entries: []entry{
			{pid: 2, kind: kindRun, command: "api", status: statusDown},
			{pid: 3, kind: kindRun, command: "web", status: statusDown},
		}},
	}
	b := m.processesReport()
	b.lit = true
	text := texts(drawProcesses(b, 1, panelWidth, 20, plain))
	golden(t, "processes-verdicts-44x20.txt", text)
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "conn ─") && !strings.HasSuffix(strings.TrimRight(line, " "), "─") {
			t.Errorf("a project that wants nothing says something: %q", line)
		}
	}
	if !strings.Contains(text, " 2 DOWN") {
		t.Errorf("the project with nothing up does not say so:\n%s", text)
	}
	// Every block is at the margin: the folder the two projects share
	// is not a project and nothing is happening in it.
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "w0zro") {
			t.Errorf("the folder over the projects is a block of its own: %q", line)
		}
		if trimmed := strings.TrimRight(line, " "); trimmed != "" && !isPanelRow(trimmed) && !strings.HasPrefix(trimmed, "   ") {
			t.Errorf("a block is not at the margin: %q", line)
		}
	}
}

// isPanelRow says whether a trimmed panel line is a process row rather
// than a block's eyebrow: it begins with a dot, past the margin.
func isPanelRow(line string) bool {
	for _, f := range strings.Fields(line) {
		return f == dotWants || f == dotWorks || f == dotRests || f == dotOver || f == "▸" || f == cursorBar
	}
	return false
}

// A row keeps its port when the width is short: the port is where you
// would go, and the row is looked at for it. The command is elided
// beside it rather than the port being cut off behind it, and the
// project's name, which is the eyebrow's now and not every row's, is
// elided from the left where the rule has no room for it.
func TestARowKeepsItsPortWhenTheWidthIsShort(t *testing.T) {
	long := "/Volumes/work/some-organization-name/auditboard-backend-services"
	m := newModel(plain)
	m.view, m.inside, m.width, m.height = viewProcesses, true, panelWidth, 20
	m.projects = []project{{path: long, entries: []entry{
		{pid: 5, kind: kindRun, command: "pnpm start --host --strict-port", cwd: long, status: statusActive, ports: []string{"3000"}},
		{pid: 6, kind: kindRun, command: "node server.js", cwd: long, status: statusActive, ports: []string{"8080", "8081"}},
	}}}
	for _, width := range []int{panelWidth, 50, 40} {
		text := texts(drawProcesses(m.processesReport(), 5, width, 20, plain))
		if !strings.Contains(text, ":3000") || !strings.Contains(text, ":8080 :8081") {
			t.Errorf("at %d wide the rows lost their ports:\n%s", width, text)
		}
		if !strings.Contains(text, "pnpm") || !strings.Contains(text, "node") {
			t.Errorf("at %d wide the rows lost their commands:\n%s", width, text)
		}
	}
	// At the narrowest the command is what yields, and the port is
	// still there when it has.
	narrow := texts(drawProcesses(m.processesReport(), 5, 40, 20, plain))
	if !strings.Contains(narrow, "…") || !strings.Contains(narrow, ":3000") {
		t.Errorf("narrow, the command did not yield to the port:\n%s", narrow)
	}
}

// The right of a row is one column, and the ports end at it: a row of
// one port and a row of three finish level, down every project, so the
// ports are read down the panel rather than found along each row. A
// row with a word to stand by — waiting, at fault, over — says that
// word in the column instead, which is why the column is not the gap
// it would be if the ports had it to themselves: most rows either
// serve or have something to say, and the two never want it at once.
func TestThePortsEndAtOneColumn(t *testing.T) {
	m := newModel(plain)
	m.view, m.inside, m.width, m.height = viewProcesses, true, panelWidth, 20
	m.projects = []project{{path: "/Users/w0zro/projects/w0zro/conn", entries: []entry{
		{pid: 5, kind: kindRun, command: "node server.js", status: statusActive, ports: []string{"8080"}},
		{pid: 6, kind: kindRun, command: "caddy run", status: statusActive, ports: []string{"443", "80"}},
		{pid: 7, kind: kindShell, command: "zsh", status: statusIdle},
		{pid: 8, kind: kindRun, command: "npm run build", status: statusDown, ports: []string{"4000"}},
	}}}
	ends := map[string]int{}
	var serves, quiet, ended string
	for _, line := range strings.Split(texts(drawProcesses(m.processesReport(), 0, panelWidth, 20, plain)), "\n") {
		for _, at := range []struct {
			word string
			line *string
		}{{"node", &serves}, {"caddy", &quiet}, {"zsh", nil}, {"npm", &ended}} {
			if !strings.Contains(line, at.word) {
				continue
			}
			if at.line != nil {
				*at.line = line
			}
			ends[at.word] = utf8.RuneCountInString(strings.TrimRight(line, " "))
		}
	}
	if serves == "" || quiet == "" || ended == "" {
		t.Fatalf("the rows are missing:\n%q\n%q\n%q", serves, quiet, ended)
	}
	// One port, three ports, and a word in the column's place all
	// finish where the panel finishes.
	if ends["node"] != ends["caddy"] || ends["caddy"] != ends["npm"] {
		t.Errorf("the column is ragged: %v\n%q\n%q\n%q", ends, serves, quiet, ended)
	}
	if !strings.HasSuffix(serves, ":8080") || !strings.HasSuffix(quiet, ":443 :80") {
		t.Errorf("the ports are not at the end:\n%q\n%q", serves, quiet)
	}
	// A row with nothing to say and nothing open ends at its command,
	// and the commands all start together whatever stands at the end.
	if ends["zsh"] >= ends["node"] {
		t.Errorf("a row with no port was carried out to the column: %v", ends)
	}
	if strings.Index(quiet, "caddy") != strings.Index(serves, "node") {
		t.Errorf("the commands do not start together:\n%q\n%q", serves, quiet)
	}
}

// The word a row stands by takes the column from its port: a process
// that is down is down, and where it would have gone when it was up is
// not the thing to say about it.
func TestARowsWordTakesTheColumnFromItsPort(t *testing.T) {
	m := newModel(plain)
	m.view, m.inside, m.width, m.height, m.now = viewProcesses, true, panelWidth, 20, processesNow
	m.projects = []project{{path: "/Users/w0zro/projects/w0zro/conn", entries: []entry{
		{pid: 5, kind: kindRun, command: "npm run build", status: statusDown, ports: []string{"4000"}},
		{pid: 6, kind: kindContact, command: "claude", status: statusWaiting, since: processesNow.Add(-9 * time.Minute), ports: []string{"7000"}},
	}}}
	b := m.processesReport()
	for _, lit := range []bool{true, false} {
		b.lit = lit
		text := texts(drawProcesses(b, 0, panelWidth, 20, plain))
		if !strings.Contains(text, "DOWN") {
			t.Errorf("lit %v: the down row lost its word:\n%s", lit, text)
		}
		// On the blink's dark half the column is still the word's: a
		// port coming up in the gap would make the row say one thing
		// and then the other, second by second.
		if strings.Contains(text, ":4000") || strings.Contains(text, ":7000") {
			t.Errorf("lit %v: a port stands where the row's word does:\n%s", lit, text)
		}
	}
}

// A wait is stamped with how long, and the stamp blinks as the wide
// view's WAITING does: on the dark half its cells are the ground and
// the rest of the row holds still. A fault's stamp holds still on
// both halves.
func TestTheWaitingStampBlinksOnThePanel(t *testing.T) {
	m := newModel(plain)
	m.view, m.inside, m.width, m.height, m.now = viewProcesses, true, panelWidth, 20, processesNow
	m.projects = []project{{path: "/Users/w0zro/projects/w0zro/conn", entries: []entry{
		{pid: 1, kind: kindContact, command: "claude", status: statusWaiting, since: processesNow.Add(-9 * time.Minute)},
		{pid: 2, kind: kindEditor, command: "vim", status: statusStopped, fault: true},
	}}}
	b := m.processesReport()
	b.lit = true
	on := drawProcesses(b, 0, panelWidth, 20, plain)
	b.lit = false
	off := drawProcesses(b, 0, panelWidth, 20, plain)
	if !strings.Contains(texts(on), " 9 MIN") || strings.Contains(texts(on), "9 min") {
		t.Errorf("the lit half does not stamp the wait:\n%s", texts(on))
	}
	if strings.Contains(texts(off), "9 MIN") {
		t.Errorf("the dark half still says it:\n%s", texts(off))
	}
	if !strings.Contains(texts(off), " STOPPED") {
		t.Errorf("the fault's stamp went dark with the wait's:\n%s", texts(off))
	}
	if len(on) != len(off) {
		t.Fatalf("the halves are %d rows and %d", len(on), len(off))
	}
	// Only what the wait is stamped on differs between the halves:
	// the row, and the eyebrow saying the project is waiting.
	for i := range on {
		if lit, dark := on[i].text, off[i].text; lit != dark && !strings.Contains(lit, "9 MIN") && !strings.Contains(lit, statusWaiting) {
			t.Errorf("row %d moved between the halves:\n%q\n%q", i, lit, dark)
		}
	}
}

// Nothing to list is said on the panel as it is in the tree. The panel
// drew nothing at all here, which is what a panel that has failed to
// draw looks like.
func TestThePanelSaysWhenThereIsNothingToList(t *testing.T) {
	for _, filed := range []bool{true, false} {
		empty := composeProcesses(nil, nil, "", testProjRoots, testIsProject, "/Users/w0zro", processesNow, "", false, filed)
		if out := texts(drawProcesses(empty, 0, panelWidth, 20, plain)); !strings.Contains(out, "NO PROCESSES") {
			t.Errorf("nothing running, filed %v, says nothing:\n%s", filed, out)
		}
		unread := composeProcesses(nil, nil, "", testProjRoots, testIsProject, "/Users/w0zro", processesNow, "the process table could not be read: lsof: not found", false, filed)
		unread.stalled = true
		out := texts(drawProcesses(unread, 0, panelWidth, 20, plain))
		if !strings.Contains(out, "LSOF: NOT FOUND") {
			t.Errorf("the table could not be read, filed %v, and the view says nothing:\n%s", filed, out)
		}
		if !strings.Contains(out, "DOCKER NOT ANSWERING") {
			t.Errorf("the notes are lost where there are no rows, filed %v:\n%s", filed, out)
		}
		if len(drawProcesses(unread, 0, panelWidth, 20, plain)) != 20 {
			t.Errorf("the drawing is not the height it was given, filed %v", filed)
		}
	}
}
