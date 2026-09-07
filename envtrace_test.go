package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestATraceSaysWhichFileAndLineSetEachVariable(t *testing.T) {
	t.Setenv("HOME", "/Users/me")
	trace := strings.Join([]string{
		"+/etc/zshrc:5> export PAGER=less",
		"+/Users/me/.zprofile:3> eval '$(/opt/homebrew/bin/brew shellenv)'",
		"+(eval):1> export HOMEBREW_PREFIX=/opt/homebrew",
		"+(eval):4> export PATH=/opt/homebrew/bin:/usr/bin",
		"+/Users/me/.zshrc:12> [ -f /Users/me/.zshrc.local ]",
		"+/Users/me/.zshrc:12> source /Users/me/.zshrc.local",
		"+/Users/me/.zshrc.local:5> export GITHUB_TOKEN=ghp_xxx",
		"+/Users/me/.zshrc.local:6> typeset -gx EDITOR=nvim VISUAL=nvim",
		"+/Users/me/.zshrc:20> path+=( /Users/me/bin )",
		"+/Users/me/.zshrc:21> git config core.editor=nvim",
		"++/Users/me/.zshrc:22> LOCAL_ONLY=1",
		"+myfunc:2> export FROM_FUNC=1",
		"a line that is not a trace",
	}, "\n")
	got := parseTrace(trace)
	for name, want := range map[string]string{
		"PAGER":           "/etc/zshrc:5",
		"HOMEBREW_PREFIX": "~/.zprofile:3", // the eval's own line
		"PATH":            "~/.zshrc:20",   // the last to set it: path+=
		"GITHUB_TOKEN":    "~/.zshrc.local:5",
		"EDITOR":          "~/.zshrc.local:6",
		"VISUAL":          "~/.zshrc.local:6",
		"LOCAL_ONLY":      "~/.zshrc:22",
		"FROM_FUNC":       "~/.zshrc:22", // the last file line seen stands in for the function
	} {
		if got[name] != want {
			t.Errorf("%s set at %q, want %q", name, got[name], want)
		}
	}
	if _, found := got["core.editor"]; found || len(got) != 8 {
		t.Errorf("origins = %v, want the eight assignments and no argument taken for one", got)
	}
}

func TestAssignmentsAreReadOffACommand(t *testing.T) {
	for command, want := range map[string]string{
		"export A=1 B=2":            "A B",
		"typeset -gx C=3":           "C",
		"declare -x D=4":            "D",
		"E=5":                       "E",
		"F=6 G=7 some-command":      "F G",
		"path=( /usr/bin )":         "PATH",
		"manpath+=(/usr/share/man)": "MANPATH",
		"fpath+=(/usr/share/zsh)":   "",
		"git config a=b":            "",
		"export":                    "",
		"":                          "",
	} {
		if got := strings.Join(assignedIn(command), " "); got != want {
			t.Errorf("assignedIn(%q) = %q, want %q", command, got, want)
		}
	}
}

func TestStartupFilesAreGreppedAndFollowed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ZDOTDIR", "")
	local := filepath.Join(home, ".zshrc.local")
	files := map[string]string{
		".zshenv":      "export EDITOR=vim\n",
		".zshrc":       "# GITHUB_TOKEN=commented\n[ -f ~/.zshrc.local ] && source ~/.zshrc.local\n. \"$HOME/.aliases\"\nexport PAGER=less\n",
		".zshrc.local": "export GITHUB_TOKEN=ghp_xxx\n",
		".aliases":     "alias ll='ls -l'\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(home, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got := startupFiles("/bin/zsh")
	var mine []string
	for _, f := range got {
		if strings.HasPrefix(f, home) {
			mine = append(mine, filepath.Base(f))
		}
	}
	if strings.Join(mine, " ") != ".zshenv .zshrc .zshrc.local .aliases" {
		t.Errorf("startupFiles = %v, want the user's files and what .zshrc sources, in order", mine)
	}
	named := namedIn(local)
	if named["GITHUB_TOKEN"] != "~/.zshrc.local:1" {
		t.Errorf("namedIn = %v", named)
	}
	if named := namedIn(filepath.Join(home, ".zshrc")); named["PAGER"] != "~/.zshrc:4" || named["GITHUB_TOKEN"] != "" {
		t.Errorf("namedIn(.zshrc) = %v, want PAGER on line 4 and the comment passed over", named)
	}
	if startupFiles("/usr/bin/fish") != nil {
		t.Error("fish has files, but not ones this reads")
	}
}

