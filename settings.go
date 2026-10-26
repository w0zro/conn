package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/w0zro/conn/internal/theme"
	"github.com/w0zro/conn/internal/tmux"
)

// The settings: , on either screen puts them over it. A row a theme
// there is, then a row a ground, dark, light and the system's; enter
// puts the server on the row's, where it stands, and keeps it in
// config.json for the next server. On the system's, the panel asks the
// system its appearance as it runs, and puts the server on the other
// ground when the system has gone over to it.

// A setting is a row of the settings: a theme by name, or a ground.
type setting struct {
	kind string // "theme" or "mode"
	name string
}

// settingRows are the settings' rows: every theme, then the grounds.
func (m model) settingRows() []setting {
	var out []setting
	for _, n := range theme.Names(themesDir(m.home)) {
		out = append(out, setting{"theme", n})
	}
	return append(out, setting{"mode", "dark"}, setting{"mode", "light"}, setting{"mode", "system"})
}

// mode is what the server is on, by its mode file.
func (m model) mode() theme.Mode {
	if m.srv == nil {
		return theme.Mode{Dark: m.g.Dark(), Theme: theme.Conn.Name}
	}
	md, ok := theme.ReadMode(m.srv.Socket)
	if !ok {
		return theme.Mode{Dark: true, Theme: theme.Conn.Name}
	}
	return md
}

// on says whether a setting is what the server is on.
func (s setting) on(md theme.Mode) bool {
	switch {
	case s.kind == "theme":
		return s.name == md.Theme
	case s.name == "system":
		return md.System
	}
	return !md.System && (s.name == "dark") == md.Dark
}

// now says whether a setting is the ground the system has the server
// on: what following it came to.
func (s setting) now(md theme.Mode) bool {
	return s.kind == "mode" && md.System && s.name != "system" && (s.name == "dark") == md.Dark
}

// settingsKey is a key while the settings stand: the arrows move, and
// j and k, around at the ends, g and G to the top and the bottom,
// enter puts the server on the row's, and esc or , again puts them
// away.
func (m model) settingsKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	rows := m.settingRows()
	switch k.String() {
	case "esc", ",", "q", "ctrl+c":
		m.settings = false
	case "up", "k", "ctrl+p":
		m.setAt = ring(m.setAt, -1, len(rows))
	case "down", "j", "ctrl+n":
		m.setAt = ring(m.setAt, 1, len(rows))
	case "g":
		m.setAt = 0
	case "G":
		m.setAt = max(len(rows)-1, 0)
	case "enter", "space":
		if m.setAt >= len(rows) || m.srv == nil {
			return m, nil
		}
		md, s := m.mode(), rows[m.setAt]
		switch {
		case s.kind == "theme":
			md.Theme = s.name
		case s.name == "system":
			md.System = true
			if dark, known := systemDark(); known {
				md.Dark = dark
			}
		default:
			md.Dark, md.System = s.name == "dark", false
		}
		srv, home := m.srv, m.home
		return m, func() tea.Msg {
			g, err := retheme(srv, home, md)
			return rethemed{g, err}
		}
	}
	return m, nil
}

// rethemed is the server put on a mode: the ground the panel is to
// wear, or why it could not be.
type rethemed struct {
	g   theme.Ground
	err error
}

// takeTheme is the panel wearing what the server was just put on: its
// styles, its own ground, and the key bar written again in it.
func (m model) takeTheme(r rethemed) model {
	if r.err != nil {
		m.note = r.err.Error()
		return m
	}
	wear(r.g)
	m.g, m.bar, m.banded = r.g, "", ""
	if m.own != "" {
		_ = m.srv.Paint(m.own, r.g.Surface)
	}
	return m
}

// drawSettings is the settings: the themes under a label, the grounds
// under another, and on beside what the server is on.
func (m model) drawSettings() ([]string, foot) {
	lines := head("conn", "settings")
	// What a choice does, said once: the server is dressed where it
	// stands, and the next one comes up the same.
	lines = append(lines, m.row([]piece{{"enter changes it now, in every pane", gray}}, piece{}, false), m.row([]piece{{"and keeps it in " + tilde(filepath.Join(configDir(m.home), "config.json"), m.home), gray}}, piece{}, false), "")
	md := m.mode()
	last := ""
	for i, s := range m.settingRows() {
		if s.kind != last {
			if last != "" {
				lines = append(lines, "")
			}
			lines = append(lines, m.label(s.kind))
			last = s.kind
		}
		name, mark := gray, piece{}
		switch {
		case s.on(md):
			name, mark = ink, piece{"on", accent}
		case s.now(md):
			mark = piece{"now", gray}
		}
		lines = append(lines, m.row([]piece{{s.name, name}}, mark, i == m.setAt))
	}
	return lines, foot{hints: settingsHints}
}

// askSystemEvery is how often the panel asks the system its
// appearance while the server follows it.
const askSystemEvery = 5 * time.Second

// followSystem puts the server on the ground the system has gone over
// to, where the server follows the system and the system says. The
// server's mode is read where it is kept, so a conn that put the
// server on a ground from outside is seen here too.
func (m model) followSystem() tea.Cmd {
	if m.srv == nil {
		return nil
	}
	srv, home := m.srv, m.home
	return func() tea.Msg {
		md, ok := theme.ReadMode(srv.Socket)
		if !ok || !md.System {
			return nil
		}
		dark, known := systemDark()
		if !known || dark == md.Dark {
			return nil
		}
		md.Dark = dark
		g, err := retheme(srv, home, md)
		return rethemed{g, err}
	}
}

// retheme puts the server on a mode where it stands: written down
// beside the socket, the configuration written and sourced again, the
// holds started again to draw in it, Claude Code's theme rewritten, and
// the mode kept in config.json so the next server comes up on it.
func retheme(srv *tmux.Server, home string, md theme.Mode) (theme.Ground, error) {
	th, err := theme.Load(themesDir(home), md.Theme)
	if err != nil {
		return theme.Ground{}, err
	}
	g := th.On(md.Dark)
	if err := theme.WriteMode(srv.Socket, md); err != nil {
		return theme.Ground{}, err
	}
	if err := dressServer(srv, home, g); err != nil {
		return theme.Ground{}, err
	}
	if self, err := os.Executable(); err == nil {
		out, _ := srv.Run("list-panes", "-a", "-F", "#{pane_id} #{"+holdMark+"}")
		var cmds []tmux.Cmd
		for _, l := range strings.Split(out, "\n") {
			if id, hold, _ := strings.Cut(l, " "); hold == "1" {
				cmds = append(cmds, holdIn(id, holdCommand(self))...)
			}
		}
		if len(cmds) > 0 {
			_, _ = srv.Do(cmds...)
		}
	}
	return g, keepMode(home, md)
}

// keepMode writes the mode into config.json as its theme and ground,
// and leaves everything else in the file as it was.
func keepMode(home string, md theme.Mode) error {
	path := filepath.Join(configDir(home), "config.json")
	c := map[string]json.RawMessage{}
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return err
	default:
		if err := json.Unmarshal(b, &c); err != nil {
			return err
		}
	}
	ground := "light"
	switch {
	case md.System:
		ground = "system"
	case md.Dark:
		ground = "dark"
	}
	c["theme"], _ = json.Marshal(md.Theme)
	c["ground"], _ = json.Marshal(ground)
	out, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o644)
}
