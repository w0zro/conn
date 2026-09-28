package theme

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// readGhostty is a datum Ghostty port: its keys, and its sixteen.
func readGhostty(t *testing.T, name string) (map[string]string, [16]string) {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "datum", name))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	keys, palette := map[string]string{}, [16]string{}
	for sc := bufio.NewScanner(f); sc.Scan(); {
		k, v, ok := strings.Cut(sc.Text(), " = ")
		if !ok || strings.HasPrefix(k, "#") {
			continue
		}
		if k == "palette" {
			slot, hex, _ := strings.Cut(v, "=")
			i, err := strconv.Atoi(slot)
			if err != nil || i < 0 || i > 15 {
				t.Fatalf("%s: slot %q", name, slot)
			}
			palette[i] = hex
			continue
		}
		keys[k] = v
	}
	return keys, palette
}

// readClaude is a datum Claude Code port: its overrides, each rgb(r,g,b)
// written back as a hex.
func readClaude(t *testing.T, name string) map[string]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "datum", name))
	if err != nil {
		t.Fatal(err)
	}
	var port struct {
		Overrides map[string]string `json:"overrides"`
	}
	if err := json.Unmarshal(b, &port); err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for k, v := range port.Overrides {
		var r, g, bl int
		if _, err := fmt.Sscanf(v, "rgb(%d,%d,%d)", &r, &g, &bl); err != nil {
			t.Fatalf("%s: %s is %q", name, k, v)
		}
		out[k] = fmt.Sprintf("#%02X%02X%02X", r, g, bl)
	}
	return out
}

// datum is datum's own: every value in conn's table is the one datum's
// generator wrote into a port, kept under testdata/datum. The sixteen,
// the ground and the ink and the cursor and the selection are the
// Ghostty port's; the washes, the bands and the quietest text are the
// Claude Code port's. Refreshing datum is copying the four files.
func TestDatumIsDatumsOwn(t *testing.T) {
	for _, on := range []struct {
		name string
		g    Ground
	}{{"dark", Datum.Dark}, {"light", Datum.Light}} {
		g := on.g
		keys, palette := readGhostty(t, "ghostty-"+on.name)
		claude := readClaude(t, "claude-"+on.name+".json")
		for i := range palette {
			if !strings.EqualFold(g.Scheme[i], palette[i]) {
				t.Errorf("%s slot %d is %s; datum's is %s", on.name, i, g.Scheme[i], palette[i])
			}
		}
		for _, c := range []struct{ role, have, want, from string }{
			{"ground", Hex(g.Ground), keys["background"], "background"},
			{"ink", Hex(g.Ink), keys["foreground"], "foreground"},
			{"accent", g.Accent, keys["cursor-color"], "cursor-color"},
			{"border", g.Border, keys["selection-background"], "selection-background"},
			{"shimmer", g.Shimmer, palette[13], "slot 13, call"},
			{"gray", g.Gray, claude["inactive"], "inactive"},
			{"faint", g.Faint, claude["promptBorder"], "promptBorder"},
			{"parchment", g.Parchment, claude["text"], "text"},
			{"messageBg", g.MessageBg, claude["userMessageBackground"], "userMessageBackground"},
			{"messageHoverBg", g.MessageHoverBg, claude["userMessageBackgroundHover"], "userMessageBackgroundHover"},
			{"toolBg", g.ToolBg, claude["userMessageBackground"], "userMessageBackground, bg1"},
			{"diffAddedBg", g.DiffAddedBg, claude["diffAdded"], "diffAdded"},
			{"diffRemovedBg", g.DiffRemovedBg, claude["diffRemoved"], "diffRemoved"},
			{"diffAddedDim", g.DiffAddedDim, claude["diffAddedDimmed"], "diffAddedDimmed"},
			{"diffRemovedDim", g.DiffRemovedDim, claude["diffRemovedDimmed"], "diffRemovedDimmed"},
			{"diffAddedWord", g.DiffAddedWord, claude["diffAddedWord"], "diffAddedWord"},
			{"diffRemovedWord", g.DiffRemovedWord, claude["diffRemovedWord"], "diffRemovedWord"},
		} {
			if c.want == "" {
				t.Errorf("%s: datum's port has no %s", on.name, c.from)
			} else if !strings.EqualFold(c.have, c.want) {
				t.Errorf("%s %s is %s; datum's %s is %s", on.name, c.role, c.have, c.from, c.want)
			}
		}
	}
}

// A role's color in the vim colorscheme carries the slot that holds
// the same hex, whichever theme that is, and NONE where none does.
func TestARoleColorSaysItsSlot(t *testing.T) {
	for _, c := range []struct {
		m    Mode
		role string
		want string
	}{
		{connOn(true), "accent", "9"},
		{connOn(true), "border", "0"},
		{connOn(false), "border", "NONE"},
		{connOn(false), "ink", "15"},
		{Mode{"datum", true}, "accent", "5"},
		{Mode{"datum", true}, "ink", "7"},
		{Mode{"datum", false}, "ink", "0"},
		{Mode{"datum", false}, "border", "7"},
		{Mode{"datum", false}, "faint", "NONE"},
	} {
		g := c.m.Wear()
		h := map[string]string{"accent": g.Accent, "border": g.Border, "ink": Hex(g.Ink), "faint": g.Faint}[c.role]
		if got := RoleColor(g, h); got.Cterm != c.want || got.GUI != h {
			t.Errorf("%s %s: RoleColor(%s) = %+v, want cterm %s", c.m.Theme, c.role, h, got, c.want)
		}
	}
}
