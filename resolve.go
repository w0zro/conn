package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/w0zro/conn/internal/shell"
	"github.com/w0zro/conn/internal/station"
	"github.com/w0zro/conn/internal/work"
	"github.com/w0zro/conn/internal/work/claude"
)

// What a verb means for a process conn has no word for is asked, not
// written into conn: cc on a server conn knows no client for is a
// question put to Claude Code, with the process and its project as
// what it is asked about, and the answer is a command offered on the
// key bar the way x offers its kill — y runs it, any other key lets it
// go. Nothing is kept: every press asks again, until the asking is
// worth making quicker.
//
// A port that speaks the web is not asked about. The browser is its
// client, and a request to the port answers that in a moment, where
// asking takes seconds.

// askWait is how long conn waits on Claude Code's answer.
const askWait = 60 * time.Second

// talkTo is cc on a row with no client conn knows: the port in the
// browser where it speaks the web, and otherwise the question asked,
// off the loop.
func (m model) talkTo(e work.Entry, pl work.Project) tea.Cmd {
	port := ""
	if serving(e) {
		port = e.Ports[0]
	}
	name := work.Program(e.AsTyped())
	prompt := clientPrompt(e, pl.Path)
	dir := pl.Path
	return func() tea.Msg {
		if port != "" {
			if scheme, ok := speaksWeb(port); ok {
				if err := browse(scheme + "://localhost:" + port); err != nil {
					return noticeMsg{err.Error()}
				}
				return nil
			}
		}
		line, err := askClaude(prompt)
		if err != nil {
			return noticeMsg{"claude would not say what talks to " + name + ": " + err.Error()}
		}
		return resolvedMsg{command: line, about: name, dir: dir}
	}
}

// resolvedMsg is a command Claude Code answered for a verb, to be
// offered before it runs.
type resolvedMsg struct {
	command, about, dir string
}

// offer arms the answer as a question: y opens it in a pane at the
// project and goes in, as a client conn knows is opened.
func (m model) offer(msg resolvedMsg) model {
	srv := m.srv
	line := msg.command + " 2>&1 || " + shell.HoldOpen
	m.kill = &pendingKill{prompt: question(msg.command, msg.about), end: func() tea.Msg {
		sh, err := srv.OpenCmd(msg.dir, line)
		if err != nil {
			return noticeMsg{"the session could not be opened: " + err.Error()}
		}
		return openedMsg{shell: sh}
	}}
	return m
}

// clientPrompt is what Claude Code is asked for cc: the process as the
// panel knows it, and the project it works in by its top-level files,
// which say what the stack is.
func clientPrompt(e work.Entry, project string) string {
	var b strings.Builder
	b.WriteString("A process runs on this machine: ")
	b.WriteString(e.AsTyped())
	if len(e.Ports) > 0 {
		fmt.Fprintf(&b, ", listening on port %s on localhost", strings.Join(e.Ports, ", "))
	}
	if e.Cwd != "" {
		fmt.Fprintf(&b, ", working in %s", e.Cwd)
	}
	if e.Container != "" {
		fmt.Fprintf(&b, ", in the docker container %s", e.Container)
	}
	b.WriteString(".")
	if names := topFiles(project); len(names) > 0 {
		fmt.Fprintf(&b, " Its project is %s, holding %s.", project, strings.Join(names, ", "))
	}
	b.WriteString(" Reply with one shell command, and nothing else, no code fence, that opens an interactive client session talking to this process — psql for postgres, redis-cli for redis, a REPL for its runtime. If nothing talks to it, reply NONE.")
	return b.String()
}

// topFiles is the names at the top of a project, a few dozen at most.
func topFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if len(out) == 40 {
			break
		}
		out = append(out, e.Name())
	}
	return out
}

// askClaude puts a question to Claude Code in print mode, with no
// tools and no servers of its own, and answers its one line.
func askClaude(prompt string) (string, error) {
	bin := station.LookPath(claude.Program)
	if bin == "" {
		return "", errors.New(claude.Program + " is not on the path")
	}
	ctx, cancel := context.WithTimeout(context.Background(), askWait)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-p", "--model", "haiku", "--tools", "", "--strict-mcp-config", "--no-session-persistence", prompt)
	cmd.Dir = os.TempDir()
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return "", errors.New("no answer in " + askWait.String())
	}
	if err != nil {
		return "", err
	}
	return answerLine(string(out))
}

// answerLine is the command in an answer: its first line that is not a
// fence or blank. NONE is no command.
func answerLine(out string) (string, error) {
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l == "" || strings.HasPrefix(l, "```") {
			continue
		}
		l = strings.Trim(l, "`")
		if strings.EqualFold(l, "none") {
			return "", errors.New("nothing talks to it")
		}
		return l, nil
	}
	return "", errors.New("the answer was empty")
}

// speaksWeb says whether a port of this machine answers as the web
// does, and by which scheme: a TLS handshake, or an HTTP status line to
// a request. A port that does neither within serveWait is not a page.
func speaksWeb(port string) (string, bool) {
	addr := net.JoinHostPort("localhost", port)
	d := &net.Dialer{Timeout: serveWait}
	if c, err := tls.DialWithDialer(d, "tcp", addr, &tls.Config{InsecureSkipVerify: true}); err == nil { //nolint:gosec // telling a scheme, not trusting a peer; see localScheme
		_ = c.Close()
		return "https", true
	}
	c, err := net.DialTimeout("tcp", addr, serveWait)
	if err != nil {
		return "", false
	}
	defer func() { _ = c.Close() }()
	if err := c.SetDeadline(time.Now().Add(serveWait)); err != nil {
		return "", false
	}
	if _, err := c.Write([]byte("HEAD / HTTP/1.0\r\nHost: localhost\r\n\r\n")); err != nil {
		return "", false
	}
	line, err := bufio.NewReader(c).ReadString('\n')
	if err != nil && line == "" {
		return "", false
	}
	return "http", strings.HasPrefix(line, "HTTP/")
}
