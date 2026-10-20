// conn: one operator, many processes.
//
// conn opens on the projects work has happened in, read from what the
// machine records. In a project it lists what runs and the
// conversations Claude Code has had there; enter goes to one, picking it
// back up where it is at rest.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/w0zro/conn/internal/station"
	"github.com/w0zro/conn/internal/theme"
	"github.com/w0zro/conn/internal/tmux"
)

// version is what the release build stamps on the binary: what git
// describe says. A build that came another way has none, and the boot
// console reads the module system instead.
var version string

func main() {
	err := run(os.Stdout, os.Args[1:])
	var wrong usageError
	switch {
	case errors.As(err, &wrong):
		// A line conn could not read: what was wrong, and the use.
		fmt.Fprintf(os.Stderr, "conn: %s\n%s", wrong.wrong, uses)
		os.Exit(2)
	case err != nil:
		fmt.Fprintln(os.Stderr, "conn:", err)
		os.Exit(1)
	}
}

// uses is what conn answers to help, and under a line it could not
// read: every verb, a line each, with what it does.
const uses = `usage: conn [--light|--dark] [--theme NAME]   open on your projects; inside conn, back to the panel
       conn ls                              the processes of the project, a line each
       conn l NAME|:PORT|PID [-n N]         what one said, its last N lines
       conn u [NAME] [-w] -- COMMAND...     start a command here; -w waits, and exits as it did
       conn x NAME|:PORT|PID                end it
       conn r NAME|:PORT|PID                start it again
       conn down                            take the server down, and everything in it
       conn theme claude                    write conn's theme for Claude Code
       conn --version                       the build this is
`

// A usageError is a line conn could not read, and what was wrong with
// it; main says so and shows the use.
type usageError struct{ wrong string }

func (e usageError) Error() string { return e.wrong }

// versionLine is the build this is: the release, and where it is not
// exactly one, the commit it was built from, its date, and modified
// where the tree had changes past it.
func versionLine(stamp string) string {
	b := station.ReadBuild(stamp)
	line := "conn"
	if b.Tag != "" {
		line += " " + b.Tag
	}
	if b.Exact {
		return line
	}
	if b.Tag != "" {
		line += " (devel)"
	}
	if b.Commit != "" {
		line += " · " + b.Commit
	}
	if !b.Time.IsZero() {
		line += " · " + b.Time.UTC().Format("2006-01-02")
	}
	if b.Modified {
		line += " · modified"
	}
	return line
}

// commands are the verbs for whoever has no panel, by name.
var commands = map[string]func(w io.Writer, home, dir string, args []string) int{
	"ls":   runLs,
	"l":    runL,
	"u":    runU,
	"x":    runX,
	"r":    runR,
	"down": runDown,
}

func run(w io.Writer, args []string) error {
	args, want, err := theme.ParseFlags(args)
	if err != nil {
		return usageError{err.Error()}
	}
	if len(args) > 0 {
		switch args[0] {
		case "--version", "-V":
			_, err := fmt.Fprintln(w, versionLine(version))
			return err
		case "help", "-h", "--help":
			_, err := fmt.Fprint(w, uses)
			return err
		case "panel":
			return runPanel(args[1:])
		case "hold":
			return runHold()
		case "theme":
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			return runTheme(w, home, want, args[1:])
		}
		if cmd, ok := commands[args[0]]; ok {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			os.Exit(cmd(w, home, dir, args[1:]))
		}
		return usageError{"no verb " + args[0]}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	srv, err := findServer(home)
	if err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	// From inside conn, conn is ctrl-space away; this is the same. A
	// flag given there puts the server on the ground it names where it
	// stands.
	if insideConn(srv.Socket) {
		if want.Any() {
			return reground(srv, self, home, want)
		}
		own, err := findPanel(srv, "")
		if err != nil || own.pane == "" {
			return fmt.Errorf("conn's own window is gone; leave with ctrl-b d and run conn again")
		}
		_, err = srv.Do(tmux.Cmd{"select-window", "-t", own.window}, tmux.SelectPane(own.pane))
		return err
	}
	code, err := attach(srv, self, home, want)
	if errors.Is(err, errEarlier) {
		if !ask("An earlier conn's server is up. End it, and everything running in it?") {
			return fmt.Errorf("left the earlier conn running")
		}
		if err := endServer(srv); err != nil {
			return err
		}
		code, err = attach(srv, self, home, want)
	}
	if err != nil {
		return err
	}
	if code != 0 {
		os.Exit(code)
	}
	return nil
}

// ask puts a yes-or-no question on the terminal; no is the answer to
// anything but yes.
func ask(q string) bool {
	fmt.Fprintf(os.Stderr, "%s [y/N] ", q)
	a, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	a = strings.ToLower(strings.TrimSpace(a))
	return a == "y" || a == "yes"
}

// runTheme writes conn's theme for a program that draws its own hex
// rather than asking for a color by name, which is the one thing in a
// pane conn's server cannot dress: Claude Code. The ground is what a
// flag says, what the server is on, or the terminal's own.
func runTheme(w io.Writer, home string, want theme.Want, args []string) error {
	if len(args) != 1 || args[0] != "claude" {
		return usageError{"conn theme takes claude, and nothing else"}
	}
	srv, err := findServer(home)
	if err != nil {
		return err
	}
	g, err := askGround(srv, home, want)
	if err != nil {
		return err
	}
	return dressClaude(w, home, ask, g)
}

// dressClaude writes the theme, and offers to put Claude Code on it
// when nothing of the user's own is in the way: a settings file on one
// of the themes Claude Code comes with, or on none. A custom theme is
// somebody's own doing, and conn says what it is and leaves it.
func dressClaude(w io.Writer, home string, ask func(string) bool, g theme.Ground) error {
	path, err := theme.WriteClaudeTheme(home, g)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(w, "Wrote conn's theme for Claude Code to %s\n", tilde(path, home))
	in, ok := theme.ThemeInUse(home)
	switch {
	case !ok:
		_, _ = fmt.Fprintf(w, "Claude Code has no settings file yet; it will read the theme once %q is its theme.\n", theme.ClaudeThemeRef)
	case in == theme.ClaudeThemeRef:
		// Already on it, and the themes directory is watched: a session
		// that is up has the new colors already.
	case !theme.BuiltinThemes[in]:
		_, _ = fmt.Fprintf(w, "Claude Code is on %s, which is not conn's to change. Pick Conn with /theme when you want it.\n", in)
	case ask("Put Claude Code on it now?"):
		if err := theme.UseClaudeTheme(home); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(w, "Claude Code is on %s. A session that is up picks it up as the directory is watched.\n", theme.ClaudeThemeRef)
	default:
		_, _ = fmt.Fprintf(w, "Pick Conn with /theme, or set %q as the theme in ~/.claude/settings.json.\n", theme.ClaudeThemeRef)
	}
	return nil
}
