package main

import (
	"errors"
	"os/exec"
	"runtime"
	"time"

	"github.com/w0zro/conn/internal/station"
)

// A row that serves says a port, and the port is a place to go: o on it
// opens localhost:PORT where the operator can see it. It is the one key
// of conn's that acts outside the station — the browser is not a
// process of the server and gets no pane — and it is here because the
// panel already knows the port, and reading it off the row to type it
// into a browser by hand is the thing conn is for.
//
// The scheme is asked for rather than assumed. Most of what runs on a
// port of this machine serves http, and some serves https — a stack
// held behind TLS to the last hop, which is a work machine more often
// than not — and a browser sent to the wrong one of the two lands on
// the server saying so, which is a question conn had the port in front
// of it to answer.

// serveWait is the longest a port is given to answer, dialling and
// again handshaking or answering a request. It is spent off the loop,
// in the command cc answers with, so the panel is not held for it.
const serveWait = time.Second

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
	path := station.LookPath(name)
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
