package main

import (
	"path"
	"slices"
	"strings"
)

// A knownProgram is a program conn knows by name: a server with a client of its own,
// which is more use on its row than a shell near it. postgres is the
// first. A row is the program where its command says so, or the brew
// formula it was started as, or the docker image it runs; and S on it
// is a session with the server by that client, in a pane of the
// station, where s is still a shell at the project.
type knownProgram struct {
	client   string   // the client, by name: what S runs and what the bar says
	commands []string // the server's process names
	formula  string   // the brew formula, before any @version
	image    string   // the docker image, before any tag
	// args is what the client is given to reach the server listening
	// on a port of this machine.
	args func(port string) string
	// inContainer is the client's whole line inside the container, as
	// the user the image was given; userEnv names that user in the
	// container's environment, and user is who it is unless set.
	inContainer func(user string) string
	userEnv     string
	user        string
}

var knownPrograms = []knownProgram{{
	client:      "psql",
	commands:    []string{"postgres", "postmaster"},
	formula:     "postgresql",
	image:       "postgres",
	args:        func(port string) string { return "-h localhost -p " + port + " postgres" },
	inContainer: func(user string) string { return "psql -U " + user },
	userEnv:     "POSTGRES_USER",
	user:        "postgres",
}}

// programOf is the program a row is, or nil. A brew service is known by
// its formula and a container by its image, since neither row's
// command is the server's own; anything else by the process name, its
// own or the listener's it folded, since a postgres under a shell is
// the shell's row on the panel and the port on it is the server's.
func programOf(e entry, c *container) *knownProgram {
	for i := range knownPrograms {
		p := &knownPrograms[i]
		switch {
		case e.brew != "":
			if formulaBase(e.brew) == p.formula {
				return p
			}
		case c != nil:
			if imageBase(c.image) == p.image {
				return p
			}
		default:
			for _, command := range []string{e.command, e.listener} {
				if command != "" && slices.Contains(p.commands, path.Base(program(command))) {
					return p
				}
			}
		}
	}
	return nil
}

// formulaBase is a formula's name before its version and after its
// tap: postgresql for homebrew/core/postgresql@14.
func formulaBase(formula string) string {
	name, _, _ := strings.Cut(formula, "@")
	return path.Base(name)
}

// imageBase is an image's name alone: postgres for
// docker.io/library/postgres:16, or for postgres@sha256:….
func imageBase(image string) string {
	image = path.Base(image)
	image, _, _ = strings.Cut(image, "@")
	image, _, _ = strings.Cut(image, ":")
	return image
}

// programUnder is the program a row is where a session with it can be
// opened: in its container, or on the port it listens on. A server
// with no port conn can see, a brew service that is down, has nothing
// to connect to.
func (m model) programUnder(e entry) *knownProgram {
	p := programOf(e, m.containerAt(e.pid))
	if p == nil || e.container == "" && len(e.ports) == 0 {
		return nil
	}
	return p
}
