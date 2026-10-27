package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// A conversation running in a terminal conn did not start is a row on
// the panel, and enter on it brings it in: at rest, the foot asks
// "bring it in? it ends in its terminal", y ends it there and puts it
// in the workspace with the keys in it, esc leaves it be; tab then
// reaches it as it does any waiting conversation, and the title and the
// bell count it as they did. Mid-turn, the foot says it can come in
// when its turn ends, and nothing is ended. What is read is the
// machine, as the panel reads it: tmux's panes and Claude Code's
// instance files and transcripts; the other terminal is a sleep, and
// the claude that picks the conversation back up is a stand-in that
// writes its instance file as Claude Code would.
func TestAConversationFromAnotherTerminalComesIn(t *testing.T) {
	shots, g := darkShots(t, 0)

	bin, scratch := tmuxScratch(t)
	t.Setenv("LC_ALL", "C.UTF-8")
	web := newRepo(t, filepath.Join(scratch, "web"))

	const title = "Move the flag into config"
	s1 := "aaaaaaaa-0000-0000-0000-000000000001"
	write(t, transcriptPath(scratch, web, s1),
		`{"type":"user","cwd":`+strconv.Quote(web)+`,"message":{"role":"user","content":"move the flag into config"}}`+"\n"+
			`{"type":"ai-title","aiTitle":`+strconv.Quote(title)+`}`+"\n")

	// The claude a pane picks a conversation back up with: it says it is
	// waiting, as Claude Code does in its instance file, and stays.
	stubs := filepath.Join(scratch, "stubs")
	write(t, filepath.Join(stubs, "claude"), "#!/bin/sh\n"+
		"while [ $# -gt 0 ]; do [ \"$1\" = --resume ] && id=\"$2\"; shift; done\n"+
		"printf '{\"sessionId\":\"%s\",\"status\":\"waiting\",\"cwd\":\"%s\"}' \"$id\" \"$PWD\" > \"$CLAUDE_CONFIG_DIR/sessions/$$.json\"\n"+
		"exec sleep 1000\n")
	if err := os.Chmod(filepath.Join(stubs, "claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubs+string(os.PathListSeparator)+os.Getenv("PATH"))

	srv, ws, own := panelWindow(t, bin, scratch, g)

	// A dev server runs in conn, a row to go to and tab from.
	devPane, err := startNamed(srv, "dev", "exec sleep 1000", web, web, roleStack)
	if err != nil {
		t.Fatal(err)
	}

	// The conversation runs in another terminal: a sleep conn did not
	// start, its instance file saying what Claude Code would.
	other := exec.Command("sleep", "1000")
	if err := other.Start(); err != nil {
		t.Fatal(err)
	}
	ended := make(chan struct{})
	go func() { _ = other.Wait(); close(ended) }()
	t.Cleanup(func() { _ = other.Process.Kill() })
	instance := filepath.Join(scratch, ".claude", "sessions", strconv.Itoa(other.Process.Pid)+".json")
	status := func(s string) {
		write(t, instance, `{"sessionId":"`+s1+`","status":"`+s+`","cwd":"`+web+`"}`)
	}
	stillThere := func() bool {
		select {
		case <-ended:
			return false
		default:
			return syscall.Kill(other.Process.Pid, 0) == nil
		}
	}

	// The scratch is temporary, which is no project; here it is one.
	k := knownRoots(web)

	read := func(m model) model { return pump(m, m.read(), 8) }
	press := func(m model, k tea.Key) model {
		next, c := m.Update(tea.KeyPressMsg(k))
		return pump(next.(model), c, 8)
	}
	typed := func(m model, r rune) model { return press(m, tea.Key{Code: r, Text: string(r)}) }
	// The web panel, read from the machine, the cursor on the
	// conversation from the other terminal.
	panel := func() model {
		m := model{srv: srv, ws: ws, own: own, screen: onProject, home: scratch, conn: "conn", known: k, g: g, loaded: true,
			cur: project{Name: "web", Root: web}, width: panelWidth, height: 34}
		m = read(m)
		for i, it := range m.items() {
			if it.row >= 0 && m.rows[it.row].elsewhere {
				m.cursor = i
				return m
			}
		}
		t.Fatalf("web's panel has no row for the conversation in the other terminal:\n%s", m.View().Content)
		return m
	}

	// The foot is what stands under the panel; the bar is the key bar
	// across the window's foot, as tmux has it.
	// What conn says stands on the band, written there from the draw.
	says := func(m model) string {
		_, f := m.draw()
		return f.say
	}
	bar := func() string {
		got, _ := srv.Global(barMark)
		return strings.Join(strings.Fields(plainText(got)), " ")
	}
	titleAndBell := func() string {
		got, err := srv.Run("display", "-p", "-t", sessionName, "#{T:set-titles-string} #{window_bell_flag}")
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(got)
	}
	held := func() *pane {
		ps, _ := panes(srv)
		for i := range ps {
			if ps[i].Session == s1 {
				return &ps[i]
			}
		}
		return nil
	}

	frame := func(name string, v model) {
		if !shots.on() {
			return
		}
		shots.note("title-and-bell.txt", fmt.Sprintf("%s: %s", name, titleAndBell()))
		shots.panel(name, v, srv, true)
	}

	// Still working, it can come in when its turn ends, and nothing is
	// ended, whatever is pressed after.
	status("busy")
	m := panel()
	frame("120x36.ans", m)
	m = press(m, tea.Key{Code: tea.KeyEnter})
	frame("working-120x36.ans", m)
	const working = "working; it can come in when its turn ends"
	if got := says(m); !strings.Contains(got, working) {
		t.Errorf("enter on a conversation still at work in another terminal, the band says %q, want %q", got, working)
	}
	_ = typed(m, 'y') // what it does is read off the machine below
	time.Sleep(500 * time.Millisecond)
	if !stillThere() {
		t.Errorf("a conversation still at work in another terminal was ended")
	}
	if p := held(); p != nil {
		t.Errorf("a conversation still at work in another terminal was brought in, to %s", p.ID)
	}

	// At rest, enter asks.
	status("waiting")
	m = panel()
	before := titleAndBell()
	if before == "" {
		t.Errorf("the tab has no title")
	}
	m = press(m, tea.Key{Code: tea.KeyEnter})
	frame("ask-120x36.ans", m)
	const ask = "bring it in? it ends in its terminal"
	if got := says(m); !strings.Contains(got, ask) {
		t.Fatalf("enter on a conversation at rest in another terminal, the band says %q, want %q", got, ask)
	}
	for _, want := range []string{"y bring it in", "esc leave it"} {
		if got := bar(); !strings.Contains(got, want) {
			t.Errorf("while it asks, the key bar says %q, want %q", got, want)
		}
	}
	time.Sleep(300 * time.Millisecond)
	if !stillThere() || held() != nil {
		t.Fatalf("asking ended the conversation in the other terminal, or brought it in")
	}

	// esc leaves it where it is.
	m = press(m, tea.Key{Code: tea.KeyEscape})
	if got := says(m); strings.Contains(got, ask) {
		t.Errorf("after esc the foot still asks: %q", got)
	}
	time.Sleep(300 * time.Millisecond)
	if !stillThere() || held() != nil {
		t.Fatalf("esc ended the conversation in the other terminal, or brought it in")
	}

	// A yes ends it there and shows it in conn's window, the keys in it.
	m = panel()
	m = press(m, tea.Key{Code: tea.KeyEnter})
	if got := says(m); !strings.Contains(got, ask) {
		t.Fatalf("enter again, the foot says %q, want %q", got, ask)
	}
	m = typed(m, 'y')
	var in *pane
	for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		m = read(m)
		if p := held(); p != nil && !stillThere() && m.in == p.ID {
			if shown, ok, _ := ws.shown(); ok && shown.ID == p.ID {
				in = p
				break
			}
		}
	}
	if stillThere() {
		t.Errorf("y did not end the conversation in its terminal")
	}
	if in == nil {
		t.Fatalf("y did not bring the conversation into conn's window: held %v, keys in %q", held(), m.in)
	}
	for deadline := time.Now().Add(5 * time.Second); len(m.waiting) == 0 && time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		m = read(m)
	}
	frame("brought-120x36.ans", m)
	if after := titleAndBell(); after != before {
		t.Errorf("bringing it in changed the tab's title and bell, from %q to %q", before, after)
	}
	for _, r := range m.rows {
		if r.elsewhere {
			t.Errorf("the conversation is still a row from another terminal")
		}
	}

	// tab reaches it like any waiting conversation, from another row.
	m = read(m)
	m = pump(m, m.showing(devPane, true), 8)
	m.in = devPane
	m = press(m, tea.Key{Code: tea.KeyTab})
	if m.in != in.ID {
		t.Errorf("tab went into %q, want the conversation brought in, %s", m.in, in.ID)
	}
}
