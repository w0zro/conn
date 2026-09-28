package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// conn writes the theme where Claude Code looks, says so, and offers to
// point Claude Code at it only when nothing of the user's own is in the
// way.
func TestConnWritesTheThemeAndOffersOnce(t *testing.T) {
	yes := func(string) bool { return true }
	write := func(t *testing.T, settings string) string {
		t.Helper()
		home := t.TempDir()
		if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
			t.Fatal(err)
		}
		if settings != "" {
			if err := os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte(settings), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return home
	}

	// On a built-in theme, conn offers, and the answer is taken.
	home := write(t, `{
  "model": "opus[1m]",
  "theme": "dark",
  "autoMode": true
}
`)
	msg, ok := dressProgram([]string{"claude"}, home, yes)
	if !ok || !strings.Contains(msg, "themes/conn.json") {
		t.Errorf("the theme was not written: %q", msg)
	}
	if b, err := os.ReadFile(filepath.Join(home, ".claude", "themes", "conn.json")); err != nil || !strings.Contains(string(b), `"base": "dark-ansi"`) {
		t.Errorf("the file on disk: %v", err)
	}
	after, _ := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if !strings.Contains(string(after), `"theme": "custom:conn"`) {
		t.Errorf("the theme was not taken up:\n%s", after)
	}
	// Everything else in the file is the user's still, in their order.
	if !strings.HasPrefix(string(after), "{\n  \"model\": \"opus[1m]\",\n") || !strings.Contains(string(after), `"autoMode": true`) {
		t.Errorf("the settings file was rewritten:\n%s", after)
	}

	// On somebody's own custom theme, conn says what it is and leaves it.
	home = write(t, `{"theme": "custom:datum-dark"}`)
	msg, ok = dressProgram([]string{"claude"}, home, yes)
	after, _ = os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if !ok || !strings.Contains(msg, "custom:datum-dark") || strings.Contains(string(after), "custom:conn") {
		t.Errorf("conn changed a theme of the user's own: %q\n%s", msg, after)
	}

	// Already on it: conn writes the file and says nothing more of it.
	home = write(t, `{"theme": "custom:conn"}`)
	msg, ok = dressProgram([]string{"claude"}, home, yes)
	if !ok || strings.Count(msg, "\n") != 1 || !strings.HasPrefix(msg, "Wrote conn's theme") {
		t.Errorf("conn said something about a theme already in use: %q", msg)
	}

	// With nobody to ask, conn writes the file and says how to pick it.
	home = write(t, `{"theme": "dark"}`)
	msg, ok = dressProgram([]string{"claude"}, home, nil)
	after, _ = os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if !ok || !strings.Contains(msg, "/theme") || strings.Contains(string(after), "custom:conn") {
		t.Errorf("conn set the theme with nobody to ask: %q", msg)
	}

	// A settings file that names the theme in more than one project is not
	// conn's to edit by guessing which.
	home = write(t, `{"theme": "dark", "somethingElse": {"theme": "of its own"}}`)
	if err := useClaudeTheme(home); err == nil {
		t.Error("conn guessed which theme to rewrite")
	}
}

// conn dresses the programs it has a theme for, and says so for any
// other.
func TestConnDressesWhatItHasAThemeFor(t *testing.T) {
	home := t.TempDir()
	for _, args := range [][]string{{}, {"emacs"}, {"claude", "dark"}} {
		if _, ok := dressProgram(args, home, nil); ok {
			t.Errorf("conn theme %v was taken", args)
		}
	}
}

// conn writes the colorscheme where nvim looks for one, and says so.
func TestConnWritesTheVimColorscheme(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	msg, ok := dressProgram([]string{"vim"}, home, nil)
	if !ok || !strings.Contains(msg, "nvim/colors/conn.vim") {
		t.Fatalf("not written: %q", msg)
	}
	b, err := os.ReadFile(filepath.Join(home, ".config", "nvim", "colors", "conn.vim"))
	if err != nil || !strings.Contains(string(b), "let g:colors_name = 'conn'") {
		t.Errorf("the file on disk: %v", err)
	}
	// nvim is the same program by either name.
	if _, ok := dressProgram([]string{"nvim"}, home, nil); !ok {
		t.Error("conn theme nvim was not taken")
	}
	// XDG_CONFIG_HOME is where it goes when it is set.
	other := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", other)
	if _, ok := dressProgram([]string{"vim"}, home, nil); !ok {
		t.Fatal("not written under XDG_CONFIG_HOME")
	}
	if _, err := os.Stat(filepath.Join(other, "nvim", "colors", "conn.vim")); err != nil {
		t.Errorf("XDG_CONFIG_HOME was not used: %v", err)
	}
}
