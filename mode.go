package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	term "github.com/charmbracelet/x/term"
	"golang.org/x/sys/unix"
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
// if none is up yet: conn's dark, until one has picked for itself.
func serverMode(socket string) mode {
	if m, ok := readModeFile(socket); ok {
		return m
	}
	return mode{theme: defaultTheme, dark: true}
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
// and for what they did not, conn's own theme on the terminal's own
// ground, asked fresh.
func askMode(o override) mode {
	m := mode{theme: defaultTheme, dark: true}
	if o.dark == nil {
		m.dark = detectDark()
	}
	return o.over(m)
}

// parseModeFlags reads --light and --dark off the front of conn's own
// arguments, before any command name: which ground to come up on,
// instead of asking the terminal or a server's mode file. It stops at
// the first argument that is not one of the two, dark or light or a
// command's own, and answers what is left of args from there, whole.
// The two flags together is a contradiction; neither leaves the choice
// where it always was.
func parseModeFlags(args []string) (rest []string, o override, err error) {
	for i, a := range args {
		switch a {
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
		default:
			return args[i:], o, nil
		}
	}
	return nil, o, nil
}

// detectDark asks the terminal for its own background with OSC 11 and
// reads what comes back. A terminal that says nothing within the wait,
// or says something conn cannot read, is dark - which is what every
// terminal was before conn asked, and the safe read of a query that
// went nowhere.
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

	if _, err := os.Stdout.WriteString("\x1b]11;?\x1b\\"); err != nil {
		return true
	}
	if !waitReadable(fd, 200*time.Millisecond) {
		return true
	}
	reply := readOSCReply(os.Stdin)
	dark, ok := parseBackground(reply)
	if !ok {
		return true
	}
	return dark
}

// waitReadable blocks until fd has a byte waiting or the wait passes.
// os.Stdin's own read deadline is what a query with nowhere to go was
// meant to lean on, but a pty's fd does not support one - Go answers
// SetReadDeadline with "file type does not support deadline" on one,
// silently, since the error was never checked - so the read that
// followed blocked forever on a terminal that never replies to OSC 11
// and never closes the pty either. Polling the raw fd first works on
// any fd, pty included.
func waitReadable(fd uintptr, wait time.Duration) bool {
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	n, err := unix.Poll(fds, int(wait.Milliseconds()))
	return err == nil && n > 0
}

// readOSCReply reads an OSC response one byte at a time until it ends -
// with BEL, or with ST (ESC \) - or the read stalls or a cap is hit.
func readOSCReply(r interface{ Read([]byte) (int, error) }) []byte {
	buf := make([]byte, 0, 64)
	one := make([]byte, 1)
	for len(buf) < 64 {
		n, err := r.Read(one)
		if n == 0 || err != nil {
			break
		}
		buf = append(buf, one[0])
		if one[0] == '\a' || (len(buf) >= 2 && buf[len(buf)-2] == 0x1b && buf[len(buf)-1] == '\\') {
			break
		}
	}
	return buf
}

// oscBackground finds OSC 11's color in its reply: rgb:RRRR/GGGG/BBBB,
// however many hex digits a channel came in.
var oscBackground = regexp.MustCompile(`rgb:([0-9a-fA-F]+)/([0-9a-fA-F]+)/([0-9a-fA-F]+)`)

// parseBackground reads OSC 11's reply as dark or light, by the same
// relative luminance a screen reader uses to say if text passes on a
// ground: below half is dark.
func parseBackground(reply []byte) (dark, ok bool) {
	m := oscBackground.FindSubmatch(reply)
	if m == nil {
		return false, false
	}
	channel := func(h []byte) float64 {
		v, err := strconv.ParseUint(string(h), 16, 64)
		if err != nil {
			return 0
		}
		max := uint64(1)<<(4*uint(len(h))) - 1
		return float64(v) / float64(max)
	}
	r, g, b := channel(m[1]), channel(m[2]), channel(m[3])
	luminance := 0.2126*r + 0.7152*g + 0.0722*b
	return luminance < 0.5, true
}
