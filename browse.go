package main

import (
	"crypto/tls"
	"errors"
	"net"
	"os/exec"
	"runtime"
	"time"

	tea "charm.land/bubbletea/v2"
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
// again handshaking. It is spent off the loop, in the command o
// answers with, so the panel is not held for it.
const serveWait = time.Second

// localURL is where a port of this machine is, as a browser is given
// it, under the scheme that port answers to.
func localURL(port string) string {
	return localScheme(port) + "://localhost:" + port
}

// localScheme asks a port of this machine whether it speaks TLS, by
// offering a handshake and seeing whether one comes back. Anything
// else is http: an http server reads the hello as a request line and
// answers 400, a port that has gone between the reading and the key is
// not dialled at all, and a server too slow to answer within serveWait
// is a server conn will not hold the operator up over. http is what a
// port is when nothing says otherwise, and the far more common of the
// two.
//
// The certificate is not checked. conn sends nothing to the port and
// is not asking who it is; it is telling one scheme from the other,
// and a dev server's certificate is signed by nobody in particular.
// The browser makes up its own mind about the certificate a moment
// later, which is where that question belongs and where the operator
// can answer it.
func localScheme(port string) string {
	c, err := net.DialTimeout("tcp", net.JoinHostPort("localhost", port), serveWait)
	if err != nil {
		return "http"
	}
	defer func() { _ = c.Close() }()
	if err := c.SetDeadline(time.Now().Add(serveWait)); err != nil {
		return "http"
	}
	t := tls.Client(c, &tls.Config{InsecureSkipVerify: true, ServerName: "localhost"})
	if err := t.Handshake(); err != nil {
		return "http"
	}
	return "https"
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
	port := e.ports[0]
	return func() tea.Msg {
		if err := browse(localURL(port)); err != nil {
			return noticeMsg{err.Error()}
		}
		return nil
	}
}
