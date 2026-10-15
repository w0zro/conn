package theme

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The theme is a theme file Claude Code can read: its own shape, on the
// ansi base of its ground so what conn says nothing about falls through
// to the slot the pane already has, and no token written twice.
func TestTheClaudeThemeIsATheme(t *testing.T) {
	for _, g := range []Ground{Dark, Light} {
		var got struct {
			Name      string            `json:"name"`
			Base      string            `json:"base"`
			Overrides map[string]string `json:"overrides"`
		}
		out := claudeThemeJSON(g)
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("not a theme file: %v\n%s", err, out)
		}
		if got.Name != "Conn" || got.Base != g.claudeBase() || strings.HasPrefix(got.Base, "dark") != g.Dark() {
			t.Errorf("named %q on %q", got.Name, got.Base)
		}
		seen := map[string]bool{}
		n := 0
		for _, grp := range ClaudeTheme(g) {
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
		// The file is grouped the way the handoff groups it, so it can
		// be read against it; the groups are what the blank lines
		// separate.
		if groups := strings.Count(out, "\n\n"); groups != len(ClaudeTheme(g))-1 {
			t.Errorf("%d blank lines between %d groups", groups, len(ClaudeTheme(g)))
		}
	}
}

// Every color is one conn draws: a slot of the scheme, a role, a
// reference to a slot, or one of the grounds no slot has a name for.
// Nothing is a color from somewhere else.
func TestTheClaudeThemeIsDrawnFromConnsOwn(t *testing.T) {
	for _, g := range []Ground{Dark, Light} {
		known := map[string]bool{Hex(g.Ground): true, Hex(g.Ink): true}
		for _, c := range g.Scheme {
			known[c] = true
		}
		for _, c := range g.Roles() {
			known[c] = true
		}
		for _, grp := range ClaudeTheme(g) {
			for _, tk := range grp {
				if !strings.HasPrefix(tk.Color, "ansi:") && !known[tk.Color] {
					t.Errorf("%s is %s, which is no color of conn's", tk.Name, tk.Color)
				}
			}
		}
	}
}

// A verdict is slot-shaped, so it is written as the slot and follows
// the pane's own sixteen. The accent is "you, here": the dialog that
// stops and waits, the meter, and the > before what you said; never a
// mode.
func TestTheClaudeThemeSpendsItsColorsWhereItSays(t *testing.T) {
	at := map[string]string{}
	for _, grp := range ClaudeTheme(Dark) {
		for _, tk := range grp {
			at[tk.Name] = tk.Color
		}
	}
	for _, k := range []string{"success", "error", "warning", "merged"} {
		if !strings.HasPrefix(at[k], "ansi:") {
			t.Errorf("%s is %s, not a slot", k, at[k])
		}
	}
	for _, k := range []string{"permission", "rate_limit_fill", "subtle"} {
		if at[k] != Dark.Accent {
			t.Errorf("%s is %s, not the accent", k, at[k])
		}
	}
	// The mark turns while Claude works, so it is drawn at work: the
	// accent in conn's own, a hue of its own where a theme has one.
	if at["claude"] != Dark.AtWork || at["claudeShimmer"] != Dark.AtWorkShimmer {
		t.Errorf("the mark is %s and %s, not at work", at["claude"], at["claudeShimmer"])
	}
	for _, mode := range []string{"planMode", "autoAccept", "fastMode", "bashBorder", "promptBorder"} {
		if at[mode] == Dark.Accent {
			t.Errorf("%s takes the accent", mode)
		}
	}
}

// RefreshClaudeTheme keeps the file on the ground the server is on: it
// rewrites what conn theme claude already wrote, and writes nothing
// where that command has never run.
func TestRefreshClaudeThemeKeepsTheFileCurrent(t *testing.T) {
	home := t.TempDir()
	RefreshClaudeTheme(home, Dark)
	if _, err := os.Stat(claudeThemePath(home)); err == nil {
		t.Error("RefreshClaudeTheme wrote a file conn theme claude never had")
	}
	if _, err := WriteClaudeTheme(home, Dark); err != nil {
		t.Fatal(err)
	}
	RefreshClaudeTheme(home, Light)
	b, err := os.ReadFile(claudeThemePath(home))
	if err != nil || !strings.Contains(string(b), `"base": "light-ansi"`) {
		t.Errorf("the file was not refreshed to light: %v\n%s", err, b)
	}
}

// UseClaudeTheme changes the one value and nothing else in the file,
// and will not guess between two.
func TestUseClaudeTheme(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{\n  \"model\": \"opus\",\n  \"theme\" : \"dark\",\n  \"x\": 1\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if in, ok := ThemeInUse(home); !ok || in != "dark" {
		t.Errorf("ThemeInUse = %q, %v", in, ok)
	}
	if err := UseClaudeTheme(home); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "{\n  \"model\": \"opus\",\n  \"theme\": \"custom:conn\",\n  \"x\": 1\n}\n" {
		t.Errorf("settings after UseClaudeTheme:\n%s", b)
	}
	if err := os.WriteFile(path, []byte(`{"theme": "dark", "env": {"theme": "x"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := UseClaudeTheme(home); err == nil {
		t.Error("two theme keys were not refused")
	}
}
