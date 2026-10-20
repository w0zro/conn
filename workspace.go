package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/w0zro/conn/internal/shell"
	"github.com/w0zro/conn/internal/tmux"
)

// conn's window is the panel on the left and the workspace on the
// right. Every process conn holds lives in a window of its own, out of
// sight; showing one swaps its pane into the workspace, and what stood
// there back to the window it came from, still running. The workspace
// follows the panel's cursor, the keys staying on the panel; enter
// moves the keys into it, and ctrl-space brings them back. Where the
// cursor is on nothing that has a pane, a hold stands in the workspace:
// conn's own blank pane.

// panelWidth is the panel's columns; the workspace has the rest.
const panelWidth = 50

// holdMark is on a hold, which is conn's and no process of a project's.
const holdMark = "@conn_hold"

// swaps keeps changes to the workspace one at a time: each reads what
// stands there and swaps against it, and two at once each read it
// before the other's swap landed.
var swaps sync.Mutex

// A workspace is conn's window as the panel arranges it: the window,
// the panel's pane, and the hold command, this build's.
type workspace struct {
	srv           *tmux.Server
	window, panel string
	hold          string
}

func newWorkspace(srv *tmux.Server, window, panel, self string) workspace {
	return workspace{srv: srv, window: window, panel: panel, hold: holdCommand(self)}
}

// holdCommand is how a build runs a hold.
func holdCommand(self string) string {
	return "exec " + shell.Quote(self) + " hold"
}

// holdIn is the commands that start a pane again as a hold, in its
// place: what it was marked as goes with it.
func holdIn(id, hold string) []tmux.Cmd {
	cmds := []tmux.Cmd{tmux.Respawn(id, hold), tmux.SetPane(id, holdMark, "1")}
	for _, mark := range []string{sessionMark, nameMark, runMark, dirMark, exitMark, firstMark} {
		cmds = append(cmds, tmux.Cmd{"set-option", "-pu", "-t", id, mark})
	}
	return cmds
}

// closePane takes a pane down and says how. A pane standing in the
// workspace is started again as a hold in its place, so conn's window
// keeps its shape; any other is killed.
func closePane(srv *tmux.Server, hold, id string) (string, error) {
	swaps.Lock()
	defer swaps.Unlock()
	if standing(srv, id) {
		_, err := srv.Do(holdIn(id, hold)...)
		return "respawn-pane " + id, err
	}
	return "kill-pane " + id, srv.ClosePane(id)
}

// standing says whether a pane is the one in the workspace: in the
// panel's window, and not the panel.
func standing(srv *tmux.Server, id string) bool {
	ps, err := panes(srv)
	if err != nil {
		return false
	}
	window := ""
	for _, p := range ps {
		if p.Panel {
			window = p.Window
		}
	}
	for _, p := range ps {
		if p.ID == id {
			return !p.Panel && window != "" && p.Window == window
		}
	}
	return false
}

// shown is what stands in the workspace: the pane beside the panel, and
// whether there is one.
func (w workspace) shown() (pane, bool, error) {
	out, err := w.srv.Run("list-panes", "-t", w.window, "-F", paneFormat)
	if err != nil {
		return pane{}, false, err
	}
	for _, p := range parsePanes(out) {
		if p.ID != w.panel {
			return p, true, nil
		}
	}
	return pane{}, false, nil
}

// ensure makes the workspace where there is none — conn's window just
// made, or the pane in the workspace closed — and puts a hold in it
// where what stood there has ended. A process that ends in the
// workspace stays, dead, by the window's remain-on-exit, so the layout
// holds until the hold takes its place.
func (w workspace) ensure() (pane, error) {
	p, ok, err := w.shown()
	if err != nil {
		return pane{}, err
	}
	if !ok {
		home, _ := os.UserHomeDir()
		out, err := w.srv.Run("split-window", "-h", "-d", "-t", w.panel, "-c", home, "-P", "-F", "#{pane_id}", w.hold)
		if err != nil {
			return pane{}, err
		}
		id := strings.TrimSpace(out)
		if _, err := w.srv.Do(tmux.SetPane(id, holdMark, "1"), tmux.SetWindow(w.window, "remain-on-exit", "on"), tmux.ResizeWidth(w.panel, panelWidth)); err != nil {
			return pane{}, err
		}
		return pane{ID: id, Hold: true, Window: w.window}, nil
	}
	if p.Dead {
		if _, err := w.srv.Do(holdIn(p.ID, w.hold)...); err != nil {
			return pane{}, err
		}
		p.Dead, p.Hold = false, true
	}
	return p, nil
}

