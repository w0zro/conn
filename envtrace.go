package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// The environment block says what a variable is, not where it was set.
// The shell knows: run once more with -x and a PS4 that names the file
// and the line, it prints every command its startup files run, and the
// assignment of a variable is one of those lines. That is the source
// column's answer for a variable the rc files set — ~/.zshrc.local:5 —
// whether the shell is the pane's or the one conn was launched from,
// which ran the same files. The trace costs a start of the shell, so it
// runs after the page's first paint and the column fills in when it
// lands. What the trace does not find is looked for in the startup files
// by name, and said to be named there: a line the grep finds may be
// behind an if the trace did not take, or a comment.

// traceTimeout is how long the shell gets to start and be traced.
const traceTimeout = 8 * time.Second

// envTraceMsg is the trace's word: each variable the startup files set,
// by name, with the file and line that set it; the files the grep looked
// through, for the fallback; and the entries /etc/paths and /etc/paths.d
// put on PATH.
type envTraceMsg struct {
	origins map[string]string
	named   map[string]string
	paths   map[string]string
}

// traceStartup is the command that traces the shell and greps its files,
// off the render path.
func traceStartup(shell string) tea.Cmd {
	return func() tea.Msg {
		origins := traceShell(shell)
		named := map[string]string{}
		files := startupFiles(shell)
		for _, f := range files {
			for name, line := range namedIn(f) {
				if _, found := named[name]; !found {
					named[name] = line
				}
			}
		}
		return envTraceMsg{origins: origins, named: named, paths: systemPaths("/etc")}
	}
}

// traceArgs is how a shell is run traced, and the PS4 that makes each
// line name its file and line: zsh's prompt escapes, bash's variables.
// A shell that is neither is not traced.
func traceArgs(shell string) (ps4 string, args []string) {
	switch filepath.Base(shell) {
	case "zsh":
		return "+%N:%i> ", []string{"-xlic", "exit"}
	case "bash":
		return "+${BASH_SOURCE}:${LINENO}> ", []string{"-xlic", "exit"}
	}
	return "", nil
}

// traceShell runs the shell once, traced, and reads where its startup
// files set each variable. The shell runs with this process's
// environment, which is a pane's — TMUX set, TERM tmux's — so a startup
// file that asks whether it is inside conn gets the pane's answer.
func traceShell(shell string) map[string]string {
	ps4, args := traceArgs(shell)
	if ps4 == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), traceTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, shell, args...)
	cmd.Env = append(os.Environ(), "PS4="+ps4)
	cmd.Stdin = nil
	var trace bytes.Buffer
	cmd.Stderr = &trace
	_ = cmd.Run() // the exit is the rc files' business; the trace is what is wanted
	return parseTrace(trace.String())
}

// traceLine is one traced command: the file, the line, the command.
var traceLine = regexp.MustCompile(`^\++(.*?):(\d+)> (.*)$`)

// assignment is a name being set at the start of a command or after a
// space: NAME=, and zsh's path= and path+= for PATH.
var assignment = regexp.MustCompile(`(?:^|\s)([A-Za-z_][A-Za-z0-9_]*)\+?=`)

// parseTrace reads a traced startup: for each variable set, the file and
// line of the last command that set it. A command that runs inside an
// eval or a function is traced under that name, not a file's; it is
// filed under the last line of a file seen, which is the eval or the
// call — brew shellenv's variables are the line that evals it.
func parseTrace(trace string) map[string]string {
	origins := map[string]string{}
	where := ""
	for line := range strings.SplitSeq(trace, "\n") {
		m := traceLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		file, lineNo, command := m[1], m[2], m[3]
		if strings.HasPrefix(file, "/") || strings.HasPrefix(file, "~") {
			where = homely(file) + ":" + lineNo
		}
		if where == "" {
			continue
		}
		for _, name := range assignedIn(command) {
			origins[name] = where
		}
	}
	return origins
}

// assignedIn is the variables a command sets: the assignments of an
// export, typeset, declare or readonly, and of a bare assignment. A
// command that merely has an = in an argument sets nothing.
func assignedIn(command string) []string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return nil
	}
	switch fields[0] {
	case "export", "typeset", "declare", "readonly", "local":
		command = strings.TrimSpace(strings.TrimPrefix(command, fields[0]))
		for strings.HasPrefix(command, "-") {
			_, rest, _ := strings.Cut(command, " ")
			command = strings.TrimSpace(rest)
		}
	default:
		if !strings.Contains(fields[0], "=") {
			return nil
		}
	}
	var names []string
	for _, m := range assignment.FindAllStringSubmatch(command, -1) {
		name := m[1]
		switch name {
		case "path":
			name = "PATH"
		case "manpath":
			name = "MANPATH"
		case "fpath", "cdpath":
			continue
		}
		names = append(names, name)
	}
	return names
}

