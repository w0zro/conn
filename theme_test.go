package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The theme is a theme file Claude Code can read: its own shape, on the
// ansi base so what conn says nothing about falls through to the slot
// the pane already has, and no token written twice.
func TestTheClaudeThemeIsATheme(t *testing.T) {
	var got struct {
		Name      string            `json:"name"`
		Base      string            `json:"base"`
		Overrides map[string]string `json:"overrides"`
	}
	out := claudeThemeJSON()
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not a theme file: %v\n%s", err, out)
	}
	if got.Name != "Conn" || got.Base != "dark-ansi" {
		t.Errorf("named %q on %q", got.Name, got.Base)
	}
	seen := map[string]bool{}
	n := 0
	for _, g := range claudeTheme() {
		for _, tk := range g {
			if seen[tk.name] {
				t.Errorf("%s is written twice", tk.name)
			}
			seen[tk.name] = true
			n++
		}
	}
	if len(got.Overrides) != n {
		t.Errorf("%d tokens written, %d read back", n, len(got.Overrides))
	}
	// The file is grouped the way the handoff groups it, so it can be
	// read against it; the groups are what the blank lines separate.
	if groups := strings.Count(out, "\n\n"); groups != len(claudeTheme())-1 {
		t.Errorf("%d blank lines between %d groups", groups, len(claudeTheme()))
	}
}

// Every color is one conn draws: a slot of the scheme, one of the
// console's own, a reference to a slot, or one of the grounds no slot
// has a name for. Nothing is a color from somewhere else.
func TestTheThemeIsDrawnFromConnsOwn(t *testing.T) {
	known := map[string]bool{
		hex(groundColor): true, hex(inkColor): true, grayHex: true, borderHex: true,
		cursorHex: true, shimmerHex: true, parchmentHex: true, messageBg: true,
		diffAddedBg: true, diffRemovedBg: true, diffAddedDim: true, diffRemovedDim: true,
		diffAddedWord: true, diffRemovedWord: true,
		messageHoverBg: true, toolBg: true,
	}
	for _, c := range scheme {
		known[c] = true
	}
	for _, g := range claudeTheme() {
		for _, tk := range g {
			if strings.HasPrefix(tk.color, "ansi:") {
				continue
			}
			if !known[tk.color] {
				t.Errorf("%s is %s, which is no color of conn's", tk.name, tk.color)
			}
		}
	}
}

// A verdict is slot-shaped, so it is written as the slot and follows
// the pane's own sixteen. The orange is "you, here": the mark, the
// dialog that stops and waits, and the meter — never a mode.
func TestTheThemeSpendsItsColorsWhereItSays(t *testing.T) {
	at := map[string]string{}
	for _, g := range claudeTheme() {
		for _, tk := range g {
			at[tk.name] = tk.color
		}
	}
	for _, k := range []string{"success", "error", "warning", "merged"} {
		if !strings.HasPrefix(at[k], "ansi:") {
			t.Errorf("%s is %s, not a slot", k, at[k])
		}
	}
	accent := cursorHex
	for _, k := range []string{"claude", "permission", "rate_limit_fill"} {
		if at[k] != accent {
			t.Errorf("%s is %s, not the accent", k, at[k])
		}
	}
	for _, mode := range []string{"planMode", "autoAccept", "fastMode", "bashBorder", "promptBorder"} {
		if at[mode] == accent {
			t.Errorf("%s takes the accent", mode)
		}
	}
}

// conn writes the theme where Claude Code looks, says so, and offers to
// point Claude Code at it only when nothing of the user's own is in the
// way.
func TestConnWritesTheThemeAndOffersOnce(t *testing.T) {
	holdMode(t) // dressProgram puts conn on the ground its server is on
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

// refreshClaudeTheme keeps the file on the mode the server is on: it
// rewrites what conn theme claude already wrote, and writes nothing
// where that command has never run.
func TestRefreshClaudeThemeKeepsTheFileCurrent(t *testing.T) {
	home := t.TempDir()

	// Never written: refreshing writes nothing.
	refreshClaudeTheme(home)
	if _, err := os.Stat(filepath.Join(home, ".claude", "themes", "conn.json")); err == nil {
		t.Error("refreshClaudeTheme wrote a file conn theme claude never had")
	}

	// Written once, on dark; the server moves to light; a refresh
	// catches the file up without being asked again.
	if _, err := writeClaudeTheme(home); err != nil {
		t.Fatal(err)
	}
	holdMode(t)
	applyMode(connOn(false))
	refreshClaudeTheme(home)
	b, err := os.ReadFile(filepath.Join(home, ".claude", "themes", "conn.json"))
	if err != nil || !strings.Contains(string(b), `"base": "light-ansi"`) {
		t.Errorf("the file was not refreshed to light: %v\n%s", err, b)
	}
}

// conn dresses the programs it has a theme for, and says so for any
// other.
func TestConnDressesWhatItHasAThemeFor(t *testing.T) {
	holdMode(t) // dressProgram puts conn on the ground its server is on
	home := t.TempDir()
	for _, args := range [][]string{{}, {"emacs"}, {"claude", "dark"}} {
		if _, ok := dressProgram(args, home, nil); ok {
			t.Errorf("conn theme %v was taken", args)
		}
	}
}
