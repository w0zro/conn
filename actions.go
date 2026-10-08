package main

import (
	"path/filepath"
	"strings"

	"github.com/w0zro/conn/internal/room"
	"github.com/w0zro/conn/internal/work"
	"github.com/w0zro/conn/internal/work/brew"
	"github.com/w0zro/conn/internal/work/claude"
	"github.com/w0zro/conn/internal/work/docker"

	"github.com/w0zro/conn/internal/station"

	"github.com/w0zro/conn/internal/tmux"

	"github.com/w0zro/conn/internal/config"

	tea "charm.land/bubbletea/v2"
)

// What the panel asks of the server, each off the loop as a command:
// a process into the bay, a shell opened at a project, the bay opened
// beside the panel, and the small ones — zoom, width, detach — through
// serverCmd.

// openBay opens the bay beside the panel, with the hold in it.
func (m model) openBay() tea.Cmd {
	home, self := m.head.Login.Home, m.self
	return m.serverCmd(func() error { return m.srv.SplitBay(home, self) })
}

// reviveBay puts a hold in a bay whose pane died, in its own shape.
func (m model) reviveBay() tea.Cmd {
	home, self := m.head.Login.Home, m.self
	return m.serverCmd(func() error { return m.srv.ReviveBay(home, self) })
}

// reach puts a process in the bay, off the loop, and passes back the
// terminal that is in the bay once it is there.
func (m model) reach(target room.Pane, tty string) tea.Cmd {
	srv := m.srv
	return func() tea.Msg {
		if srv.Show(target) != nil {
			return nil
		}
		return reachedMsg{tty}
	}
}

// openReadout puts the readout in the bay, off the loop. It follows the
// panel's cursor from there, so this is asked once and not again for
// every row read: focus stays on the panel, and j and k carry the page
// along with them.
func (m model) openReadout() tea.Cmd {
	home, self, srv := m.head.Login.Home, m.self, m.srv
	return func() tea.Msg {
		if srv.ShowReadout(home, self) != nil {
			return nil
		}
		return readoutMsg{on: true}
	}
}

// openShell opens a shell at a project, off the loop, and passes
// back what tmux said of it.
func (m model) openShell(dir string) tea.Cmd {
	srv := m.srv
	return func() tea.Msg {
		sh, err := srv.Open(dir)
		if err != nil {
			return noticeMsg{"the shell could not be opened: " + err.Error()}
		}
		return openedMsg{shell: sh}
	}
}

// startContact opens a contact at a project, off the loop, the way
// openShell opens a shell there.
func (m model) startContact(dir string) tea.Cmd {
	srv := m.srv
	return func() tea.Msg {
		sh, err := srv.OpenCmd(dir, claude.Command(srv.Socket))
		if err != nil {
			return noticeMsg{"the contact could not be opened: " + err.Error()}
		}
		return openedMsg{shell: sh}
	}
}

// scanProjects walks the roots off the loop; what it found, or why it
// could not, comes back as a message. A config file that will not parse
// is said instead of the walk, and ahead of it: the roots being walked
// are then not the ones the operator asked for, and that is the first
// thing to know.
func (m model) scanProjects() tea.Cmd {
	roots, cfgErr := config.Roots(m.head.Login.Home)
	return func() tea.Msg {
		if cfgErr != nil {
			return projectsMsg{err: "THE CONFIG COULD NOT BE READ: " + cfgErr.Error()}
		}
		ps, err := findProjects(roots)
		if err != nil {
			return projectsMsg{err: "THE ROOTS COULD NOT BE WALKED: " + err.Error()}
		}
		return projectsMsg{projects: ps}
	}
}

// scanSessions reads a project's suspended sessions off the loop, the
// process table as it stood when the sessions view opened, so a
// leftover session file cannot be mistaken for one still going.
func (m model) scanSessions(dirs []string) tea.Cmd {
	projects := m.projects
	return func() tea.Msg {
		return sessionsMsg{dirs: dirs, sessions: claude.Suspended(dirs, projects)}
	}
}

// openResumed opens a shell that picks a suspended session back
// up, off the loop, the way startContact opens a fresh one.
func (m model) openResumed(dir, id string) tea.Cmd {
	srv := m.srv
	return func() tea.Msg {
		sh, err := srv.OpenCmd(dir, claude.ResumeCommand(srv.Socket, id))
		if err != nil {
			return nil
		}
		return openedMsg{shell: sh}
	}
}

