package theme

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// skelly's sixteen are its brand's own: every slot, and the ground and
// the ink, are what the brand's palette.mjs --terminal wrote, kept
// under testdata/skelly, and so is the cursor at night. Refreshing
// skelly is writing the file again. By day the cursor is --uv-bg,
// not the terminal's --acc-text; and the selection is
// neither ground's terminal one, whose tint of phosphor in phosphor
// ink put three accents on the screen at once.
func TestSkellyIsSkellysOwn(t *testing.T) {
	skelly, err := Load(t.TempDir(), "skelly")
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join("testdata", "skelly", "terminal.json"))
	if err != nil {
		t.Fatal(err)
	}
	var port map[string]map[string]string
	if err := json.Unmarshal(b, &port); err != nil {
		t.Fatal(err)
	}
	slots := []string{
		"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white",
		"brightBlack", "brightRed", "brightGreen", "brightYellow", "brightBlue", "brightMagenta", "brightCyan", "brightWhite",
	}
	for _, on := range []struct {
		name string
		g    Ground
	}{{"dark", skelly.Dark}, {"light", skelly.Light}} {
		g, want := on.g, port[on.name]
		for i, k := range slots {
			if !strings.EqualFold(g.Scheme[i], want[k]) {
				t.Errorf("%s slot %d is %s; the brand's %s is %s", on.name, i, g.Scheme[i], k, want[k])
			}
		}
		checks := []struct{ role, have, from string }{
			{"ground", Hex(g.Ground), "background"},
			{"ink", Hex(g.Ink), "foreground"},
		}
		if on.name == "dark" {
			checks = append(checks, struct{ role, have, from string }{"cursor", g.Cursor, "cursor"})
		}
		for _, c := range checks {
			if want[c.from] == "" {
				t.Errorf("%s: the brand's terminal has no %s", on.name, c.from)
			} else if !strings.EqualFold(c.have, want[c.from]) {
				t.Errorf("%s %s is %s; the brand's %s is %s", on.name, c.role, c.have, c.from, want[c.from])
			}
		}
	}
}