func TestSystemPathsNameTheirFiles(t *testing.T) {
	etc := t.TempDir()
	if err := os.WriteFile(filepath.Join(etc, "paths"), []byte("/usr/local/bin\n/usr/bin\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(etc, "paths.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(etc, "paths.d", "go"), []byte("/usr/local/go/bin\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := systemPaths(etc)
	if got["/usr/bin"] != filepath.Join(etc, "paths") || got["/usr/local/go/bin"] != filepath.Join(etc, "paths.d", "go") || len(got) != 3 {
		t.Errorf("systemPaths = %v", got)
	}
}

func TestTheTracesWordGoesIntoTheSourceColumn(t *testing.T) {
	rows := []envRow{
		{kind: envVarRow, name: "GITHUB_TOKEN", source: "the shell"},
		{kind: envVarRow, name: "HOME", source: "the terminal"},
		{kind: envVarRow, name: "GREETING", source: "the shell"},
		{kind: envVarRow, name: "npm_config_x", source: "npm run dev"},
		{kind: envVarRow, name: "PATH", source: "the shell", folds: true},
		{kind: envEntryRow, under: "PATH", value: "/usr/bin"},
		{kind: envEntryRow, under: "PATH", value: "~/bin", note: "missing", noteTone: toneBad},
	}
	got := withOrigins(rows, envTraceMsg{
		origins: map[string]string{"GITHUB_TOKEN": "~/.zshrc.local:5", "npm_config_x": "~/.zshrc:1"},
		named:   map[string]string{"GREETING": "~/.zshrc:9", "HOME": "~/.zshrc:2"},
		paths:   map[string]string{"/usr/bin": "/etc/paths", "/Users/me/bin": "/etc/paths.d/me"},
	})
	for i, want := range []string{"~/.zshrc.local:5", "named in ~/.zshrc:2", "named in ~/.zshrc:9", "npm run dev", "the shell"} {
		if got[i].source != want {
			t.Errorf("%s's source = %q, want %q", got[i].name, got[i].source, want)
		}
	}
	if got[5].note != "/etc/paths" || got[6].note != "missing" {
		t.Errorf("entries = %q, %q; want the system's file on the one, and the missing left as it was", got[5].note, got[6].note)
	}
}

// TestAShellIsTracedForReal runs the shells this machine has, each with
// startup files of the test's own, and reads the assignments back.
func TestAShellIsTracedForReal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ZDOTDIR", home)
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("# a comment\nexport CONN_TRACED=yes\nif [ -n \"$CONN_NEVER\" ]; then export CONN_SKIPPED=1; fi\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".bash_profile"), []byte("export CONN_TRACED=yes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for shell, want := range map[string]string{"zsh": "~/.zshrc:2", "bash": "~/.bash_profile:1"} {
		path, err := exec.LookPath(shell)
		if err != nil {
			t.Logf("no %s here", shell)
			continue
		}
		got := traceShell(path)
		if got["CONN_TRACED"] != want {
			t.Errorf("%s: CONN_TRACED set at %q, want %q (trace: %v)", shell, got["CONN_TRACED"], want, got)
		}
		if _, found := got["CONN_SKIPPED"]; found {
			t.Errorf("%s: a branch not taken was read as run", shell)
		}
	}
	if got := traceShell("/usr/bin/fish"); got != nil {
		t.Errorf("fish traced: %v", got)
	}
}
