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
	"os"
	"strings"

	"github.com/w0zro/conn/internal/tmux"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "conn:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "panel":
			return runPanel()
		default:
			return fmt.Errorf("conn takes no arguments; it opens on your projects")
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	srv, err := findServer(home)
	if err != nil {
		return err
	}
	// From inside conn, conn is ctrl-space away; this is the same.
	if insideConn(srv.Socket) {
		own, err := findPanel(srv, "")
		if err != nil || own.pane == "" {
			return fmt.Errorf("conn's own window is gone; leave with ctrl-b d and run conn again")
		}
		_, err = srv.Do(tmux.Cmd{"select-window", "-t", own.window})
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	code, err := attach(srv, self, home)
	if errors.Is(err, errEarlier) {
		if !ask("An earlier conn's server is up. End it, and everything running in it?") {
			return fmt.Errorf("left the earlier conn running")
		}
		if err := endServer(srv); err != nil {
			return err
		}
		code, err = attach(srv, self, home)
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