// hasPid says whether a process is among what was read.
func hasPid(projects []work.Project, pid int) bool {
	for _, pl := range projects {
		for _, e := range pl.Entries {
			if e.PID == pid {
				return true
			}
		}
	}
	return false
}

// serverCmd runs a server action off the loop. What the server did is
// on the window for the operator to see, and what it did not do has no
// row of its own to be said on.
func (m model) serverCmd(act func() error) tea.Cmd {
	return func() tea.Msg {
		_ = act()
		return nil
	}
}

// openClient is S on a row that is a program conn knows: a session with
// the server by its own client, psql for postgres. In a container the
// client runs inside it by docker exec, as the user the image was
// given. On this machine it connects to the port the row listens on,
// and is found on the path, or under the formula's own prefix where
// brew keeps it out of the path, as it does postgresql. The pane holds
// on a failure so what went wrong can be read, as a shell in a
// container does; a session ended by the operator takes its pane with
// it, as a shell does.
func (m model) openClient(e work.Entry, p *knownProgram, dir string) tea.Cmd {
	srv := m.srv
	if dir == "" {
		dir = e.Cwd
	}
	if e.Container != "" {
		id := e.Container
		return func() tea.Msg {
			user := containerEnv(id, p.userEnv)
			if user == "" {
				user = p.user
			}
			cmd := tmux.ShellQuote(docker.Path) + " exec -it " + tmux.ShellQuote(id) + " " + p.inContainer(user) + " 2>&1 || " + tmux.HoldOpen
			sh, err := srv.OpenShellIn(dir, cmd, id)
			if err != nil {
				return nil
			}
			return openedMsg{shell: sh}
		}
	}
	if len(e.Ports) == 0 {
		return nil
	}
	port, formula := e.Ports[0], e.Brew
	return func() tea.Msg {
		client := station.LookPath(p.client)
		if client == "" && formula != "" {
			if out, err := brew.Says(brew.Wait, "--prefix", formula); err == nil {
				if c := filepath.Join(strings.TrimSpace(string(out)), "bin", p.client); station.LookPath(c) != "" {
					client = c
				}
			}
		}
		if client == "" {
			return noticeMsg{p.client + " was not found on the path"}
		}
		cmd := tmux.ShellQuote(client) + " " + p.args(port) + " 2>&1 || " + tmux.HoldOpen
		sh, err := srv.OpenCmd(dir, cmd)
		if err != nil {
			return noticeMsg{"the session could not be opened: " + err.Error()}
		}
		return openedMsg{shell: sh}
	}
}

// containerEnv is one variable of a container's environment, as docker
// inspect reports it, or nothing.
func containerEnv(id, name string) string {
	if name == "" || docker.Path == "" {
		return ""
	}
	out, err := docker.Says(docker.Wait, "inspect", "-f", "{{range .Config.Env}}{{println .}}{{end}}", id)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if v, ok := strings.CutPrefix(line, name+"="); ok {
			return v
		}
	}
	return ""
}

// pickShell is run inside the container to choose its shell. bash is
// tested for rather than tried, because exec replaces the shell and a
// failed exec ends it: exec bash || exec sh never reaches the fallback,
// and on an image with no bash — which is most of the small ones — it
// exits 127 rather than giving you the sh that was there all along.
const pickShell = "command -v bash >/dev/null 2>&1 && exec bash || exec sh"

// openHelp puts the manual in the workspace. The page is written out of
// the binary first, so what is shown is the manual this conn was built
// with rather than whatever is installed on the machine.
func (m model) openHelp() tea.Cmd {
	home, self, srv := m.head.Login.Home, m.self, m.srv
	return func() tea.Msg {
		if srv.ShowHelp(home, self) != nil {
			return nil
		}
		return detourMsg{toManual}
	}
}

// openTheSettings puts the settings in the workspace, the way the
// manual goes there.
func (m model) openTheSettings() tea.Cmd {
	home, self, srv := m.head.Login.Home, m.self, m.srv
	return func() tea.Msg {
		if srv.ShowSettings(home, self) != nil {
			return nil
		}
		return detourMsg{toSettings}
	}
}
