package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// ss and st put their question in the workspace: a pane of conn's own
// running conn offer for the role, marked as a hold, followed there
// while it stands. Over, with the first pane it started on it, that
// pane takes the cursor; over with none, the workspace follows the
// cursor again.
func TestResolveStandsInTheWorkspace(t *testing.T) {
	srv, _, _ := scratchServer(t)
	// The question's pane stays once its command has ended, so the
	// test can read it.
	if _, err := srv.Run("set-option", "-g", "remain-on-exit", "on"); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(srv.Socket)
	m := model{srv: srv, self: "/x/conn", screen: onProject, cur: project{Name: "d", Root: dir}, rows: []row{{pane: &pane{ID: "%9"}}}}
	m, _ = press(t, m, "s")
	m, cmd := press(t, m, "t")
	if m.offering == "" || m.follow != m.offering || cmd == nil {
		t.Fatalf("st opened no question: offering %q, follow %q", m.offering, m.follow)
	}
	out, err := srv.Run("list-panes", "-s", "-t", sessionName, "-F", "#{pane_id} #{"+holdMark+"} #{pane_start_command}")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, m.offering+" 1 ") && strings.Contains(l, "/x/conn") && strings.Contains(l, " offer check ") && strings.Contains(l, dir) {
			found = true
		}
	}
	if !found {
		t.Errorf("the question's pane %s is not a hold running conn offer:\n%s", m.offering, out)
	}
	if m.wanted() != m.offering {
		t.Errorf("the workspace is to show %q, want the question", m.wanted())
	}
	// A read while it stands keeps following it, row or none.
	at := time.Now().Add(time.Second)
	next, _ := m.Update(read{at: at, root: dir, rows: m.rows, offerUp: true})
	if m = next.(model); m.follow != m.offering {
		t.Errorf("a read while the question stands let go of it: follow %q", m.follow)
	}
	// Over, having started %4: the cursor lands on %4's row.
	rows := []row{{pane: &pane{ID: "%9"}}, {pane: &pane{ID: "%4", Name: "test"}}}
	next, _ = m.Update(read{at: at, root: dir, rows: rows, first: "%4"})
	m = next.(model)
	if r, ok := m.under(); m.offering != "" || !ok || r.pane == nil || r.pane.ID != "%4" || m.follow != "" {
		t.Errorf("over, the first pane started is not under the cursor: offering %q, follow %q, under %+v", m.offering, m.follow, r.pane)
	}
	// Over having started nothing: the workspace follows the cursor.
	m, _ = press(t, m, "s")
	m, _ = press(t, m, "s")
	next, _ = m.Update(read{at: at, root: dir, rows: rows})
	if m = next.(model); m.offering != "" || m.follow != "" || m.wanted() != "%4" {
		t.Errorf("over with nothing started: offering %q, follow %q, wanted %q", m.offering, m.follow, m.wanted())
	}
}

// While Claude Code's answer stands, enter and esc are the keys; j
// does not move, since the answer has no cursor, and an answer that
// comes after esc is not shown.
func TestOfferKeys(t *testing.T) {
	o := offerModel{as: roleStack, p: project{Name: "d", Root: "/p"}, asking: true, width: 80, height: 24}
	next, _ := o.Update(roleAnswer{root: "/p", as: roleStack, list: []stacked{{name: "web", command: "npm run dev"}}})
	o = next.(offerModel)
	if o.asking || len(o.list) != 1 || o.written {
		t.Fatalf("the answer was not offered: %+v", o)
	}
	if o, _ = pressOffer(t, o, "j"); o.at != 0 {
		t.Errorf("j moved the cursor on the answer: at %d", o.at)
	}
	if lines, keys := o.draw(); !strings.Contains(strings.Join(lines, "\n"), "Start these in d?") || len(keys) != 2 || keys[0].key != "enter" || keys[1].key != "esc" {
		t.Errorf("the answer's screen: %q, keys %v", lines, keys)
	}
	if _, cmd := pressOffer(t, o, "esc"); cmd == nil {
		t.Error("esc did not leave")
	}
}

