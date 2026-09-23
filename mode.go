package main

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strings"

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

// A mode is what a server came up in: a theme, by name, on one of its
// grounds.
type mode struct {
	theme string
	dark  bool
}

// current is the mode conn is in, as applyMode last left it. The mode
// is package-wide — scheme, the hexes, themeBase and the rest are all
// set from it at once — and nothing else names which one is in force,
// so a caller that needs to put it back has to have kept this. It is
// conn's dark until applyMode says otherwise, which is what every
// terminal was before conn learned to ask.
var current = mode{theme: defaultTheme, dark: true}

// themeBase is the base claudeThemeJSON sits on, dark-ansi or
// light-ansi, and vimBackground what the colorscheme tells nvim its own
// background is: whichever ground applyMode last chose.
var (
	themeBase     = "dark-ansi"
	vimBackground = "dark"
)

// applyMode puts every color conn draws from onto one ground of one
// theme. In conn it is called once, before anything reads scheme,
// groundColor, cursorHex, or any of the rest. A theme conn does not
// have is conn's own, which is what a mode file from a build that had
// the theme, read by one that does not, comes to. A test binary is one
// process running every test, so a test that calls it — or calls
// something that calls it, which dressProgram does on its way to
// writing a theme — leaves the mode it chose standing for whatever
// runs next; see holdMode.
func applyMode(m mode) {
	t, ok := themeNamed(m.theme)
	if !ok {
		t, m.theme = connTheme, defaultTheme
	}
	current = m
	wear(t.on(m.dark))
	themeBase, vimBackground = "dark-ansi", "dark"
	if !m.dark {
		themeBase, vimBackground = "light-ansi", "light"
	}
}

// modePath is where the mode a server came up in is kept, beside its
// socket and its tmux.conf.
func modePath(socket string) string {
	return filepath.Join(filepath.Dir(socket), "mode")
}

// readModeFile is the mode written at modePath, and whether one was:
// a server that has not picked yet has nothing there. The file is one
// line, the ground and then the theme: "light datum". A file from
// before conn had themes says the ground alone, and is read as conn's.
func readModeFile(socket string) (mode, bool) {
	b, err := os.ReadFile(modePath(socket))
	if err != nil {
		return mode{}, false
	}
	words := strings.Fields(string(b))
	m := mode{theme: defaultTheme, dark: len(words) == 0 || words[0] != "light"}
	if len(words) > 1 {
		if _, ok := themeNamed(words[1]); ok {
			m.theme = words[1]
		}
	}
	return m, true
}

// writeMode records the mode a fresh server comes up in, so a later
// conn - attaching, or asking for a theme - reads the same one back
// instead of asking the terminal again.
func writeMode(socket string, m mode) error {
	if err := os.MkdirAll(filepath.Dir(modePath(socket)), 0o700); err != nil {
		return err
	}
	ground := "dark"
	if !m.dark {
		ground = "light"
	}
	return os.WriteFile(modePath(socket), []byte(ground+" "+m.theme+"\n"), 0o600)
}

// serverMode is the mode the server on this socket came up in, or would
// if none is up yet: what the configuration says, until one has picked
// for itself.
func serverMode(socket, home string) mode {
	if m, ok := readModeFile(socket); ok {
		return m
	}
	return configMode(home)
}

// configMode is the mode the file asks for: its theme, and its ground
// where it names one. A file naming no ground is dark here — this is
// the mode for a conn with no server to read and nobody to ask, and
// dark is what every terminal was before conn learned to ask. askMode
// is where the terminal is asked.
func configMode(home string) mode {
	m := mode{theme: configTheme(home), dark: true}
	if dark, ok := groundNamed(configGround(home)); ok {
		m.dark = dark
	}
	return m
}

// configGround is the ground the file names, as written.
func configGround(home string) string {
	c, _ := readConfig(home)
	return c.Ground
}

// An override is what the flags said ahead of the command: a ground,
// when --light or --dark was given, and a theme, when --theme was.
type override struct {
	dark  *bool
	theme string
}

// over is the mode with what the flags said laid over it; what they
// did not say stands.
func (o override) over(m mode) mode {
	if o.dark != nil {
		m.dark = *o.dark
	}
	if o.theme != "" {
		m.theme = o.theme
	}
	return m
}

// askMode is the mode a fresh server comes up in: what the flags said,
// and for what they did not, what the configuration says — the theme,
// and the ground where it names one, the terminal's own asked fresh
// where it does not. A file naming a ground is an operator who wants
// the same one wherever they are; the question put to the terminal is
// for everybody else, which is most stations.
func askMode(o override, home string) mode {
	m := configMode(home)
	if o.dark == nil {
		if _, named := groundNamed(configGround(home)); !named {
			m.dark = detectDark()
		}
	}
	return o.over(m)
}

// parseModeFlags reads --light, --dark and --theme NAME off the front
// of conn's own arguments, before any command name: which ground and
// which theme to come up in, instead of asking the terminal, the
// configuration or a server's mode file. It stops at the first
// argument that is not one of these, dark or light or a command's own,
// and answers what is left of args from there, whole. --dark with
// --light, or --theme twice with two names, is a contradiction; a
// theme conn does not have is an error that names the ones it does.
// Neither leaves the choice where it always was.
func parseModeFlags(args []string) (rest []string, o override, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dark":
			if o.dark != nil && !*o.dark {
				return nil, override{}, fmt.Errorf("--dark and --light are a contradiction")
			}
			dark := true
			o.dark = &dark
		case "--light":
			if o.dark != nil && *o.dark {
				return nil, override{}, fmt.Errorf("--dark and --light are a contradiction")
			}
			light := false
			o.dark = &light
		case "--theme":
			if i+1 == len(args) {
				return nil, override{}, fmt.Errorf("--theme wants a theme's name: %s", themeNames())
			}
			name := args[i+1]
			if _, ok := themeNamed(name); !ok {
				return nil, override{}, fmt.Errorf("conn has no theme %s; it has %s", name, themeNames())
			}
			if o.theme != "" && o.theme != name {
				return nil, override{}, fmt.Errorf("--theme %s and --theme %s are a contradiction", o.theme, name)
			}
			o.theme = name
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
	for _, t := range themes {
		names = append(names, t.name)
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
	if !stdoutIsTerminal() || !stdinIsTerminal() {
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
	return isDark(bg)
}

// isDark reads a ground as dark or light by the same relative luminance
// a screen reader uses to say if text passes on it: below half is dark.
func isDark(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	luminance := 0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)
	return luminance/0xffff < 0.5
}
