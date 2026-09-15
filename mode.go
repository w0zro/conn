package main

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	term "github.com/charmbracelet/x/term"
	"golang.org/x/sys/unix"
)

// conn comes up on one of two grounds. Dark is every terminal it ever
// knew; light is for the terminal that says its own ground is light
// when conn asks. The choice is made once, the first time conn brings
// up a tmux server that is not there yet, and holds for that server's
// life: conn down and a relaunch is how it is asked again. A run with
// no server behind it - no tmux on the machine, or a pane of conn's own
// server, where the server already chose - asks fresh or reads what the
// server chose, in place of guessing.
//
// Light is not dark with the lightness flipped. On paper, emphasis is
// more ink, not more light, so the light scheme's bright slots go
// darker than its normal ones - the opposite of the dark scheme, where
// bright is lighter.
//
// The two neutral slots are the exception, and keep what every program
// means by them: 0 is black, which on paper is ordinary text and the
// strongest ink there is, and 8 is the gray a program dims with. They
// are not a border and a quieter border; a program writing ANSI-0
// expects to be read.

// darkMode is the ground conn is on, as applyMode last left it. The
// ground is package-wide — scheme, the hexes, themeBase and the rest
// are all set from it at once — and nothing else names which one is in
// force, so a caller that needs to put it back has to read the answer
// out of one of the colors. It is dark until applyMode says otherwise,
// which is what every terminal was before conn learned to ask.
var darkMode = true

// The two grounds and the two inks.
var (
	darkGround  = color.RGBA{R: 21, G: 19, B: 15, A: 255}
	darkInk     = color.RGBA{R: 230, G: 223, B: 208, A: 255}
	lightGround = color.RGBA{R: 0xEF, G: 0xE9, B: 0xDB, A: 255}
	lightInk    = color.RGBA{R: 0x1A, G: 0x16, B: 0x11, A: 255}
)

// The sixteen, dark and light. darkScheme is what scheme was before
// there was a choice; lightScheme is the same table, on paper.
var darkScheme = [16]string{
	"#2A2620", // black
	"#FF7847", // red
	"#93C98B", // green
	"#E3A94F", // yellow
	"#7FA7C9", // blue
	"#C98BA8", // magenta
	"#7FC7BD", // cyan
	"#BFB39A", // white
	"#5C564A", // bright black
	"#E85D2F", // bright red
	"#A8DBA0", // bright green
	"#F2C06E", // bright yellow
	"#9BBEDB", // bright blue
	"#DBA6C0", // bright magenta
	"#9AD9D0", // bright cyan
	"#E6DFD0", // bright white
}

var lightScheme = [16]string{
	"#2B2620", // black
	"#A63214", // red
	"#23703F", // green
	"#8A5F00", // yellow
	"#3E5F7A", // blue
	"#7A4258", // magenta
	"#0D6B70", // cyan
	"#4A4335", // white
	"#9A9080", // bright black
	"#BD3A1D", // bright red
	"#1C5A33", // bright green
	"#75500A", // bright yellow
	"#32506A", // bright blue
	"#68384B", // bright magenta
	"#0A585D", // bright cyan
	"#1A1611", // bright white
}

const (
	darkCursorHex  = "#E85D2F"
	lightCursorHex = "#BD3A1D"
)

// The border, and the grounds that go with it: a pane's edge, a
// selection, the band behind what you said. On dark this is scheme[0],
// which is the darkest thing there is and so the quietest
// edge. On light it cannot be: light's scheme[0] is black, and black is
// what a program writing ANSI-0 means by ordinary text — Claude Code
// writes the unchanged lines of a diff in it. A border pale enough to
// be an edge on paper is #D8D0BD, which is 1.27:1 against lightGround
// and unreadable as text, so the two part company here rather than in
// the sixteen.
const (
	darkBorderHex  = "#2A2620"
	lightBorderHex = "#D8D0BD"
)

// The gray of the console's second rank, light; the dark one is grayHex
// in theme.go, renamed darkGrayHex.
const lightGrayHex = "#6F6656"

// The quietest tier of text, light; the dark one is faintHex in
// theme.go, renamed darkFaintHex. Apart from scheme[8] (ANSI-8, light's
// #9A9080), which stays exactly as it was for the pane's own sake - see
// the comment on darkFaintHex.
const lightFaintHex = "#867C6A"

