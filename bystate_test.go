package main

import (
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
	if want := "WAITING FOR YOU:52 WORKING:7 OPEN:14 NOT RUNNING:6"; strings.Join(got, " ") != want {
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
	if back := unfiled(out); len(back) != 2 || back[0].path != "/w/b" || len(back[0].entries) != 2 || back[1].path != "/w/a" || len(back[1].entries) != 4 {
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
	for _, want := range []string{"WAITING FOR YOU ─", "─ 2", "WORKING ─", "OPEN ─", "NOT RUNNING ─",
		"●  claude", "9 min", "2 min", "●  go test ./... ◐", "○  zsh", "◌  worker", " Stopped"} {
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
	has := func(bar, key, does string) bool { return strings.Contains(bar, key+" #[nobold fg="+grayHex+"]"+does) }
	m.cursor = 1
	bar := m.bar()
	for _, want := range [][2]string{{"j k", "Move"}, {"Enter", "Open"}, {"Tab", "Next waiting"}, {"x", "End it"}, {"s", "Shell"}, {"u", "Bring up"}, {"?", "Help"}} {
		if !has(bar, want[0], want[1]) {
			t.Errorf("on the shell the bar lacks %s %s:\n%s", want[0], want[1], bar)
		}
	}
	m.cursor = 2 // a contact in no pane conn holds: nowhere to go into
	if bar := m.bar(); has(bar, "Enter", "Open") || !has(bar, "x", "End it") {
		t.Errorf("on an unreachable contact the bar offers %s", bar)
	}
	m.cursor = -9 // declared and down: brought up, not ended
	if bar := m.bar(); !has(bar, "Enter", "Bring it up") || has(bar, "x", "End it") {
		t.Errorf("on a down declaration the bar offers %s", bar)
	}
	all := unfiled(m.projects)
	for i := range all[0].entries {
		if all[0].entries[i].pid == 2 {
			all[0].entries[i].status = statusIdle
		}
	}
	m.projects = byState(all)
	if bar := m.bar(); has(bar, "Tab", "Next waiting") {
		t.Errorf("with nothing waiting the bar offers tab:\n%s", bar)
	}
	m.inside = false
	if bar := m.bar(); has(bar, "s", "Shell") || has(bar, "u", "Bring up") {
		t.Errorf("outside the server the bar offers keys that need it:\n%s", bar)
	}
	// With nothing running there is no row and so no project to act at:
	// the list and the manual are what is left.
	empty := model{view: viewProcesses, inside: true, focused: true}
	if bar := empty.bar(); has(bar, "s", "Shell") || has(bar, "a", "New contact") || !has(bar, "p", "Projects") || !has(bar, "?", "Help") {
		t.Errorf("with no rows the bar offers %s", bar)
	}
	m.inside, m.focused = true, false
	if bar := m.bar(); !has(bar, "ctrl-space -", "Panel") || !has(bar, "ctrl-space ?", "Help") || has(bar, "x", "End it") {
		t.Errorf("with the keys in a process the bar offers %s", bar)
	}
	m.focused = true
	m.kill = &pendingKill{prompt: "kill -TERM 1 · zsh?"}
	if bar := m.bar(); !strings.Contains(bar, "CONFIRM") || !strings.Contains(bar, "kill -TERM 1 · zsh?") || !has(bar, "y", "Yes") {
		t.Errorf("a question armed puts %s on the bar", bar)
	}
	// The clock at the right edge of the band, as a mission clock reads.
	m.up, m.now = processesNow.Add(-(5*24*time.Hour + 2*time.Hour + 14*time.Minute)), processesNow
	if up := m.upWord(); !strings.Contains(up, "T+ 5d 02h 14m ") {
		t.Errorf("the clock reads %q", up)
	}
}
