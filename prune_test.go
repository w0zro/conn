package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/w0zro/conn/internal/theme"
)

// st and ss show what a project has written down before they run it:
// each check or stack part on a line of its own with its command, a
// count at the right of the question, the cursor on the first. x takes
// the line off for good — from the list, from what the keys run and
// from what new agents are told — and says which in gray; enter runs
// what is left, esc runs nothing, and a list with every line gone says
// so. A project with nothing written down still asks Claude Code.
func TestStAndSsListWhatIsWrittenDownAndPruneIt(t *testing.T) {
	g := theme.Dark
	wear(g)
	t.Cleanup(undress)
	shots := os.Getenv("SHOTS")
	grayInk := strings.SplitN(gray.Render("x"), "x", 2)[0]
	plainInk := strings.SplitN(ink.Render("x"), "x", 2)[0]
	rowOf := func(name, command string) *regexp.Regexp {
		return regexp.MustCompile(`(^|\s)` + regexp.QuoteMeta(name) + `\s+` + regexp.QuoteMeta(command) + `\s*$`)
	}

	for _, c := range []struct {
		as            role
		key, ask      string
		names         []string
		commands      []string
		fromElsewhere string
		told          func(name string) string
	}{
		{roleCheck, "t", "Run these checks in web?", []string{"unit", "types", "bundle"}, []string{"echo one", "echo two", "echo three"}, "echo four", func(n string) string { return "conn u " + n + " -w -- " }},
		{roleStack, "s", "Start these in web?", []string{"api", "worker", "queue"}, []string{"sleep 1001", "sleep 1002", "sleep 1003"}, "sleep 1004", func(n string) string { return "conn u " + n + " -- " }},
	} {
		keys := "s" + c.key
		t.Run(keys, func(t *testing.T) {
			srv, _, _ := scratchServer(t)
			dir := filepath.Dir(srv.Socket)
			elsewhere := filepath.Join(dir, "elsewhere")
			path := startedPath(srv.Socket)
			for i, n := range c.names {
				if err := recordStart(path, started{Root: dir, Name: n, Command: c.commands[i], Role: c.as}); err != nil {
					t.Fatal(err)
				}
			}
			// Another project's part under the very name that is taken off
			// here is another project's.
			if err := recordStart(path, started{Root: elsewhere, Name: c.names[1], Command: c.fromElsewhere, Role: c.as}); err != nil {
				t.Fatal(err)
			}
			doomed := c.names[1]

			// listed is the screen's lines as plain text, and the raw ones.
			listed := func(m model) ([]string, []string) {
				raw := strings.Split(m.View().Content, "\n")
				plain := make([]string, len(raw))
				for i, l := range raw {
					plain[i] = strings.TrimRight(plainText(l), " ")
				}
				return plain, raw
			}
			open := func() model {
				t.Helper()
				m := model{srv: srv, conn: "conn", screen: onProject, cur: project{Name: "web", Root: dir}, rows: []row{}, width: 120, height: 36}
				m, _ = press(t, m, "s")
				m, _ = press(t, m, c.key)
				if plain, _ := listed(m); !strings.Contains(strings.Join(plain, "\n"), c.ask) {
					t.Fatalf("%s ran what was written down without showing it first:\n%s", keys, strings.Join(plain, "\n"))
				}
				return m
			}
			ps := func() []string {
				t.Helper()
				all, err := panes(srv)
				if err != nil {
					t.Fatal(err)
				}
				var out []string
				for _, p := range all {
					out = append(out, p.Name)
				}
				return out
			}
			frame := func(name string, m model) {
				if shots == "" || c.as != roleCheck {
					return
				}
				var b strings.Builder
				fmt.Fprintf(&b, "\x1b]10;%s\x1b\\\x1b]11;%s\x1b\\\x1b[2J\x1b[H", theme.Hex(g.Ink), g.Surface)
				for _, l := range strings.Split(m.View().Content, "\n") {
					b.WriteString(l + "\x1b[0m" + strings.Repeat(" ", max(120-ansi.StringWidth(l), 0)) + "\r\n")
				}
				if err := os.WriteFile(filepath.Join(shots, name), []byte(strings.TrimSuffix(b.String(), "\r\n")+"\x1b[0m"), 0o644); err != nil {
					t.Error(err)
				}
			}
			// rows is the lines that say a name and its command, in order.
			rows := func(plain []string, names, commands []string) (at []int) {
				for i := range names {
					for n, l := range plain {
						if rowOf(names[i], commands[i]).MatchString(l) {
							at = append(at, n)
							break
						}
					}
				}
				return at
			}
			remaining := func(skip string) (ns, cs []string) {
				for i, n := range c.names {
					if n != skip {
						ns, cs = append(ns, n), append(cs, c.commands[i])
					}
				}
				return
			}

			// Pressed, the keys list what is written down and start nothing.
			m := open()
			plain, _ := listed(m)
			screen := strings.Join(plain, "\n")
			ask := -1
			for i, l := range plain {
				if strings.Contains(l, c.ask) {
					ask = i
				}
			}
			if ask < 0 {
				t.Fatalf("%s does not ask %q:\n%s", keys, c.ask, screen)
			}
			counted := func(line string, n int) bool {
				return regexp.MustCompile(regexp.QuoteMeta(c.ask) + fmt.Sprintf(`\s{3,}%d(\s\S+)?$`, n)).MatchString(line)
			}
			if !counted(plain[ask], 3) {
				t.Errorf("the question %q has no count of 3 at its right", plain[ask])
			}
			at := rows(plain, c.names, c.commands)
			if len(at) != 3 || at[0] != ask+2 && at[0] != ask+1 || at[1] != at[0]+1 || at[2] != at[1]+1 {
				t.Fatalf("%s does not list the three, name and command each on its own line, in order, under the question:\n%s", keys, screen)
			}
			if strings.Contains(screen, c.fromElsewhere) {
				t.Errorf("%s lists another project's:\n%s", keys, screen)
			}
			if !strings.HasPrefix(plain[at[0]], cursorBar) || strings.HasPrefix(plain[at[1]], cursorBar) {
				t.Errorf("the cursor is not on the first line only:\n%s", screen)
			}
			if !strings.Contains(screen, "enter") || !strings.Contains(screen, "esc") {
				t.Errorf("the bar names no enter and esc:\n%s", screen)
			}
			if got := ps(); contains(got, c.names...) {
				t.Errorf("%s started %v before enter", keys, got)
			}
			frame("list-120x36.ans", m)

			// j and k move the cursor.
			m, _ = press(t, m, "j")
			if plain, _ = listed(m); !strings.HasPrefix(plain[at[1]], cursorBar) || strings.HasPrefix(plain[at[0]], cursorBar) {
				t.Errorf("j did not move the cursor to the second line:\n%s", strings.Join(plain, "\n"))
			}
			m, _ = press(t, m, "k")
			if plain, _ = listed(m); !strings.HasPrefix(plain[at[0]], cursorBar) {
				t.Errorf("k did not move the cursor back to the first line:\n%s", strings.Join(plain, "\n"))
			}

			// x on the second line takes it off, and says so on the foot,
			// in the ink, as a note of conn's own.
			m, _ = press(t, m, "j")
			m, _ = press(t, m, "x")
			plain, raw := listed(m)
			screen = strings.Join(plain, "\n")
			ns, cs := remaining(doomed)
			if now := rows(plain, ns, cs); len(now) != 2 || now[1] != now[0]+1 {
				t.Fatalf("after x the two that are left are not listed:\n%s", screen)
			}
			note := -1
			for i, l := range plain {
				if strings.Contains(l, doomed) {
					if rowOf(doomed, c.commands[1]).MatchString(l) {
						t.Fatalf("x left %s on the list:\n%s", doomed, screen)
					}
					note = i
				}
			}
			if note < 0 {
				t.Errorf("nothing names %s as taken off:\n%s", doomed, screen)
			} else if !strings.Contains(raw[note], plainInk) || strings.Contains(raw[note], grayInk) {
				t.Errorf("the note %q is not in the ink (%q): %q", plain[note], plainInk, raw[note])
			}
			for _, l := range plain {
				if strings.Contains(l, c.ask) && !counted(l, 2) {
					t.Errorf("the question %q does not count the two that are left", l)
				}
			}
			frame("pruned-120x36.ans", m)

			// It is gone for good: from what the keys run, and from what
			// new agents are told; another project's of that name is not.
			here := func(root string) bool { return root == dir }
			note0 := told("conn", srv.Socket, here)
			if strings.Contains(note0, c.told(doomed)+c.commands[1]) {
				t.Errorf("new agents are still told %s:\n%s", doomed, note0)
			}
			for _, i := range []int{0, 2} {
				if want := c.told(c.names[i]) + c.commands[i]; !strings.Contains(note0, want) {
					t.Errorf("new agents are no longer told %q:\n%s", want, note0)
				}
			}
			if got := told("conn", srv.Socket, func(root string) bool { return root == elsewhere }); !strings.Contains(got, c.told(doomed)+c.fromElsewhere) {
				t.Errorf("another project's %s went with it:\n%s", doomed, got)
			}

			// esc runs nothing, and the part is still off the list when
			// the keys are pressed again.
			m, _ = press(t, m, "esc")
			if plain, _ = listed(m); strings.Contains(strings.Join(plain, "\n"), c.ask) {
				t.Error("esc left the list standing")
			}
			if contains(ps(), c.names...) {
				t.Errorf("esc started %v", ps())
			}
			m = open()
			plain, _ = listed(m)
			if now := rows(plain, ns, cs); len(now) != 2 {
				t.Fatalf("%s again does not list the two that are left:\n%s", keys, strings.Join(plain, "\n"))
			}
			if strings.Contains(strings.Join(plain, "\n"), c.commands[1]) {
				t.Errorf("%s again lists %s:\n%s", keys, doomed, strings.Join(plain, "\n"))
			}

			// enter runs what is left, and only that.
			_, cmd := press(t, m, "enter")
			if cmd == nil {
				t.Fatal("enter ran nothing")
			}
			said := ""
			var find func(tea.Cmd)
			find = func(c tea.Cmd) {
				switch msg := c().(type) {
				case tea.BatchMsg:
					for _, c := range msg {
						if c != nil {
							find(c)
						}
					}
				case startedRoles:
					said = msg.said
				}
			}
			find(cmd)
			if !strings.Contains(said, c.names[0]) || !strings.Contains(said, c.names[2]) || strings.Contains(said, doomed) {
				t.Errorf("enter said %q; want %s and %s run and %s not", said, c.names[0], c.names[2], doomed)
			}
			got := ps()
			if !contains(got, c.names[0], c.names[2]) || contains(got, doomed) {
				t.Errorf("after enter the panes are %v; want %s and %s and not %s", got, c.names[0], c.names[2], doomed)
			}

			// Every line taken off leaves a list that says so, and enter
			// there runs nothing.
			m = open()
			m, _ = press(t, m, "x")
			m, _ = press(t, m, "x")
			plain, _ = listed(m)
			screen = strings.Join(plain, "\n")
			if c.as == roleCheck {
				if want := "No checks written down in web. st asks Claude what they are."; !strings.Contains(screen, want) {
					t.Errorf("with every line off the list does not say %q:\n%s", want, screen)
				}
			} else if !strings.Contains(screen, "written down in web") || !strings.Contains(screen, "ss asks Claude") {
				t.Errorf("with every line off the list does not say that nothing is written down in web and that ss asks Claude:\n%s", screen)
			}
			for _, l := range plain {
				if _, after, ok := strings.Cut(l, c.ask); ok && regexp.MustCompile(`\d`).MatchString(after) {
					t.Errorf("a count stands over an empty list: %q", l)
				}
			}
			frame("empty-120x36.ans", m)
			for i, n := range c.names {
				if got := told("conn", srv.Socket, here); strings.Contains(got, c.told(n)+c.commands[i]) {
					t.Errorf("after every x new agents are still told %s:\n%s", n, got)
				}
			}
			// With nothing written down, the keys ask Claude Code as before.
			m, _ = press(t, m, "esc")
			m, _ = press(t, m, "s")
			m, cmd = press(t, m, c.key)
			if cmd == nil {
				t.Errorf("%s with nothing written down did not ask Claude Code", keys)
			}
			if plain, _ = listed(m); !strings.Contains(strings.Join(plain, "\n"), "asking Claude") {
				t.Errorf("%s with nothing written down does not say it is asking:\n%s", keys, strings.Join(plain, "\n"))
			}
		})
	}
}

