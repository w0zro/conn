package main

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

// What a process is, past its name: the binary it runs, which node and
// which python, and the environment it was started with, which is what
// says whether the server is in development or production and which
// database it is talking to. Both are read once, for the one process
// being inspected; both are the process's own business, so only the
// user's own processes answer.

// binaryOf is the executable a process runs, or nothing when it cannot be
// read: the exe link under /proc on Linux, and elsewhere what lsof lists
// as the process's text.
func binaryOf(pid int) string {
	if runtime.GOOS == "linux" {
		exe, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/exe")
		if err != nil {
			return ""
		}
		return exe
	}
	out, err := listing(scanTimeout, "lsof", "-p", strconv.Itoa(pid), "-a", "-d", "txt", "-Fn")
	if err != nil && len(out) == 0 {
		return ""
	}
	for line := range strings.SplitSeq(string(out), "\n") {
		if strings.HasPrefix(line, "n/") {
			return line[1:]
		}
	}
	return ""
}

// environOf is the environment a process was started with, name=value
// each, or nothing when it cannot be read: environ under /proc on Linux,
// and elsewhere what ps -E prints after the command line. macOS keeps
// the environment of its own binaries to itself; a dev server is not
// one of those.
func environOf(pid int) []string {
	if runtime.GOOS == "linux" {
		raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/environ")
		if err != nil {
			return nil
		}
		return strings.FieldsFunc(string(raw), func(r rune) bool { return r == 0 })
	}
	argv, err := ps(pid, "command=")
	if err != nil {
		return nil
	}
	both, err := listing(scanTimeout, "ps", "-E", "-p", strconv.Itoa(pid), "-o", "command=")
	if err != nil {
		return nil
	}
	return parseEnviron(strings.TrimSpace(string(both)), argv)
}

// parseEnviron reads the environment out of what ps -E prints: the
// command line, then the environment, one space between everything. The
// command line is taken off the front, known from ps without -E; what is
// left is variables, a value running on until the next name.
func parseEnviron(both, argv string) []string {
	rest := strings.TrimSpace(strings.TrimPrefix(both, argv))
	if rest == "" {
		return nil
	}
	var env []string
	for tok := range strings.SplitSeq(rest, " ") {
		if envName.MatchString(tok) || len(env) == 0 {
			env = append(env, tok)
			continue
		}
		env[len(env)-1] += " " + tok
	}
	return env
}

// envName is the shape of a variable's start: a name, and the sign.
var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// tellingEnv picks, out of an environment, the variables that say which
// environment a process runs in and what it talks to: the *_ENV and
// settings-module family, the port and host it was given, the logging
// level, and every URL, address and DSN. A secret is never one of them —
// a name that says key, token, secret or password is left out — and the
// password in a URL is masked. Sorted by name, and at most envShown.
func tellingEnv(env []string) []string {
	var out []string
	for _, kv := range env {
		name, value, ok := strings.Cut(kv, "=")
		if !ok || !tellingName(name) || secretName(name) {
			continue
		}
		out = append(out, name+"="+maskURL(value))
	}
	slices.Sort(out)
	if len(out) > envShown {
		out = out[:envShown]
	}
	return out
}

// envShown is how many variables the pane lists, at most.
const envShown = 8

// tellingName reports a variable worth the pane's line.
func tellingName(name string) bool {
	upper := strings.ToUpper(name)
	switch upper {
	case "ENV", "ENVIRONMENT", "PORT", "HOST", "HOSTNAME", "DEBUG", "LOG_LEVEL", "RUST_LOG",
		"VIRTUAL_ENV", "CONDA_DEFAULT_ENV", "DJANGO_SETTINGS_MODULE", "NODE_OPTIONS", "GOFLAGS":
		return true
	}
	for _, suffix := range []string{"_ENV", "_URL", "_URI", "_DSN", "_HOST", "_PORT", "_ADDR", "_ADDRESS"} {
		if strings.HasSuffix(upper, suffix) {
			return true
		}
	}
	return false
}

// secretName reports a variable whose value is a secret by its name.
func secretName(name string) bool {
	upper := strings.ToUpper(name)
	for _, word := range []string{"SECRET", "TOKEN", "KEY", "PASSWORD", "PASSWD", "PASS", "CREDENTIAL", "AUTH"} {
		if strings.Contains(upper, word) {
			return true
		}
	}
	return false
}

// urlPassword is the password in a URL's user part.
var urlPassword = regexp.MustCompile(`(://[^/:@\s]*):[^@/\s]*@`)

// maskURL is a value with the password of any URL in it masked.
func maskURL(value string) string {
	return urlPassword.ReplaceAllString(value, "$1:•••@")
}

// homely is a path with the home directory as ~, for a pane that has a
// column to say it in.
func homely(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || !under(path, home) {
		return path
	}
	return filepath.Join("~", strings.TrimPrefix(path, home))
}