// The grounds no slot has a name for, light: the same washes and bars
// theme.go names dark, in the light ground's own temperature. A wash
// this pale needs more room from the ground than the same wash does on
// dark to read as a color at all rather than a shade of the ground
// itself - lightness compresses toward white long before it compresses
// toward black. Chosen, like lightFaintHex, by holding each one to at
// least the contrast its dark counterpart already has against its own
// ground (WCAG ratio; e.g. dark's diffAddedWord is 1.65:1 against
// darkGround, light's #C2D4B0 was only 1.30:1 against lightGround -
// #A4C187 is what 1.65:1 costs on the same hue).
const (
	lightDiffAddedBg     = "#CAD7BB"
	lightDiffRemovedBg   = "#E8D3C4"
	lightDiffAddedDim    = "#DFE1CD"
	lightDiffRemovedDim  = "#EBDED0"
	lightDiffAddedWord   = "#A4C187"
	lightDiffRemovedWord = "#E0BDA4"
	lightMessageHoverBg  = "#CFC6B0"
	lightToolBg          = "#E6DFCF"
)

// applyMode puts every color conn draws from onto one ground. In conn
// it is called once, before anything reads scheme, groundColor,
// cursorHex, or any of the rest. A test binary is one process running
// every test, so a test that calls it — or calls something that calls
// it, which dressProgram does on its way to writing a theme — leaves
// the ground it chose standing for whatever runs next; see holdMode.
func applyMode(dark bool) {
	darkMode = dark
	if dark {
		groundColor, inkColor = darkGround, darkInk
		scheme = darkScheme
		cursorHex, borderHex = darkCursorHex, darkBorderHex
		grayHex = darkGrayHex
		faintHex = darkFaintHex
		diffAddedBg, diffRemovedBg = darkDiffAddedBg, darkDiffRemovedBg
		diffAddedDim, diffRemovedDim = darkDiffAddedDim, darkDiffRemovedDim
		diffAddedWord, diffRemovedWord = darkDiffAddedWord, darkDiffRemovedWord
		messageHoverBg, toolBg = darkMessageHoverBg, darkToolBg
		themeBase, vimBackground = "dark-ansi", "dark"
		return
	}
	groundColor, inkColor = lightGround, lightInk
	scheme = lightScheme
	cursorHex, borderHex = lightCursorHex, lightBorderHex
	grayHex = lightGrayHex
	faintHex = lightFaintHex
	diffAddedBg, diffRemovedBg = lightDiffAddedBg, lightDiffRemovedBg
	diffAddedDim, diffRemovedDim = lightDiffAddedDim, lightDiffRemovedDim
	diffAddedWord, diffRemovedWord = lightDiffAddedWord, lightDiffRemovedWord
	messageHoverBg, toolBg = lightMessageHoverBg, lightToolBg
	themeBase, vimBackground = "light-ansi", "light"
}

// modePath is where the mode a server came up on is kept, beside its
// socket and its tmux.conf.
func modePath(socket string) string {
	return filepath.Join(filepath.Dir(socket), "mode")
}

// readModeFile is the mode written at modePath, and whether one was:
// a server that has not picked yet has nothing there.
func readModeFile(socket string) (dark, ok bool) {
	b, err := os.ReadFile(modePath(socket))
	if err != nil {
		return false, false
	}
	return strings.TrimSpace(string(b)) != "light", true
}

// writeMode records the mode a fresh server comes up on, so a later
// conn - attaching, or asking for a theme - reads the same one back
// instead of asking the terminal again.
func writeMode(socket string, dark bool) error {
	if err := os.MkdirAll(filepath.Dir(modePath(socket)), 0o700); err != nil {
		return err
	}
	mode := "dark"
	if !dark {
		mode = "light"
	}
	return os.WriteFile(modePath(socket), []byte(mode), 0o600)
}

// serverMode is the mode the server on this socket came up on, or would
// if none is up yet: dark, until one has picked light for itself.
func serverMode(socket string) bool {
	dark, ok := readModeFile(socket)
	return !ok || dark
}

// detectDark asks the terminal for its own background with OSC 11 and
// reads what comes back. A terminal that says nothing within the wait,
// or says something conn cannot read, is dark - which is what every
// terminal was before conn asked, and the safe read of a query that
// went nowhere.
// askDark is a ground already chosen - a --light or --dark flag - or
// the terminal's own, asked fresh.
func askDark(override *bool) bool {
	if override != nil {
		return *override
	}
	return detectDark()
}

// parseModeFlags reads --light and --dark off the front of conn's own
// arguments, before any command name: which ground to come up on,
// instead of asking the terminal or a server's mode file. It stops at
// the first argument that is not one of the two, dark or light or a
// command's own, and answers what is left of args from there, whole.
// The two flags together is a contradiction; neither leaves the choice
// where it always was.
func parseModeFlags(args []string) (rest []string, override *bool, err error) {
	for i, a := range args {
		switch a {
		case "--dark":
			if override != nil && !*override {
				return nil, nil, fmt.Errorf("--dark and --light are a contradiction")
			}
			dark := true
			override = &dark
		case "--light":
			if override != nil && *override {
				return nil, nil, fmt.Errorf("--dark and --light are a contradiction")
			}
			light := false
			override = &light
		default:
			return args[i:], override, nil
		}
	}
	return nil, override, nil
}

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
