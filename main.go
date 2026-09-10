package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
)

// dressProgram writes a theme for a program conn holds but cannot dress
// through its server, since the program writes its own hex rather than
// asking for a color by name: Claude Code and nvim. It answers
// what to say and whether it went well; ask says yes to a question, and
// is nil where there is nobody to ask.
func dressProgram(args []string, home string, ask func(string) bool) (string, bool) {
	if len(args) != 1 {
		return "conn theme: say which program: conn theme claude, conn theme vim\n", false
	}
	switch args[0] {
	case "claude":
		return dressClaude(home, ask)
	case "vim", "nvim":
		return dressVim(home)
	}
	return fmt.Sprintf("conn theme: conn has no theme for %s; it has one for claude and one for vim\n", args[0]), false
}

// dressClaude writes the theme, and offers to put Claude Code on it
// when nothing of the user's own is in the way: a settings file on one
// of the themes Claude Code comes with, or on none. A custom theme is
// somebody's own doing, and conn says what it is and leaves it.
func dressClaude(home string, ask func(string) bool) (string, bool) {
	path, err := writeClaudeTheme(home)
	if err != nil {
		return fmt.Sprintf("conn theme: %v\n", err), false
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Wrote conn's theme for Claude Code to %s\n", tilde(path, home))
	in, ok := themeInUse(home)
	switch {
	case !ok:
		fmt.Fprintf(&b, "Claude Code has no settings file yet; it will read the theme once %q is its theme.\n", claudeThemeRef)
	case in == claudeThemeRef:
		// Already on it, and the themes directory is watched: a session
		// that is up has the new colors already.
	case !builtinThemes[in]:
		fmt.Fprintf(&b, "Claude Code is on %s, which is not conn's to change. Pick Conn with /theme when you want it.\n", in)
	case ask != nil && ask("Put Claude Code on it now?"):
		if err := useClaudeTheme(home); err != nil {
			fmt.Fprintf(&b, "conn theme: %v\n", err)
			return b.String(), false
		}
		fmt.Fprintf(&b, "Claude Code is on %s. A session that is up picks it up as the directory is watched.\n", claudeThemeRef)
	default:
		fmt.Fprintf(&b, "Pick Conn with /theme, or set %q as the theme in ~/.claude/settings.json.\n", claudeThemeRef)
	}
	return b.String(), true
}

// asks is how conn puts a yes-or-no question, when there is somebody at
// the terminal to answer it and nowhere for the answer to be piped from.
func asks() func(string) bool {
	if !stdoutIsTerminal() || !stdinIsTerminal() {
		return nil
	}
	return func(q string) bool {
		fmt.Printf("%s [y/N] ", q)
		var answer string
		if _, err := fmt.Scanln(&answer); err != nil {
			return false // nothing typed, or no line to read: that is a no
		}
		answer = strings.ToLower(strings.TrimSpace(answer))
		return answer == "y" || answer == "yes"
	}
}

// conn comes up on its boot console, reads out the machine, runs its
// start-up checks, and continues to the watch. On a machine with tmux
// the first conn brings up a tmux server of its own, with conn as the
// rail of its home window and the slot beside it, and puts the terminal
// on it; a later conn attaches to what is there. conn down takes the
// server down with everything in it. Without tmux, conn shows the
// console and the watch and can reach nothing. Off a terminal it writes
// the console and is done.
// Everything else it was is in the history, and comes back piece by
// piece, in the form it is wanted in.
func main() {
	if holdEnv(os.Args) {
		home, _ := os.UserHomeDir()
		if err := runHold(findServer(home), colored()); err != nil {
			fmt.Fprintf(os.Stderr, "conn hold: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "theme" {
		home, _ := os.UserHomeDir()
		msg, ok := dressProgram(os.Args[2:], home, asks())
		if !ok {
			fmt.Fprint(os.Stderr, msg)
			os.Exit(1)
		}
		fmt.Print(msg)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "down" {
		home, _ := os.UserHomeDir()
		msg, ok := takeDown(findServer(home), home)
		if !ok {
			fmt.Fprint(os.Stderr, msg)
			os.Exit(1)
		}
		fmt.Print(msg)
		return
	}
	if !stdoutIsTerminal() {
		for _, r := range screen(compose(readStation(), time.Now()), minCols, 0, plain) {
			fmt.Println(r.text)
		}
		return
	}
	home, _ := os.UserHomeDir()
	srv := findServer(home)
	inside := srv != nil && insideConn(os.Getenv("TMUX"), srv.socket)
	if srv != nil && !inside {
		self, err := os.Executable()
		if err == nil {
			var code int
			code, err = srv.attach(self, home)
			if err == nil {
				os.Exit(code)
			}
		}
		// The server could not be brought up: conn goes on without it, and
		// says so.
		fmt.Fprintf(os.Stderr, "conn: the tmux server could not be brought up: %v\n", err)
		srv = nil
	}
	m := newModel(colored())
	m.srv, m.inside = srv, inside
	m.self, _ = os.Executable()
	if _, err := tea.NewProgram(m, programOptions()...).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "conn: %v\n", err)
		os.Exit(1)
	}
}

// programOptions are the options both of conn's programs run with. The
// palette is truecolor, and a terminal that says COLORTERM=truecolor is
// taken at its word: under tmux the renderer would otherwise ask tmux
// about the terminal's terminfo, which says nothing of the RGB the
// server was told to use, and draw the palette in 256 colors.
func programOptions() []tea.ProgramOption {
	switch strings.ToLower(os.Getenv("COLORTERM")) {
	case "truecolor", "24bit":
		return []tea.ProgramOption{tea.WithColorProfile(colorprofile.TrueColor)}
	}
	return nil
}