// A start that fails at once is asked about again, once nothing is
// still watched; one that fails later, or ends well, or is still going
// past the watch, is let go of. Claude saying the command is right
// leaves a last word, and enter or esc closes on it.
func TestFailedAtOnceIsAskedAgain(t *testing.T) {
	start := time.Now()
	o := offerModel{as: roleCheck, p: project{Name: "d", Root: "/p"}, state: map[string]string{}, width: 80, height: 24}
	for _, n := range []string{"test", "lint", "build"} {
		o.list = append(o.list, stacked{name: n, command: "make " + n})
		o.runs = append(o.runs, watched{project: "/p", stacked: o.list[len(o.list)-1], as: roleCheck, at: start})
	}
	soon, late := start.Add(time.Second), start.Add(10*time.Second)
	// The old test, ended before the start, is not the run.
	ps := []pane{
		{ID: "%0", Name: "test", Dir: "/p", Exit: "1", Ended: start.Add(-time.Minute)},
		{ID: "%1", Name: "test", Dir: "/p", Exit: "127", Ended: soon},
		{ID: "%2", Name: "lint", Dir: "/p"},
		{ID: "%3", Name: "build", Dir: "/p"},
	}
	next, cmd := o.watched(ps, soon)
	o = next.(offerModel)
	if o.asking || len(o.runs) != 2 || cmd == nil {
		t.Fatalf("asked before the watch was over: asking %v, watching %d", o.asking, len(o.runs))
	}
	if lines, _ := o.draw(); !strings.Contains(strings.Join(lines, "\n"), "failed at once, exited 127") {
		t.Errorf("the screen does not say test failed: %q", lines)
	}
	ps[2] = pane{ID: "%2", Name: "lint", Dir: "/p", Exit: "1", Ended: late}
	next, cmd = o.watched(ps, late)
	o = next.(offerModel)
	if !o.asking || cmd == nil || len(o.again) != 1 || o.again[0].name != "test" || o.again[0].pane != "%1" || len(o.runs) != 0 {
		t.Fatalf("asking %v, again %+v, watching %d", o.asking, o.again, len(o.runs))
	}
	if lines, _ := o.draw(); !strings.Contains(strings.Join(lines, "\n"), "test failed at once; asking Claude again") {
		t.Errorf("the screen does not say why it asks: %q", lines)
	}
	// Claude says the command is right: the last word, and enter closes.
	next, _ = o.Update(roleAnswer{root: "/p", as: roleCheck, kept: true})
	o = next.(offerModel)
	if o.asking || o.again != nil || !strings.Contains(o.over, "the failure is the project's") {
		t.Errorf("kept: asking %v, over %q", o.asking, o.over)
	}
	if _, cmd := pressOffer(t, o, "enter"); cmd == nil {
		t.Error("enter on the last word did not close")
	}
	// Claude gives a command instead: offered under the same name, and
	// enter starts it again.
	o.over = ""
	o.asking, o.again = true, []failure{{stacked: stacked{name: "test", command: "make test"}, pane: "%1", exit: "127", as: roleCheck}}
	next, _ = o.Update(roleAnswer{root: "/p", as: roleCheck, list: []stacked{{name: "test", command: "go test ./..."}}})
	o = next.(offerModel)
	if lines, keys := o.draw(); !strings.Contains(strings.Join(lines, "\n"), "Run these instead?") || keys[0].key != "enter" {
		t.Errorf("the corrected command is not offered instead: %q, keys %v", lines, keys)
	}
}

// Over, the question puts the keys back on the panel: its leaving is a
// command, run off the loop, that ends with quitting.
func TestOfferLeaves(t *testing.T) {
	o := offerModel{as: roleStack, p: project{Name: "d", Root: "/p"}, err: "no answer"}
	if _, cmd := o.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape})); cmd == nil {
		t.Error("esc on an error did not leave")
	}
}
