package theme

import (
	"encoding/json"
	"image/color"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Every color of either ground parses, the dark ground is dark and
// the light one light, and a slot and a role are written alike.
func TestGrounds(t *testing.T) {
	for name, g := range map[string]Ground{"dark": Dark, "light": Light} {
		if (name == "dark") != g.Dark() {
			t.Errorf("%s ground reads as dark = %v", name, g.Dark())
		}
		for i, s := range g.Scheme {
			RGB(s)
			if Hex(RGB(s)) != s {
				t.Errorf("%s slot %d %s is not written as read", name, i, s)
			}
		}
		for role, h := range g.Roles() {
			if h == "" {
				t.Errorf("%s has no %s", name, role)
				continue
			}
			RGB(h)
		}
	}
	if !IsDark(color.RGBA{A: 255}) || IsDark(color.RGBA{R: 255, G: 255, B: 255, A: 255}) {
		t.Error("black is dark and white is light")
	}
}

// The mode a server came up on is read back as written, and a file
// from a build that named no theme wears conn's.
func TestMode(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "s")
	if _, ok := ReadMode(socket); ok {
		t.Error("a mode was read before one was written")
	}
	if err := WriteMode(socket, Mode{Dark: false, Theme: "mine"}); err != nil {
		t.Fatal(err)
	}
	if m, ok := ReadMode(socket); !ok || m != (Mode{Dark: false, Theme: "mine"}) {
		t.Errorf("ReadMode = %+v, %v", m, ok)
	}
	if err := os.WriteFile(ModePath(socket), []byte("dark\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if m, ok := ReadMode(socket); !ok || m != (Mode{Dark: true, Theme: "conn"}) {
		t.Errorf("ReadMode of a one-word file = %+v, %v", m, ok)
	}
	ForgetMode(socket)
	if _, ok := ReadMode(socket); ok {
		t.Error("a mode was read after it was forgotten")
	}
}

// --light, --dark and --theme are read off the front of the arguments
// and no further, and --dark --light is a contradiction.
func TestParseFlags(t *testing.T) {
	rest, want, err := ParseFlags([]string{"ls", "--light"})
	if err != nil || want.Any() || len(rest) != 2 {
		t.Errorf("a flag after the command is the command's: %v %v %v", rest, want, err)
	}
	rest, want, err = ParseFlags([]string{"--light", "--theme", "mine", "--light", "u", "--dark"})
	if err != nil || want.Dark == nil || *want.Dark || want.Theme != "mine" || len(rest) != 2 || rest[0] != "u" {
		t.Errorf("--light, a theme, --light, then a command: %v %+v %v", rest, want, err)
	}
	rest, want, err = ParseFlags([]string{"--theme=mine"})
	if err != nil || want.Dark != nil || want.Theme != "mine" || rest != nil {
		t.Errorf("--theme=mine alone: %v %+v %v", rest, want, err)
	}
	if _, _, err := ParseFlags([]string{"--theme"}); err == nil {
		t.Error("--theme with no name was taken")
	}
	if _, _, err := ParseFlags([]string{"--dark", "--light"}); err == nil {
		t.Error("--dark --light was not a contradiction")
	}
}

// A theme is a sheet in the directory by its name, and conn's own is
// there unless a sheet stands in for it; a name that is not there says
// what is.
func TestLoad(t *testing.T) {
	dir := t.TempDir()
	if th, err := Load(dir, "conn"); err != nil || th.Dark != Dark {
		t.Errorf("conn's own from an empty directory: %v", err)
	}
	var whole map[string]any
	_ = json.Unmarshal(connSheet(), &whole)
	b, _ := json.Marshal(map[string]any{"light": whole["light"]})
	if err := os.WriteFile(filepath.Join(dir, "paper.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	if th, err := Load(dir, "paper"); err != nil || th.Name != "paper" || th.Dark != Light {
		t.Errorf("paper: %+v, %v", th, err)
	}
	if _, err := Load(dir, "nope"); err == nil || !strings.Contains(err.Error(), "there are conn, datum, paper, skelly") {
		t.Errorf("a missing theme: %v", err)
	}
	for _, name := range []string{"", "../paper", ".hidden"} {
		if _, err := Load(dir, name); err == nil {
			t.Errorf("%q was loaded", name)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "conn.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	if th, err := Load(dir, "conn"); err != nil || th.Dark != Light {
		t.Errorf("a sheet named conn does not stand in for conn's: %v", err)
	}
}

// A sheet with one ground wears it on both, writes every color as Hex
// does, and says by name what is missing, misspelt or not a color.
func TestParse(t *testing.T) {
	sheet := func(edit func(g map[string]any)) []byte {
		var whole map[string]map[string]any
		if err := json.Unmarshal(connSheet(), &whole); err != nil {
			t.Fatal(err)
		}
		edit(whole["dark"])
		b, _ := json.Marshal(map[string]any{"dark": whole["dark"]})
		return b
	}
	th, err := Parse("mine", sheet(func(g map[string]any) { g["accent"] = "#e85d2f" }))
	if err != nil {
		t.Fatal(err)
	}
	if th.Name != "mine" || th.Light != th.Dark || th.Dark != Dark {
		t.Errorf("one dark ground is not worn on both, as conn's: %+v", th)
	}
	for want, edit := range map[string]func(g map[string]any){
		"no accent":            func(g map[string]any) { delete(g, "accent") },
		"accnet is no role":    func(g map[string]any) { g["accnet"] = "#000000" },
		`"orange" is not`:      func(g map[string]any) { g["faint"] = "orange" },
		`"#FFF" is not`:        func(g map[string]any) { g["ink"] = "#FFF" },
		"scheme has 15 colors": func(g map[string]any) { g["scheme"] = g["scheme"].([]any)[:15] },
		"scheme 3":             func(g map[string]any) { g["scheme"].([]any)[3] = "yellow" },
	} {
		if _, err := Parse("mine", sheet(edit)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want an error saying %q, got %v", want, err)
		}
	}
	for _, b := range []string{`{}`, `{"dusk": {}}`, `[]`} {
		if _, err := Parse("mine", []byte(b)); err == nil {
			t.Errorf("%s was read as a theme", b)
		}
	}
}

// Every theme conn ships reads, its dark ground dark and its light
// ground light.
func TestShippedThemes(t *testing.T) {
	names := Names(t.TempDir())
	if !slices.Equal(names, []string{"conn", "datum", "skelly"}) {
		t.Errorf("conn ships %v", names)
	}
	for _, n := range names {
		th, err := Load(t.TempDir(), n)
		if err != nil {
			t.Fatal(err)
		}
		if !th.Dark.Dark() || th.Light.Dark() {
			t.Errorf("%s: dark ground dark %v, light ground dark %v", n, th.Dark.Dark(), th.Light.Dark())
		}
	}
}

// Every theme conn ships spends its accent alike, as one color at a
// time. Most spend the accent itself: the cursor and the block are the
// accent, and the row under the cursor is on the border in the ink, so
// nothing chosen reads as what needs you. A theme whose daylight accent
// is a fill too pale to read as text spends that fill wherever the
// accent is a shape, with the ink on it: the block, the cursor and the
// row under it.
func TestShippedThemesSpendTheAccentAlike(t *testing.T) {
	for _, n := range Names(t.TempDir()) {
		th, _ := Load(t.TempDir(), n)
		for _, g := range []Ground{th.Dark, th.Light} {
			ink := Hex(g.Ink)
			switch {
			case g.Block == g.Accent:
				if g.Cursor != g.Accent || g.Chosen != g.Border || g.ChosenInk != ink {
					t.Errorf("%s: accent %s, cursor %s, the chosen row %s on %s", n, g.Accent, g.Cursor, g.ChosenInk, g.Chosen)
				}
			case g.Dark():
				t.Errorf("%s: a dark ground's block %s is not its accent %s", n, g.Block, g.Accent)
			default:
				if g.Cursor != g.Block || g.Chosen != g.Block || g.ChosenInk != ink || g.OnBlock != ink {
					t.Errorf("%s: the fill %s is not the cursor %s and the chosen row %s, each in ink", n, g.Block, g.Cursor, g.Chosen)
				}
			}
		}
	}
}