// startupFiles is the files the shell reads as it starts, in the order it
// reads them, the system's then the user's, and the files those source,
// one level down: what a grep for a name looks through.
func startupFiles(shell string) []string {
	home, _ := os.UserHomeDir()
	var files []string
	switch filepath.Base(shell) {
	case "zsh":
		dot := os.Getenv("ZDOTDIR")
		if dot == "" {
			dot = home
		}
		files = []string{"/etc/zshenv", "/etc/zprofile", "/etc/zshrc", "/etc/zlogin",
			filepath.Join(dot, ".zshenv"), filepath.Join(dot, ".zprofile"), filepath.Join(dot, ".zshrc"), filepath.Join(dot, ".zlogin")}
	case "bash":
		files = []string{"/etc/profile", "/etc/bash.bashrc", "/etc/bashrc",
			filepath.Join(home, ".bash_profile"), filepath.Join(home, ".bash_login"), filepath.Join(home, ".profile"), filepath.Join(home, ".bashrc")}
	default:
		return nil
	}
	var out []string
	for _, f := range files {
		if !exists(f) {
			continue
		}
		out = append(out, f)
		for _, s := range sourcedBy(f) {
			if exists(s) && !slices.Contains(out, s) {
				out = append(out, s)
			}
		}
	}
	return out
}

// sourceWord is a source or a . followed by a path, wherever it stands
// on the line: after a test that the file is there, most often.
var sourceWord = regexp.MustCompile(`(?:^|[;&|(]|\s)(?:source|\.)\s+["']?([^"'\s;&|)]+)`)

// sourcedBy is the files a startup file sources, as far as they can be
// read off the line: a path, with ~ and $HOME expanded; a path built from
// another variable is not followed.
func sourcedBy(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	home, _ := os.UserHomeDir()
	var out []string
	for _, m := range sourceWord.FindAllStringSubmatch(string(b), -1) {
		p := m[1]
		p = strings.ReplaceAll(p, "$HOME", home)
		p = strings.ReplaceAll(p, "${HOME}", home)
		p = expandPath(p)
		if strings.Contains(p, "$") || !filepath.IsAbs(p) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// namedIn is the variables a startup file assigns by the look of it —
// NAME= at the start of a line, an export or typeset allowed first —
// each with the file and line: the fallback for what the trace did not
// see run.
func namedIn(path string) map[string]string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	out := map[string]string{}
	for i, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		for _, name := range assignedIn(line) {
			if _, seen := out[name]; !seen {
				out[name] = homely(path) + ":" + strconv.Itoa(i+1)
			}
		}
	}
	return out
}

// systemPaths is what the system puts on PATH before any shell file
// runs: each entry of etc/paths and of the files under etc/paths.d,
// with the file it is in — path_helper's work, on macOS.
func systemPaths(etc string) map[string]string {
	out := map[string]string{}
	files := []string{filepath.Join(etc, "paths")}
	if more, err := filepath.Glob(filepath.Join(etc, "paths.d", "*")); err == nil {
		files = append(files, more...)
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for line := range strings.SplitSeq(string(b), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				if _, seen := out[line]; !seen {
					out[line] = f
				}
			}
		}
	}
	return out
}

// withOrigins is the rows with the trace's word in them: a variable the
// shell or the terminal's shell set says the file and line, or that it is
// named in a file when only the grep found it; a PATH entry the system
// put there says which file.
func withOrigins(rows []envRow, t envTraceMsg) []envRow {
	out := make([]envRow, len(rows))
	for i, r := range rows {
		switch {
		case r.kind == envVarRow && (r.source == "the shell" || r.source == "the terminal"):
			if o, ok := t.origins[r.name]; ok {
				r.source = o
			} else if n, ok := t.named[r.name]; ok {
				r.source = "named in " + n
			}
		case r.kind == envEntryRow && r.under == "PATH" && r.name == "" && r.note == "":
			if f, ok := t.paths[expandPath(r.value)]; ok {
				r.note, r.noteTone = f, toneQuiet
			}
		}
		out[i] = r
	}
	return out
}
