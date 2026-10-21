package main

import (
	"os"
	"path/filepath"
	"strconv"
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
	o := offerModel{as: roleStack, p: project{Name: "d", Root: "/p"}, asking: true, pending: map[string]bool{"/p": true}, width: 80, height: 24}
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
	o.asking, o.pending, o.again = true, map[string]bool{"/p": true}, []failure{{stacked: stacked{name: "test", command: "make test"}, pane: "%1", exit: "127", as: roleCheck}}
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

// A folder of repositories is a project whose stack and checks are its
// repositories': what each has written down is listed under it, named
// by the repository; each with nothing written down is asked about in
// a question of its own, and the answers join the list, each line
// under its repository. x on a line Claude named only takes it off the
// list.
func TestFolderAsksEachRepository(t *testing.T) {
	srv, _, _ := scratchServer(t)
	folder := t.TempDir()
	a, b := filepath.Join(folder, "api"), filepath.Join(folder, "web")
	for _, r := range []string{a, b} {
		if err := os.MkdirAll(filepath.Join(r, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := recordStart(startedPath(srv.Socket), started{Root: a, Name: "test", Command: "make test", Role: roleCheck}); err != nil {
		t.Fatal(err)
	}
	f := project{Name: "f", Root: folder, Group: true}
	o := newOffer(srv, roleCheck, f, folder, "")
	o.width, o.height = 100, 30
	if !o.written || len(o.list) != 1 || o.list[0].root != a || !o.asking || !o.pending[b] || o.pending[a] || o.Init() == nil {
		t.Fatalf("the folder's question: written %v, list %+v, asking %v, pending %v", o.written, o.list, o.asking, o.pending)
	}
	screen := func() string {
		lines, _ := o.draw()
		return plainText(strings.Join(lines, "\n"))
	}
	if s := screen(); !strings.Contains(s, "api · test") || !strings.Contains(s, "asking Claude what checks in web") {
		t.Errorf("the screen while web is asked:\n%s", s)
	}
	// An answer for a repository not asked is nothing.
	next, _ := o.Update(roleAnswer{root: folder, as: roleCheck, list: []stacked{{name: "x", command: "y"}}})
	if o = next.(offerModel); len(o.list) != 1 {
		t.Errorf("an answer for the folder itself was listed: %+v", o.list)
	}
	next, _ = o.Update(roleAnswer{root: b, as: roleCheck, list: []stacked{{name: "lint", command: "npm run lint"}}})
	o = next.(offerModel)
	if o.asking || len(o.list) != 2 || o.list[1].root != b || o.list[1].kept {
		t.Fatalf("after web's answer: asking %v, list %+v", o.asking, o.list)
	}
	if s := screen(); !strings.Contains(s, "web · lint") || strings.Contains(s, "asking Claude") {
		t.Errorf("the screen with both in:\n%s", s)
	}
	// x on web's line, which Claude named, takes it off the list and
	// writes nothing down; u puts it back.
	o.at = 1
	o, _ = pressOffer(t, o, "x")
	if len(o.list) != 1 || !strings.Contains(o.note, "off the list") {
		t.Errorf("x on an answered line: list %+v, note %q", o.list, o.note)
	}
	if list := established(readStarted(startedPath(srv.Socket)), f.belongs, roleOff); len(list) != 0 {
		t.Errorf("x on an answered line wrote it down as off: %+v", list)
	}
	if o, _ = pressOffer(t, o, "u"); len(o.list) != 2 {
		t.Errorf("u did not put it back: %+v", o.list)
	}
	// Every repository answering nothing is why the question could
	// not be answered; one of two answering nothing is said under the
	// list.
	o = newOffer(srv, roleStack, f, folder, "")
	next, _ = o.Update(roleAnswer{root: a, as: roleStack, err: "the answer named nothing"})
	next, _ = next.(offerModel).Update(roleAnswer{root: b, as: roleStack, err: "no answer in 1m30s"})
	if o = next.(offerModel); o.asking || o.err != "nothing named in api; web: no answer in 1m30s" {
		t.Errorf("with nothing answered: asking %v, err %q", o.asking, o.err)
	}
	o = newOffer(srv, roleStack, f, folder, "")
	next, _ = o.Update(roleAnswer{root: a, as: roleStack, err: "the answer named nothing"})
	next, _ = next.(offerModel).Update(roleAnswer{root: b, as: roleStack, list: []stacked{{name: "dev", command: "npm run dev"}}})
	o = next.(offerModel)
	o.width, o.height = 100, 30
	if s := screen(); o.err != "" || !strings.Contains(s, "web · dev") || !strings.Contains(s, "nothing named in api") {
		t.Errorf("with one answered: err %q, screen:\n%s", o.err, s)
	}
}

// A list longer than the pane is shown in a window around the cursor's
// line, the question and the keys standing; the answers of a folder's
// repositories stand by repository, after what is written down.
func TestLongListIsWindowed(t *testing.T) {
	o := offerModel{as: roleCheck, p: project{Name: "d", Root: "/p"}, written: true, width: 80, height: 12}
	for i := 0; i < 30; i++ {
		o.list = append(o.list, stacked{name: "c" + strconv.Itoa(i), command: "make " + strconv.Itoa(i), root: "/p", kept: true})
	}
	o.at = 20
	lines := strings.Split(plainText(o.View().Content), "\n")
	if len(lines) != 12 || !strings.Contains(lines[3], "Run these checks in d?") || !strings.Contains(lines[11], "enter") {
		t.Fatalf("the question and the keys do not stand: %d lines\n%s", len(lines), strings.Join(lines, "\n"))
	}
	s := strings.Join(lines, "\n")
	if !strings.Contains(s, cursorBar+"  c20 ") || strings.Contains(s, " c0 ") || strings.Contains(s, " c29 ") {
		t.Errorf("the window does not stand around the cursor:\n%s", s)
	}
	f := project{Name: "f", Root: "/f", Group: true}
	o = offerModel{as: roleCheck, p: f, list: []stacked{{name: "k", command: "x", root: "/f/b", kept: true}, {name: "z", command: "x", root: "/f/b"}, {name: "a", command: "x", root: "/f/a"}}}
	o.order()
	if o.list[0].name != "k" || o.list[1].root != "/f/a" && o.list[1].root != "/f/b" {
		t.Errorf("the order: %+v", o.list)
	}
}
