package theme

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
	out := claudeThemeJSON(Conn.Dark)
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not a theme file: %v\n%s", err, out)
	}
	if got.Name != "Conn" || got.Base != "dark-ansi" {
		t.Errorf("named %q on %q", got.Name, got.Base)
	}
	seen := map[string]bool{}
	n := 0
	for _, grp := range ClaudeTheme(Conn.Dark) {
		for _, tk := range grp {
			if seen[tk.Name] {
				t.Errorf("%s is written twice", tk.Name)
			}
			seen[tk.Name] = true
			n++
		}
	}
	if len(got.Overrides) != n {
		t.Errorf("%d tokens written, %d read back", n, len(got.Overrides))
	}
	// The file is grouped the way the handoff groups it, so it can be
	// read against it; the groups are what the blank lines separate.
	if groups := strings.Count(out, "\n\n"); groups != len(ClaudeTheme(Conn.Dark))-1 {
		t.Errorf("%d blank lines between %d groups", groups, len(ClaudeTheme(Conn.Dark)))
	}
}

// Every color is one conn draws: a slot of the scheme, one of the
// console's own, a reference to a slot, or one of the grounds no slot
// has a name for. Nothing is a color from somewhere else.
func TestTheThemeIsDrawnFromConnsOwn(t *testing.T) {
	g := Conn.Dark
	known := map[string]bool{
		Hex(g.Ground): true, Hex(g.Ink): true, g.Gray: true, g.Border: true,
		g.Accent: true, g.Shimmer: true, g.Parchment: true, g.MessageBg: true,
		g.DiffAddedBg: true, g.DiffRemovedBg: true, g.DiffAddedDim: true, g.DiffRemovedDim: true,
		g.DiffAddedWord: true, g.DiffRemovedWord: true,
		g.MessageHoverBg: true, g.ToolBg: true,
	}
	for _, c := range g.Scheme {
		known[c] = true
	}
	for _, grp := range ClaudeTheme(g) {
		for _, tk := range grp {
			if strings.HasPrefix(tk.Color, "ansi:") {
				continue
			}
			if !known[tk.Color] {
				t.Errorf("%s is %s, which is no color of conn's", tk.Name, tk.Color)
			}
		}
	}
}

// A verdict is slot-shaped, so it is written as the slot and follows
// the pane's own sixteen. The orange is "you, here": the mark, the
// dialog that stops and waits, and the meter — never a mode.
func TestTheThemeSpendsItsColorsWhereItSays(t *testing.T) {
	at := map[string]string{}
	for _, grp := range ClaudeTheme(Conn.Dark) {
		for _, tk := range grp {
			at[tk.Name] = tk.Color
		}
	}
	for _, k := range []string{"success", "error", "warning", "merged"} {
		if !strings.HasPrefix(at[k], "ansi:") {
			t.Errorf("%s is %s, not a slot", k, at[k])
		}
	}
	accent := Conn.Dark.Accent
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

// RefreshClaudeTheme keeps the file on the mode the server is on: it
// rewrites what conn theme claude already wrote, and writes nothing
// where that command has never run.
func TestRefreshClaudeThemeKeepsTheFileCurrent(t *testing.T) {
	home := t.TempDir()

	// Never written: refreshing writes nothing.
	RefreshClaudeTheme(home, Conn.Dark)
	if _, err := os.Stat(filepath.Join(home, ".claude", "themes", "conn.json")); err == nil {
		t.Error("RefreshClaudeTheme wrote a file conn theme claude never had")
	}

	// Written once, on dark; the server moves to light; a refresh
	// catches the file up without being asked again.
	if _, err := WriteClaudeTheme(home, Conn.Dark); err != nil {
		t.Fatal(err)
	}
	RefreshClaudeTheme(home, Conn.Light)
	b, err := os.ReadFile(filepath.Join(home, ".claude", "themes", "conn.json"))
	if err != nil || !strings.Contains(string(b), `"base": "light-ansi"`) {
		t.Errorf("the file was not refreshed to light: %v\n%s", err, b)
	}
}
