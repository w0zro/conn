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
