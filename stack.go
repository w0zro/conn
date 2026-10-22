package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Where nobody has yet run a project's stack or its checks through
// conn, Claude Code is asked what they are, from the project's
// top-level files and the opening of the manifests that say how it
// runs. The answer is shown by the question standing in the workspace
// (see offer.go), and enter starts each process in a window of its
// own under its name, as conn u starts one, and writes it down as
// what the role means here (see roles.go).

// rolePrompt is what Claude Code is asked: the project by its files,
// the opening of each manifest that says how it runs, and the role.
func rolePrompt(root string, as role) string {
	var b strings.Builder
	b.WriteString(projectDescribed(root))
	if as == roleCheck {
		b.WriteString("\n\nList the checks a developer runs on this project on this machine before calling work done — the tests, the build, the linter, a typecheck — one a line, as NAME: COMMAND, NAME one lowercase word, COMMAND as typed into a shell at the project's directory, through the project's own scripts (npm test, make check) where it has them, since a tool installed in the project is not on the shell's PATH. Only what runs to completion and passes or fails; not a dev server, an install or a deploy. Nothing else, no code fence. If there are none, reply NONE.")
	} else {
		b.WriteString("\n\nList the long-running processes a developer starts to work on this project on this machine — a dev server, a worker, a watcher, a database it runs itself — one a line, as NAME: COMMAND, NAME one lowercase word, COMMAND as typed into a shell at the project's directory, through the project's own scripts (npm run dev, make dev) where it has them, since a tool installed in the project is not on the shell's PATH. Only what keeps running; not a build, a test or an install. Nothing else, no code fence. If there are none, reply NONE.")
	}
	return b.String()
}

// stackFiles are the files that say how a project runs; stackLines how
// much of each is put in the question.
var stackFiles = []string{"Procfile", "Procfile.dev", "package.json", "compose.yaml", "compose.yml", "docker-compose.yml", "Makefile", "justfile", "Cargo.toml", "go.mod", "pyproject.toml", "Gemfile", "README.md"}

const stackLines = 80

// topFiles is the names at the top of a directory, the first forty.
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

// A stacked is one process to start under a name: as Claude Code named
// it, or as it was last run here, in which case it is kept; and in
// which repository's root.
type stacked struct {
	name, command string
	root          string // blank for the project on the screen
	kept          bool
}

// stackName is the name of a line of the answer. A name can have a
// colon in it, as a script's does (check:watch).
var stackName = regexp.MustCompile(`^[a-z][a-z0-9:_-]*$`)

// stackLine reads one line of the answer, NAME: COMMAND: the name ends
// at the first colon a space follows, or else at the first colon.
func stackLine(l string) (name, command string, ok bool) {
	at, skip := strings.Index(l, ": "), 2
	if at < 0 {
		at, skip = strings.Index(l, ":"), 1
	}
	if at <= 0 {
		return "", "", false
	}
	name, command = strings.TrimSpace(l[:at]), strings.TrimSpace(l[at+skip:])
	return name, command, stackName.MatchString(name) && command != ""
}

// parseStack is the processes in an answer, in its order: every line
// that reads NAME: COMMAND, each name once, past fences and blanks.
func parseStack(out string) ([]stacked, error) {
	var list []stacked
	seen := map[string]bool{}
	for _, l := range strings.Split(out, "\n") {
		l = strings.Trim(strings.TrimSpace(l), "`")
		if strings.EqualFold(l, "none") {
			return nil, errors.New("it named nothing")
		}
		name, command, ok := stackLine(l)
		// A name conn u takes has no colon in it.
		name = strings.ReplaceAll(name, ":", "-")
		if !ok || seen[name] {
			continue
		}
		seen[name] = true
		list = append(list, stacked{name: name, command: command})
	}
	if len(list) == 0 {
		return nil, errors.New("the answer named nothing")
	}
	return list, nil
}

// askWait is how long Claude Code is given to answer.
const askWait = 90 * time.Second

// askClaude puts a question to Claude Code in print mode, with no tools,
// no servers of its own and nothing kept of it, and answers what it
// printed.
func askClaude(prompt string) (string, error) {
	bin, err := exec.LookPath("claude")
	if err != nil {
		return "", errors.New("claude is not on the PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), askWait)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-p", "--model", "haiku", "--tools", "", "--strict-mcp-config", "--no-session-persistence", prompt)
	cmd.Dir = os.TempDir()
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return "", errors.New("no answer in " + askWait.String())
	}
	if err != nil {
		return "", refusal(string(out), stderr.String(), err)
	}
	return string(out), nil
}

// refusal is why Claude Code did not answer, in its own words where it
// printed any: it says "Not logged in · Please run /login" on its
// output and exits 1, and the status alone says nothing of that.
func refusal(out, stderr string, err error) error {
	for _, s := range []string{stderr, out} {
		if l := lastLine(s); l != "" {
			return errors.New(l)
		}
	}
	return err
}

// lastLine is the last line of s with anything on it.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// roleAnswer is what a role means in a project, as Claude Code
// answered, or why it did not.
type roleAnswer struct {
	root string
	as   role
	list []stacked
	err  string
	// Asked again about runs that failed at once, Claude Code said
	// every command was right.
	kept bool
}

// askRole asks what a role means in a project, off the loop.
func askRole(p project, as role) tea.Cmd {
	return func() tea.Msg {
		out, err := askClaude(rolePrompt(p.Root, as))
		if err == nil {
			var list []stacked
			if list, err = parseStack(out); err == nil {
				return roleAnswer{root: p.Root, as: as, list: list}
			}
		}
		return roleAnswer{root: p.Root, as: as, err: err.Error()}
	}
}

// projectDescribed is a project as a question puts it: where it is,
// the names at its top, and the opening of each manifest that says how
// it runs.
func projectDescribed(root string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "A software project is at %s", root)
	if names := topFiles(root); len(names) > 0 {
		fmt.Fprintf(&b, ", holding %s", strings.Join(names, ", "))
	}
	b.WriteString(".")
	for _, name := range stackFiles {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		if len(lines) > stackLines {
			lines = lines[:stackLines]
		}
		fmt.Fprintf(&b, "\n\n%s begins:\n%s", name, strings.Join(lines, "\n"))
	}
	return b.String()
}
