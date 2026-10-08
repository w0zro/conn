package main

import (
	"strings"
	"testing"

	"github.com/w0zro/conn/internal/work"

	"github.com/w0zro/conn/internal/tmux"

	"github.com/w0zro/conn/internal/theme"
)

// The left of the status line says which view has the keys, or the
// question armed over it, and nothing else of conn's. It is written
// when it changes and not again for the same view.
func TestConnLightsTheStatusLine(t *testing.T) {
	g := theme.Conn.Dark
	m := plainModel()
	m.inside, m.srv = true, &tmux.Server{Tmux: "/nonexistent/tmux", Socket: "/tmp/none"}
	m.projects = []work.Project{{Path: "/w", Entries: []work.Entry{
		{PID: 11, Kind: work.KindContact, Command: "claude", TTY: "ttys004", Status: work.StatusWaiting},
	}}}

	// Each panel view wears the wordmark, the band being the station's
	// and the view saying itself by its eyebrows; the console wears
	// none, covering the window with a wordmark of its own.
	for _, v := range []view{viewProcesses, viewProjects, viewSessions} {
		m.view = v
		if keys := m.keys(); keys != statusWord(wordmarkLine, theme.Hex(g.Ink), true, g) {
			t.Errorf("view %d lights %q, not the wordmark", v, keys)
		}
	}

	m.view = viewConsole
	if keys := m.keys(); keys != "" {
		t.Errorf("the console lights %q", keys)
	}
	// A question armed takes the next key whatever it is, and wears the
	// waiting color, which is the one thing waiting on you is said in.
	// It comes ahead of the view's word: while it stands, the view under
	// it cannot be worked, and its word would be a lie.
	m.view = viewProcesses
	// The question itself is on the key bar, where its answers are, and
	// the word is the band's alone.
	m.kill = &pendingKill{prompt: "END CLAUDE 11 · #1"}
	if ask := m.keys(); ask != statusBlock("CONFIRM", g) || !strings.Contains(ask, "bg="+g.Accent) {
		t.Errorf("a question armed lights %q", ask)
	}
	if bar := m.bar(); strings.Contains(bar, "CONFIRM") || !strings.Contains(bar, " END CLAUDE 11 · ##1") ||
		!strings.Contains(bar, "bg="+g.Surface+" fg="+g.Parchment) || !offered(bar, "y", "Yes") {
		t.Errorf("a question armed puts %q on the bar", bar)
	}
	m.kill = nil

	// How the processes stand is the processes view's to say, in words,
	// and nothing of it reaches the status line.
	m.projects = []work.Project{
		{Path: "/w", Entries: []work.Entry{
			{PID: 11, Kind: work.KindShell, Status: work.StatusIdle},
			{PID: 12, Kind: work.KindContact, Status: work.StatusWorking, Depth: 1},
		}},
		{Path: "/x", Entries: []work.Entry{
			{PID: 21, Kind: work.KindContact, Status: work.StatusWaiting},
			{PID: 22, Kind: work.KindShell, Status: work.StatusStopped, Fault: true},
		}},
	}
	m.view = viewProcesses
	waiting, _ := m.saying()
	quiet := m
	quiet.projects[1].Entries[0].Status = work.StatusIdle
	still, _ := quiet.saying()
	if waiting.said.keys != still.said.keys {
		t.Errorf("a contact waiting changed the status line: %q against %q", waiting.said.keys, still.said.keys)
	}

	// The first writing goes out whatever the server holds: the option
	// outlives the conn that set it, and a reground respawns the panel
	// under a fresh one that has said nothing yet.
	first := m
	first.said = nil
	if _, cmd := first.saying(); cmd == nil {
		t.Error("a conn that has said nothing yet left the status line as it found it")
	}

	// Written when it changes, and not again for the same view.
	next, cmd := m.saying()
	if cmd == nil {
		t.Fatal("what conn had not said was not put on the status line")
	}
	if _, again := next.saying(); again != nil {
		t.Error("the same word was written to the status line twice")
	}
	// Another view has other keys on the bar, so it is written.
	moved := next
	moved.view = viewProjects
	if _, changed := moved.saying(); changed == nil {
		t.Error("the keys moving to another view did not go out on the bar")
	}

	// Outside the server there is no status line to write to.
	out := m
	out.inside = false
	if _, cmd := out.saying(); cmd != nil {
		t.Error("conn wrote to the status line outside its server")
	}
}
