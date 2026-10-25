package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/w0zro/conn/internal/theme"
	"github.com/w0zro/conn/internal/tmux"
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
	bin, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("no tmux")
	}
	shots := os.Getenv("SHOTS")
	wear(theme.Dark)
	t.Cleanup(undress)
	g := theme.Dark

	scratch, err := os.MkdirTemp("/tmp", "cs-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(scratch) })
	scratch, _ = filepath.EvalSymlinks(scratch)
	t.Setenv("HOME", scratch)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(scratch, ".claude"))
	t.Setenv("LC_ALL", "C.UTF-8")
	web := filepath.Join(scratch, "web")
	if err := os.MkdirAll(web, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, web, "init", "-q")
	git(t, web, "commit", "-q", "--allow-empty", "-m", "first")

	const title = "Move the flag into config"
	s1 := "aaaaaaaa-0000-0000-0000-000000000001"
	encoded := regexp.MustCompile("[^A-Za-z0-9]").ReplaceAllString(web, "-")
	write(t, filepath.Join(scratch, ".claude", "projects", encoded, s1+".jsonl"),
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

	path := write(t, filepath.Join(scratch, "tmux.conf"), conf(g))
	srv := &tmux.Server{Tmux: bin, Socket: filepath.Join(scratch, "s")}
	t.Cleanup(func() { _ = srv.Down() })
	out, err := srv.Run("-f", path, "new-session", "-d", "-s", sessionName, "-x", "120", "-y", "36", "-c", scratch, "-P", "-F", "#{window_id} #{pane_id}", "sleep 1000")
	if err != nil {
		t.Fatal(err)
	}
	win, own, _ := strings.Cut(strings.TrimSpace(out), " ")
	if _, err := srv.Do(tmux.SetPane(own, panelMark, "1")); err != nil {
		t.Fatal(err)
	}
	ws := newWorkspace(srv, win, own, "/x")
	ws.hold = "sleep 1000"

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
	k := newKnown()
	k.root[web] = web

	// pump runs what a key or a read asks for, and what that asks for in
	// turn, as the program would.
	var pump func(m model, c tea.Cmd, depth int) model
	pump = func(m model, c tea.Cmd, depth int) model {
		if c == nil || depth > 8 {
			return m
		}
		got := make(chan tea.Msg, 1)
		go func() { got <- c() }()
		var msg tea.Msg
		select {
		case msg = <-got:
		case <-time.After(2 * time.Second):
			return m
		}
		if msg == nil {
			return m
		}
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, b := range batch {
				m = pump(m, b, depth+1)
			}
			return m
		}
		next, more := m.Update(msg)
		return pump(next.(model), more, depth+1)
	}
	read := func(m model) model { return pump(m, m.read(), 0) }
	press := func(m model, k tea.Key) model {
		next, c := m.Update(tea.KeyPressMsg(k))
		return pump(next.(model), c, 0)
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
	styles := regexp.MustCompile(`#\[[^\]]*\]`)
	bar := func() string {
		got, _ := srv.Global(barMark)
		return strings.Join(strings.Fields(styles.ReplaceAllString(got, "")), " ")
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

	var said strings.Builder
	frame := func(name string, v model) {
		if shots == "" {
			return
		}
		fmt.Fprintf(&said, "%s: %s\n", name, titleAndBell())
		if err := os.WriteFile(filepath.Join(shots, "title-and-bell.txt"), []byte(said.String()), 0o644); err != nil {
			t.Error(err)
		}
		var b strings.Builder
		fmt.Fprintf(&b, "\x1b]10;%s\x1b\\\x1b]11;%s\x1b\\\x1b[2J\x1b[H", theme.Hex(g.Ink), g.Surface)
		for _, l := range strings.Split(v.View().Content, "\n") {
			b.WriteString(l + "\x1b[0m" + strings.Repeat(" ", max(120-ansi.StringWidth(l), 0)) + "\r\n")
		}
		b.WriteString(drawnStatus(t, srv))
		if err := os.WriteFile(filepath.Join(shots, name), []byte(b.String()+"\x1b[0m"), 0o644); err != nil {
			t.Error(err)
		}
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
	m = pump(m, m.showing(devPane, true), 0)
	m.in = devPane
	m = press(m, tea.Key{Code: tea.KeyTab})
	if m.in != in.ID {
		t.Errorf("tab went into %q, want the conversation brought in, %s", m.in, in.ID)
	}
}