// A project with nothing written down at all asks Claude Code, as
// before, and lists nothing.
func TestStAndSsWithNothingWrittenDownAskClaude(t *testing.T) {
	srv, _, _ := scratchServer(t)
	dir := filepath.Dir(srv.Socket)
	for _, c := range []struct {
		key string
	}{{"t"}, {"s"}} {
		m := model{srv: srv, screen: onProject, cur: project{Name: "web", Root: dir}, rows: []row{}, width: 120, height: 36}
		m, _ = press(t, m, "s")
		m, cmd := press(t, m, c.key)
		if cmd == nil {
			t.Errorf("s%s did not ask Claude Code", c.key)
		}
		if screen := plainText(m.View().Content); !strings.Contains(screen, "asking Claude what") {
			t.Errorf("s%s does not say it is asking:\n%s", c.key, screen)
		}
	}
}

// press is a key pressed on the panel.
func press(t *testing.T, m model, k string) (model, tea.Cmd) {
	t.Helper()
	key := tea.Key{Text: k, Code: []rune(k)[0]}
	switch k {
	case "esc":
		key = tea.Key{Code: tea.KeyEscape}
	case "enter":
		key = tea.Key{Code: tea.KeyEnter}
	}
	next, cmd := m.Update(tea.KeyPressMsg(key))
	return next.(model), cmd
}

// contains is whether every name is among the names.
func contains(names []string, want ...string) bool {
	for _, w := range want {
		found := false
		for _, n := range names {
			found = found || n == w
		}
		if !found {
			return false
		}
	}
	return len(want) > 0
}
