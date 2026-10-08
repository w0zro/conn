package main

import "github.com/w0zro/conn/internal/wire"

// Where the panel's cursor is, for the page to follow: see
// internal/wire, which carries it. The subject is the panel's and the
// page's to know; the note carries its three fields.

// A subject is what the page is about: a process, by its pid; a
// project, by its path; or a suspended session, by its id. The
// processes view's cursor is always on a process; the list's cursor
// stands on projects as often as not, and the sessions list's on
// sessions, and each is as much a thing to read about as a row.
type subject struct {
	pid     int
	path    string // a project, where pid is 0
	session string // a suspended session, where both are empty
}

// none says whether there is a subject at all.
func (s subject) none() bool { return s.pid == 0 && s.path == "" && s.session == "" }

// tellCursor publishes the subject under the panel's cursor, with the
// reading it stands in.
func tellCursor(path string, at subject, r *wire.Reading) {
	wire.Publish(path, wire.Note{PID: at.pid, Path: at.path, Session: at.session, Reading: r})
}

// parseCursor reads a note: the subject the panel's cursor is on, or
// none where there is none to read, and the reading the panel
// published beside it, where it did.
func parseCursor(raw string) (subject, *wire.Reading) {
	n, ok := wire.Parse(raw)
	if !ok {
		return subject{}, nil
	}
	return subject{pid: n.PID, path: n.Path, session: n.Session}, n.Reading
}

// askCursor is the note read and parsed in one go.
func askCursor(path string) (subject, *wire.Reading) {
	return parseCursor(wire.Read(path))
}
