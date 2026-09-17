package main

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
		g    ground
	}{{"dark", datumTheme.dark}, {"light", datumTheme.light}} {
		g := on.g
		keys, palette := readGhostty(t, "ghostty-"+on.name)
		claude := readClaude(t, "claude-"+on.name+".json")
		for i := range palette {
			if !strings.EqualFold(g.scheme[i], palette[i]) {
				t.Errorf("%s slot %d is %s; datum's is %s", on.name, i, g.scheme[i], palette[i])
			}
		}
		for _, c := range []struct{ role, have, want, from string }{
			{"ground", hex(g.ground), keys["background"], "background"},
			{"ink", hex(g.ink), keys["foreground"], "foreground"},
			{"accent", g.accent, keys["cursor-color"], "cursor-color"},
			{"border", g.border, keys["selection-background"], "selection-background"},
			{"shimmer", g.shimmer, palette[13], "slot 13, call"},
			{"gray", g.gray, claude["inactive"], "inactive"},
			{"faint", g.faint, claude["promptBorder"], "promptBorder"},
			{"parchment", g.parchment, claude["text"], "text"},
			{"messageBg", g.messageBg, claude["userMessageBackground"], "userMessageBackground"},
			{"messageHoverBg", g.messageHoverBg, claude["userMessageBackgroundHover"], "userMessageBackgroundHover"},
			{"toolBg", g.toolBg, claude["userMessageBackground"], "userMessageBackground, bg1"},
			{"diffAddedBg", g.diffAddedBg, claude["diffAdded"], "diffAdded"},
			{"diffRemovedBg", g.diffRemovedBg, claude["diffRemoved"], "diffRemoved"},
			{"diffAddedDim", g.diffAddedDim, claude["diffAddedDimmed"], "diffAddedDimmed"},
			{"diffRemovedDim", g.diffRemovedDim, claude["diffRemovedDimmed"], "diffRemovedDimmed"},
			{"diffAddedWord", g.diffAddedWord, claude["diffAddedWord"], "diffAddedWord"},
			{"diffRemovedWord", g.diffRemovedWord, claude["diffRemovedWord"], "diffRemovedWord"},
		} {
			if c.want == "" {
				t.Errorf("%s: datum's port has no %s", on.name, c.from)
			} else if !strings.EqualFold(c.have, c.want) {
				t.Errorf("%s %s is %s; datum's %s is %s", on.name, c.role, c.have, c.from, c.want)
			}
		}
	}
}

// Under datum light, slot 7 is bg2 and slot 15 is bg1 - the whites, as
// ANSI means them, and no ink at all on a light ground. Everything conn
// draws as text reads there all the same, because it reads its inks by
// name: the console's own palette, what conn says on the status line,
// what Claude Code writes in, and the text of the vim colorscheme.
func TestConnReadsUnderDatumLight(t *testing.T) {
	holdMode(t)
	applyMode(mode{theme: "datum", dark: false})
	const readable = 4.5
	bg := hex(groundColor)
	if c := contrast(scheme[7], bg); c >= readable {
		t.Fatalf("datum light's slot 7 (%s) reads as text at %.2f:1; this test has nothing to show", scheme[7], c)
	}

	if c := contrast(parchmentHex, bg); c < readable {
		t.Errorf("the parchment (%s) is %.2f:1 on datum light", parchmentHex, c)
	}
	if say := statusLineSay("HELLO"); !strings.Contains(say, "fg="+parchmentHex) {
		t.Errorf("the status line says its word in %q, not the parchment", say)
	}
	at := map[string]string{}
	for _, g := range claudeTheme() {
		for _, tk := range g {
			at[tk.name] = tk.color
		}
	}
	for _, k := range []string{"text", "bashBorder", "claude", "permission", "inactive"} {
		if c := contrast(at[k], bg); c < readable {
			t.Errorf("Claude Code's %s (%s) is %.2f:1 on datum light", k, at[k], c)
		}
	}
	for _, line := range strings.Split(vimColorscheme(), "\n") {
		for _, g := range []string{"hi Normal ", "hi Delimiter ", "hi StatusLine ", "hi Pmenu ", "hi Title ", "hi @punctuation.delimiter "} {
			if !strings.HasPrefix(line, g) {
				continue
			}
			_, rest, _ := strings.Cut(line, "guifg=")
			fg, _, _ := strings.Cut(rest, " ")
			if c := contrast(fg, bg); c < readable {
				t.Errorf("%s draws text in %s, %.2f:1 on datum light", strings.TrimSpace(g), fg, c)
			}
		}
	}
}

// A role's color in the vim colorscheme carries the slot that holds
// the same hex, whichever theme that is, and NONE where none does.
func TestARoleColorSaysItsSlot(t *testing.T) {
	holdMode(t)
	for _, c := range []struct {
		m    mode
		role string
		want string
	}{
		{connOn(true), "accent", "9"},
		{connOn(true), "border", "0"},
		{connOn(false), "border", "NONE"},
		{connOn(false), "ink", "15"},
		{mode{"datum", true}, "accent", "5"},
		{mode{"datum", true}, "ink", "7"},
		{mode{"datum", false}, "ink", "0"},
		{mode{"datum", false}, "border", "7"},
		{mode{"datum", false}, "faint", "NONE"},
	} {
		applyMode(c.m)
		h := map[string]string{"accent": cursorHex, "border": borderHex, "ink": hex(inkColor), "faint": faintHex}[c.role]
		if got := roleColor(h); got.cterm != c.want || got.gui != h {
			t.Errorf("%s %s: roleColor(%s) = %+v, want cterm %s", c.m.theme, c.role, h, got, c.want)
		}
	}
}
