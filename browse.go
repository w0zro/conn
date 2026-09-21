package main

import (
	"errors"
	"os/exec"
	"runtime"

	tea "charm.land/bubbletea/v2"
)

// A row that serves says a port, and the port is a place to go: o on it
// opens http://localhost:PORT where the operator can see it. It is the
// one key of conn's that acts outside the station — the browser is not
// a process of the server and gets no pane — and it is here because
// the panel already knows the port, and reading it off the row to type
// it into a browser by hand is the thing conn is for.
//
// The scheme is http. A dev server on this machine serves http, and
// what serves https on a port conn has no way of knowing; a browser
// sent to the wrong one of the two says so in a second.

// localURL is where a port of this machine is, as a browser is given it.
func localURL(port string) string {
	return "http://localhost:" + port
}

// browse opens a URL where the operator can see it, by the program the
// system opens things with: open on macOS, xdg-open on Linux, which is
// what the desktops answer to. Linux has an open of its own, for
// virtual terminals, so the name is not tried on both.
//
// The program is started and not waited on: what it opens stays up as
// long as the operator has it, and conn holding a pane's worth of
// attention on a browser would be a wait with no end. It is reaped in
// the background so nothing of conn's is left behind, and what it is
// given is a URL conn built, never a word the operator typed.
func browse(url string) error {
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	path := lookPath(name)
	if path == "" {
		return errors.New(name + " was not found on the path")
	}
	cmd := exec.Command(path, url)
	if err := cmd.Start(); err != nil {
		return errors.New("the browser could not be opened: " + err.Error())
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// openServing is o on a row that serves: the row's port in the
// browser, off the loop, since starting a program is not something to
// hold the panel's turn on. A row that says several ports is opened at
// the lowest, which is the first one it says; what conn cannot do it
// says in a notice under the rows, the way a shell it could not open
// is said.
func (m model) openServing(e entry) tea.Cmd {
	url := localURL(e.ports[0])
	return func() tea.Msg {
		if err := browse(url); err != nil {
			return noticeMsg{err.Error()}
		}
		return nil
	}
}
