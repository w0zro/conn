package theme

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"charm.land/lipgloss/v2"
	term "github.com/charmbracelet/x/term"
)

// conn comes up on one of its two grounds. Dark is every terminal it
// ever knew; light is for the terminal that says its own ground is
// light when conn asks. The choice is made once, the first time conn
// brings up a server that is not there yet, written beside the socket,
// and holds for that server's life: the server down and conn again is
// how it is asked again. A conn with no terminal to ask — the panel in
// a pane of the server — reads what the server chose.

// ModePath is where the ground a server came up on is kept, beside its
// socket.
func ModePath(socket string) string {
	return filepath.Join(filepath.Dir(socket), "mode")
}

// A Mode is what a server came up on: a ground, the theme it wears,
// and whether the ground is the system's, followed as it changes.
type Mode struct {
	Dark   bool
	Theme  string
	System bool
}

// ReadMode is the mode written at ModePath, and whether one was: a
// server that has not chosen yet has nothing there. The file is one
// line, the ground and then the theme: "light conn", and "system"
// after them where the ground is the system's. A file of the ground
// alone, from a build that named no theme, wears conn's.
func ReadMode(socket string) (Mode, bool) {
	b, err := os.ReadFile(ModePath(socket))
	if err != nil {
		return Mode{}, false
	}
	words := strings.Fields(string(b))
	m := Mode{Dark: len(words) == 0 || words[0] != "light", Theme: Conn.Name}
	if len(words) > 1 {
		m.Theme = words[1]
	}
	m.System = len(words) > 2 && words[2] == "system"
	return m, true
}

// WriteMode records the mode a fresh server comes up on, so a later
// conn reads the same one back instead of asking the terminal again.
func WriteMode(socket string, m Mode) error {
	if err := os.MkdirAll(filepath.Dir(ModePath(socket)), 0o700); err != nil {
		return err
	}
	ground := "dark"
	if !m.Dark {
		ground = "light"
	}
	line := ground + " " + m.Theme
	if m.System {
		line += " system"
	}
	return os.WriteFile(ModePath(socket), []byte(line+"\n"), 0o600)
}

// SystemDark asks the system its appearance, and says whether it
// answered: dark or light where it did, and nothing known on a system
// conn cannot ask. macOS keeps the style in its defaults, Dark when it
// is and nothing at all when it is light; GNOME keeps its scheme in
// gsettings. Each is asked the way its own tools ask.
func SystemDark() (dark, known bool) {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("defaults", "read", "-g", "AppleInterfaceStyle").Output()
		if err == nil {
			return strings.TrimSpace(string(out)) == "Dark", true
		}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return false, true // not set: light
		}
		return false, false
	case "linux":
		out, err := exec.Command("gsettings", "get", "org.gnome.desktop.interface", "color-scheme").Output()
		if err != nil {
			return false, false
		}
		return strings.Contains(string(out), "prefer-dark"), true
	}
	return false, false
}

// ForgetMode takes the choice back, for a server that has gone down.
func ForgetMode(socket string) {
	_ = os.Remove(ModePath(socket))
}

// DetectDark asks the terminal for its own background with OSC 11, and
// reads what comes back. lipgloss puts the question, and a request for
// the terminal's attributes after it, which every terminal answers: one
// that does not know OSC 11 has said so as soon as that answer is in,
// and the wait is only for one that answers nothing at all. A terminal
// that says nothing, or something conn cannot read, is dark — what
// every terminal was before conn asked, and the safe read of a query
// that went nowhere.
func DetectDark() bool {
	if !term.IsTerminal(os.Stdout.Fd()) || !term.IsTerminal(os.Stdin.Fd()) {
		return true
	}
	fd := os.Stdin.Fd()
	state, err := term.MakeRaw(fd)
	if err != nil {
		return true
	}
	defer func() { _ = term.Restore(fd, state) }()
	bg, err := lipgloss.BackgroundColor(os.Stdin, os.Stdout)
	if err != nil || bg == nil {
		return true
	}
	return IsDark(bg)
}

// A Want is what conn's flags ask for: a ground, where Dark is not
// nil, and a theme by name, where Theme is not blank.
type Want struct {
	Dark  *bool
	Theme string
}

// Any says whether a flag asked for anything.
func (w Want) Any() bool {
	return w.Dark != nil || w.Theme != ""
}

// ParseFlags reads --light, --dark and --theme NAME off the front of
// conn's own arguments, before any command name: which ground and
// which theme to come up on, or to put a server already up on, instead
// of asking the terminal or reading what the server chose. It stops at
// the first argument that is none of these and answers what is left
// from there, whole. --dark and --light at once is a contradiction,
// and leaves the choice where it was.
func ParseFlags(args []string) (rest []string, want Want, err error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if name, ok := strings.CutPrefix(a, "--theme="); ok {
			want.Theme = name
			continue
		}
		if a == "--theme" {
			if i+1 == len(args) {
				return nil, Want{}, fmt.Errorf("--theme names a theme")
			}
			i++
			want.Theme = args[i]
			continue
		}
		dark := a == "--dark"
		if !dark && a != "--light" {
			return args[i:], want, nil
		}
		if want.Dark != nil && *want.Dark != dark {
			return nil, Want{}, fmt.Errorf("--dark and --light are a contradiction")
		}
		want.Dark = &dark
	}
	return nil, want, nil
}
