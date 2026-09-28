package theme

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strings"

	"github.com/w0zro/conn/internal/config"

	"charm.land/lipgloss/v2"
	term "github.com/charmbracelet/x/term"
)

// conn comes up in a theme, on one of its two grounds. Dark is every
// terminal it ever knew; light is for the terminal that says its own
// ground is light when conn asks. The choice is made once, the first
// time conn brings up a tmux server that is not there yet, and holds
// for that server's life: conn down and a relaunch is how it is asked
// again. A run with no server behind it - no tmux on the machine, or a
// pane of conn's own server, where the server already chose - asks
// fresh or reads what the server chose, in place of guessing. What
// each ground is made of is the theme's, in themes.go.

// A Mode is what a server came up in: a theme, by name, on one of its
// grounds.
type Mode struct {
	Theme string
	Dark  bool
}

// Wear is the ground a mode puts conn on: the theme's, dark or light,
// which everything conn draws is then handed. A theme conn does not
// have is conn's own, which is what a mode file from a build that had
// the theme, read by one that does not, comes to.
func (m Mode) Wear() Ground {
	t, ok := Named(m.Theme)
	if !ok {
		t = Conn
	}
	return t.on(m.Dark)
}

// ModePath is where the mode a server came up in is kept, beside its
// socket and its tmux.conf.
func ModePath(socket string) string {
	return filepath.Join(filepath.Dir(socket), "mode")
}

// ReadModeFile is the mode written at ModePath, and whether one was:
// a server that has not picked yet has nothing there. The file is one
// line, the ground and then the theme: "light datum". A file from
// before conn had themes says the ground alone, and is read as conn's.
func ReadModeFile(socket string) (Mode, bool) {
	b, err := os.ReadFile(ModePath(socket))
	if err != nil {
		return Mode{}, false
	}
	words := strings.Fields(string(b))
	m := Mode{Theme: Default, Dark: len(words) == 0 || words[0] != "light"}
	if len(words) > 1 {
		if _, ok := Named(words[1]); ok {
			m.Theme = words[1]
		}
	}
	return m, true
}

// WriteMode records the mode a fresh server comes up in, so a later
// conn - attaching, or asking for a theme - reads the same one back
// instead of asking the terminal again.
func WriteMode(socket string, m Mode) error {
	if err := os.MkdirAll(filepath.Dir(ModePath(socket)), 0o700); err != nil {
		return err
	}
	ground := "dark"
	if !m.Dark {
		ground = "light"
	}
	return os.WriteFile(ModePath(socket), []byte(ground+" "+m.Theme+"\n"), 0o600)
}

// ServerMode is the mode the server on this socket came up in, or would
// if none is up yet: what the configuration says, until one has picked
// for itself.
func ServerMode(socket, home string) Mode {
	if m, ok := ReadModeFile(socket); ok {
		return m
	}
	return configMode(home)
}

// configMode is the mode the file asks for: its theme, and its ground
// where it names one. A file naming no ground is dark here — this is
// the mode for a conn with no server to read and nobody to ask, and
// dark is what every terminal was before conn learned to ask. AskMode
// is where the terminal is asked.
func configMode(home string) Mode {
	m := Mode{Theme: configTheme(home), Dark: true}
	if dark, ok := config.GroundNamed(configGround(home)); ok {
		m.Dark = dark
	}
	return m
}

// configGround is the ground the file names, as written.
func configGround(home string) string {
	c, _ := config.Read(home)
	return c.Ground
}

// An Override is what the flags said ahead of the command: a ground,
// when --light or --dark was given, and a theme, when --theme was.
type Override struct {
	Dark  *bool
	Theme string
}

// Over is the mode with what the flags said laid over it; what they
// did not say stands.
func (o Override) Over(m Mode) Mode {
	if o.Dark != nil {
		m.Dark = *o.Dark
	}
	if o.Theme != "" {
		m.Theme = o.Theme
	}
	return m
}

// AskMode is the mode a fresh server comes up in: what the flags said,
// and for what they did not, what the configuration says — the theme,
// and the ground where it names one, the terminal's own asked fresh
// where it does not. A file naming a ground is an operator who wants
// the same one wherever they are; the question put to the terminal is
// for everybody else, which is most stations.
func AskMode(o Override, home string) Mode {
	m := configMode(home)
	if o.Dark == nil {
		if _, named := config.GroundNamed(configGround(home)); !named {
			m.Dark = detectDark()
		}
	}
	return o.Over(m)
}

// ParseFlags reads --light, --dark and --theme NAME off the front
// of conn's own arguments, before any command name: which ground and
// which theme to come up in, instead of asking the terminal, the
// configuration or a server's mode file. It stops at the first
// argument that is not one of these, dark or light or a command's own,
// and answers what is left of args from there, whole. --dark with
// --light, or --theme twice with two names, is a contradiction; a
// theme conn does not have is an error that names the ones it does.
// Neither leaves the choice where it always was.
func ParseFlags(args []string) (rest []string, o Override, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dark":
			if o.Dark != nil && !*o.Dark {
				return nil, Override{}, fmt.Errorf("--dark and --light are a contradiction")
			}
			dark := true
			o.Dark = &dark
		case "--light":
			if o.Dark != nil && *o.Dark {
				return nil, Override{}, fmt.Errorf("--dark and --light are a contradiction")
			}
			light := false
			o.Dark = &light
		case "--theme":
			if i+1 == len(args) {
				return nil, Override{}, fmt.Errorf("--theme wants a theme's name: %s", themeNames())
			}
			name := args[i+1]
			if _, ok := Named(name); !ok {
				return nil, Override{}, fmt.Errorf("conn has no theme %s; it has %s", name, themeNames())
			}
			if o.Theme != "" && o.Theme != name {
				return nil, Override{}, fmt.Errorf("--theme %s and --theme %s are a contradiction", o.Theme, name)
			}
			o.Theme = name
			i++
		default:
			return args[i:], o, nil
		}
	}
	return nil, o, nil
}

// themeNames is the themes conn has, said in a sentence.
func themeNames() string {
	var names []string
	for _, t := range All {
		names = append(names, t.Name)
	}
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// detectDark asks the terminal for its own background with OSC 11, and
// reads what comes back. lipgloss puts the question, and a request for
// the terminal's attributes after it, which every terminal answers: one
// that does not know OSC 11 has said so as soon as that answer is in,
// and the wait is only for one that answers nothing at all. A terminal
// that says nothing, or something conn cannot read, is dark - which is
// what every terminal was before conn asked, and the safe read of a
// query that went nowhere.
func detectDark() bool {
	if !term.IsTerminal(os.Stdout.Fd()) || !term.IsTerminal(os.Stdin.Fd()) {
		return true
	}
	fd := os.Stdin.Fd()
	state, err := term.MakeRaw(fd)
	if err != nil {
		return true
	}
	// Nothing to be done if the terminal will not go back.
	defer func() { _ = term.Restore(fd, state) }()
	bg, err := lipgloss.BackgroundColor(os.Stdin, os.Stdout)
	if err != nil || bg == nil {
		return true
	}
	return IsDark(bg)
}

// IsDark reads a ground as dark or light by the same relative luminance
// a screen reader uses to say if text passes on it: below half is dark.
func IsDark(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	luminance := 0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)
	return luminance/0xffff < 0.5
}
