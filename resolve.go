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
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/w0zro/conn/internal/room"
	"github.com/w0zro/conn/internal/shell"
	"github.com/w0zro/conn/internal/station"
	"github.com/w0zro/conn/internal/work"
	"github.com/w0zro/conn/internal/work/claude"
	"github.com/w0zro/conn/internal/work/declared"
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

// askClaude puts a question to Claude Code and answers its one line.
func askClaude(prompt string) (string, error) {
	out, err := askClaudeRaw(prompt)
	if err != nil {
		return "", err
	}
	return answerLine(out)
}

// askClaudeRaw puts a question to Claude Code in print mode, with no
// tools and no servers of its own, and answers what it printed.
func askClaudeRaw(prompt string) (string, error) {
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
	return string(out), nil
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
// a request. A port that does neither within serveWait is not a page,
// and is asked about instead.
//
// The certificate is not checked. conn is not asking who the port is;
// it is telling one scheme from another, and a dev server's
// certificate is signed by nobody in particular. The browser makes up
// its own mind about the certificate a moment later, which is where
// that question belongs and where the operator can answer it.
func speaksWeb(port string) (string, bool) {
	addr := net.JoinHostPort("localhost", port)
	d := &net.Dialer{Timeout: serveWait}
	if c, err := tls.DialWithDialer(d, "tcp", addr, &tls.Config{InsecureSkipVerify: true}); err == nil { //nolint:gosec // telling a scheme, not trusting a peer
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

// What makes up a project's stack, where the project does not declare
// it, is asked the same way: up on a project with no .conn puts its
// top-level files and what its manifests say to Claude Code, and the
// long-running processes it answers are offered on one question, each
// started on y in a pane of its own under its name, as conn u starts
// one. Nothing is kept: the next up asks again.

// stackPrompt is what Claude Code is asked for up: the project by its
// files, and the opening of each manifest that says how it runs.
func stackPrompt(project string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "A software project is at %s", project)
	if names := topFiles(project); len(names) > 0 {
		fmt.Fprintf(&b, ", holding %s", strings.Join(names, ", "))
	}
	b.WriteString(".")
	for _, name := range stackFiles {
		data, err := os.ReadFile(project + "/" + name)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		if len(lines) > stackLines {
			lines = lines[:stackLines]
		}
		fmt.Fprintf(&b, "\n\n%s begins:\n%s", name, strings.Join(lines, "\n"))
	}
	b.WriteString("\n\nList the long-running processes a developer starts to work on this project on this machine — a dev server, a worker, a watcher, a database it runs itself — one a line, as NAME: COMMAND, NAME one lowercase word, COMMAND run from the project's directory. Only what keeps running; not a build, a test or an install. Nothing else, no code fence. If there are none, reply NONE.")
	return b.String()
}

// stackFiles are the manifests that say how a project runs, read for
// up's question; stackLines how much of each.
var stackFiles = []string{"Procfile", "Procfile.dev", "package.json", "compose.yaml", "compose.yml", "docker-compose.yml", "Makefile", "justfile", "README.md"}

const stackLines = 80

// A stacked is one process of a stack as Claude Code answered it.
type stacked struct {
	name, command string
}

// stackLine is one line of the answer, NAME: COMMAND.
var stackLine = regexp.MustCompile(`^([a-z][a-z0-9-]*)\s*:\s*(\S.*)$`)

// parseStack is the processes in an answer, in its order: every line
// that reads NAME: COMMAND, each name once, past fences and blanks.
func parseStack(out string) ([]stacked, error) {
	var list []stacked
	seen := map[string]bool{}
	for _, l := range strings.Split(out, "\n") {
		l = strings.Trim(strings.TrimSpace(l), "`")
		if strings.EqualFold(l, "none") {
			return nil, errors.New("it runs nothing that keeps running")
		}
		m := stackLine.FindStringSubmatch(l)
		if m == nil || seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		list = append(list, stacked{m[1], strings.TrimSpace(m[2])})
	}
	if len(list) == 0 {
		return nil, errors.New("the answer named nothing")
	}
	return list, nil
}

// askStack asks for a project's stack, off the loop.
func (m model) askStack(project string) tea.Cmd {
	prompt := stackPrompt(project)
	return func() tea.Msg {
		out, err := askClaudeRaw(prompt)
		if err != nil {
			return noticeMsg{"claude would not say what runs " + filepath.Base(project) + ": " + err.Error()}
		}
		list, err := parseStack(out)
		if err != nil {
			return noticeMsg{"claude would not say what runs " + filepath.Base(project) + ": " + err.Error()}
		}
		return stackMsg{project: project, list: list}
	}
}

// stackMsg is a project's stack as Claude Code answered it.
type stackMsg struct {
	project string
	list    []stacked
}

// offerStack arms the stack as one question: y starts each process
// that is not up under its name, parked, as up starts a .conn's.
func (m model) offerStack(msg stackMsg) model {
	srv := m.srv
	var names []string
	for _, s := range msg.list {
		names = append(names, s.name+": "+s.command)
	}
	m.kill = &pendingKill{prompt: question("start "+strings.Join(names, "; "), filepath.Base(msg.project)), end: func() tea.Msg {
		var shells []room.Shell
		for _, s := range msg.list {
			sh, err := srv.RunNamed(msg.project, s.command, s.name, declared.Mark(msg.project, s.name), "")
			if err != nil {
				return noticeMsg{s.name + " could not be started: " + err.Error()}
			}
			shells = append(shells, sh)
		}
		return raisedMsg{shells: shells}
	}}
	return m
}