// show puts a pane in the workspace, or a hold for none, and the keys
// in it where focus is asked for. What stood there goes back to the
// window the pane came from, which takes the workspace's size so it
// keeps its shape; a hold that leaves is done with.
func (w workspace) show(target string, focus bool) error {
	swaps.Lock()
	defer swaps.Unlock()
	bay, err := w.ensure()
	if err != nil {
		return err
	}
	var cmds []tmux.Cmd
	switch {
	case target == "" && bay.Hold:
	case target == "":
		out, err := w.srv.Run("new-window", "-d", "-t", sessionName+":", "-P", "-F", "#{pane_id}", w.hold)
		if err != nil {
			return err
		}
		hold := strings.TrimSpace(out)
		cmds = append(cmds, tmux.SetPane(hold, holdMark, "1"), tmux.Swap(hold, bay.ID), w.keepShape(bay.ID), windowFor(bay))
	case target != bay.ID:
		cmds = append(cmds, tmux.Swap(target, bay.ID), w.keepShape(bay.ID))
		if bay.Hold {
			cmds = append(cmds, tmux.Kill(bay.ID))
		} else {
			cmds = append(cmds, windowFor(bay))
		}
	}
	if focus && target != "" {
		cmds = append(cmds, tmux.Cmd{"select-window", "-t", w.window}, tmux.SelectPane(target))
	}
	if len(cmds) == 0 {
		return nil
	}
	_, err = w.srv.Do(cmds...)
	return err
}

// windowFor names the window a pane has gone back out to for the pane: a
// swap moves panes between windows and leaves the names where they
// were, so without this a window is named for whatever pane it had
// first. The name is the row's: the name the pane was started under,
// else the command it was given, else the directory it stands in,
// which is how the window was named when it was opened.
func windowFor(p pane) tmux.Cmd {
	return tmux.Cmd{"rename-window", "-t", p.ID, windowName(or(p.Name, or(p.Run, filepath.Base(p.Dir))))}
}

// keepShape sizes the window a pane has been swapped back out to as the
// workspace is, so what runs in it is not reflowed to another width.
func (w workspace) keepShape(id string) tmux.Cmd {
	width, height := w.size()
	return tmux.ResizeWindow(id, width, height)
}

// size is the workspace's size: the window less the panel and the
// border between them.
func (w workspace) size() (int, int) {
	out, err := w.srv.Display(w.window, "#{window_width} #{window_height}")
	if err != nil {
		return 80, 24
	}
	var width, height int
	if _, err := fmt.Sscan(out, &width, &height); err != nil {
		return 80, 24
	}
	return max(width-panelWidth-1, 10), height
}

// runHold is a hold: blank, and a key in it puts the keys back on the
// panel. conn runs it as `conn hold` in the workspace.
func runHold() error {
	if home, err := os.UserHomeDir(); err == nil {
		if srv, err := findServer(home); err == nil {
			wear(ground(home, srv.Socket))
		}
	}
	_, err := tea.NewProgram(holdModel{}, tea.WithColorProfile(colorprofile.TrueColor)).Run()
	return err
}

type holdModel struct{ width, height int }

func (h holdModel) Init() tea.Cmd { return nil }

func (h holdModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.width, h.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		return h, func() tea.Msg {
			home, _ := os.UserHomeDir()
			if srv, err := findServer(home); err == nil {
				if p, err := findPanel(srv, ""); err == nil && p.pane != "" {
					_, _ = srv.Do(tmux.Cmd{"select-window", "-t", p.window}, tmux.SelectPane(p.pane))
				}
			}
			return nil
		}
	}
	return h, nil
}

func (h holdModel) View() tea.View {
	lines := make([]string, max(h.height, 1))
	if h.height > 2 {
		lines[h.height/2] = strings.Repeat(" ", max((h.width-len(holdWord))/2, 0)) + faint.Render(holdWord)
	}
	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	return v
}

// holdWord is what a hold says: what the workspace is for.
const holdWord = "the row under the cursor shows here"
