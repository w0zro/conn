package main

import (
	"errors"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// c talks to a row's process: a session with it, the keys in it. A
// conversation or a shell is a terminal already, and c goes into it.
// A port that answers HTTP is a web server, and its client is the
// browser. Anything else — a database, a cache, a container — has a
// client conn does not know; Claude Code is asked once what opens a
// session with it, the answer offered, and enter's command kept under
// the row's name as what c means for it here, as a check's command is
// kept (see roles.go). A client is a URL, opened in the browser, or a
// command, run in a window of its own at the project's root.

// clientPrompt is what Claude Code is asked: the project by its files,
// the process by what it is, and for the one line that talks to it.
func clientPrompt(root, name, command, dir string, ports []string) string {
	var b strings.Builder
	b.WriteString(projectDescribed(root))
	fmt.Fprintf(&b, "\n\nIn it, a process named %s", name)
	if command != "" && command != name {
		fmt.Fprintf(&b, " runs %s", command)
	}
	if dir != "" && dir != root {
		fmt.Fprintf(&b, " at %s", dir)
	}
	if len(ports) > 0 {
		fmt.Fprintf(&b, ", listening on port %s", strings.Join(ports, " and "))
	}
	b.WriteString(".\n\nAnswer with one line and nothing else: what opens an interactive session with that process on this machine — the one shell command as typed at the project's directory, with the host, port, user, database and password its configuration says (psql for a postgres, redis-cli for a redis, a shell in a container for a container, through the project's own scripts where it has them), or the URL to open in a browser for a web server. No code fence, no explanation. If nothing talks to it, reply NONE.")
	return b.String()
}

// parseClient reads the answer: its first line that says anything,
// past fences; NONE, or nothing, is no client.
func parseClient(out string) (string, error) {
	for _, l := range strings.Split(out, "\n") {
		l = strings.Trim(strings.TrimSpace(l), "`")
		if l == "" || strings.HasPrefix(l, "```") {
			continue
		}
		if strings.EqualFold(l, "none") {
			return "", errors.New("nothing talks to it")
		}
		return l, nil
	}
	return "", errors.New("the answer said nothing")
}

// isURL says whether a client is a URL for the browser rather than a
// command for a shell.
func isURL(client string) bool {
	return strings.HasPrefix(client, "http://") || strings.HasPrefix(client, "https://")
}

// probeWait is how long a port is given to answer.
const probeWait = time.Second

// speaksHTTP says whether what listens on a port answers HTTP: asked
// for /, it begins its answer with HTTP/. Something that is not a web
// server says nothing, or something else, and the question does it no
// harm.
func speaksHTTP(port string) bool {
	c, err := net.DialTimeout("tcp", "localhost:"+port, probeWait)
	if err != nil {
		return false
	}
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(probeWait))
	if _, err := fmt.Fprintf(c, "GET / HTTP/1.1\r\nHost: localhost:%s\r\nConnection: close\r\n\r\n", port); err != nil {
		return false
	}
	buf := make([]byte, 5)
	n, _ := c.Read(buf)
	return string(buf[:n]) == "HTTP/"
}

// openURL opens a URL in the browser.
func openURL(url string) error {
	opener := "open"
	if runtime.GOOS != "darwin" {
		opener = "xdg-open"
	}
	return exec.Command(opener, url).Run()
}

// askClient asks what talks to a process, off the loop.
func askClient(p project, name, command, dir string, ports []string) tea.Cmd {
	return func() tea.Msg {
		out, err := askClaude(clientPrompt(p.Root, name, command, dir, ports))
		if err == nil {
			var client string
			if client, err = parseClient(out); err == nil {
				return roleAnswer{root: p.Root, as: roleClient, list: []stacked{{name: name, command: client, root: p.Root}}}
			}
		}
		return roleAnswer{root: p.Root, as: roleClient, err: err.Error()}
	}
}
