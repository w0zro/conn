package theme

import (
	"os"
	"path/filepath"
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

// ReadMode is the ground written at ModePath, and whether one was: a
// server that has not chosen yet has nothing there. The file is one
// line, the ground first: "light", or "light conn" from a build that
// named the theme too.
func ReadMode(socket string) (dark, ok bool) {
	b, err := os.ReadFile(ModePath(socket))
	if err != nil {
		return false, false
	}
	words := strings.Fields(string(b))
	return len(words) == 0 || words[0] != "light", true
}

// WriteMode records the ground a fresh server comes up on, so a later
// conn reads the same one back instead of asking the terminal again.
func WriteMode(socket string, dark bool) error {
	if err := os.MkdirAll(filepath.Dir(ModePath(socket)), 0o700); err != nil {
		return err
	}
	ground := "dark"
	if !dark {
		ground = "light"
	}
	return os.WriteFile(ModePath(socket), []byte(ground+" conn\n"), 0o600)
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
