package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Everything in conn's server asks the terminal for a color by name and
// gets conn's: tmux draws the sixteen from the scheme. Claude Code asks
// for none of them — it writes its own hex, and tmux passes truecolor
// through untouched — so it is the one program in a pane that conn's
// palette cannot reach. conn theme claude writes a theme for it.
//
// The theme sits on Claude Code's ansi base, so a token conn says
// nothing about falls through to the slot the pane already has. A token
// whose meaning is slot-shaped is written as a reference to the slot,
// and stays in lockstep with the palette by construction; a token that
// wants one of conn's own colors is written from the same table the
// palette comes from. Only the grounds no slot has a name for — the
// washes under a diff, the band behind a message — are spelled
// out.
//
// Claude Code's own syntax coloring is not a theme's to set: it is a
// fixed map onto the sixteen, which conn has already dressed. Code in a
// pane is conn's colors either way.

// Where Claude Code keeps what conn writes and what it reads back.
const (
	claudeDir      = ".claude"
	claudeThemeRef = "custom:conn"
)

// A token and what conn would have it drawn in.
type token struct{ name, color string }

// claudeTheme is the theme, in the order the handoff lays it out: the
// mark, the words, the words that carry a verdict, the chrome that says
// what Claude may do, the diffs, the bars, the meter, and the two agent
// colors conn has an opinion about.
func claudeTheme(g ground) [][]token {
	var (
		yellow, cyan  = g.scheme[3], g.scheme[6]
		faint, accent = g.faint, g.accent
		ink           = hex(g.ink)
	)
	return [][]token{{
		{"claude", accent},
		{"claudeShimmer", g.shimmer},
	}, {
		{"text", ink},
		{"inverseText", hex(g.ground)},
		{"inactive", g.gray},
		{"subtle", faint},
		{"suggestion", g.gray},
		{"remember", cyan},
	}, {
		// A verdict is slot-shaped: it follows the pane's own sixteen.
		{"success", "ansi:green"},
		{"error", "ansi:red"},
		{"warning", "ansi:yellow"},
		{"merged", "ansi:magenta"},
	}, {
		// The accent is "you, here": the mark, and the one dialog that
		// stops and waits for you. A mode is quiet unless it changes what
		// Claude may do.
		{"promptBorder", faint},
		{"permission", accent},
		{"permissionShimmer", g.shimmer},
		{"planMode", cyan},
		{"autoAccept", yellow},
		{"bashBorder", g.parchment},
		{"ide", cyan},
		{"fastMode", yellow},
	}, {
		{"diffAdded", g.diffAddedBg},
		{"diffRemoved", g.diffRemovedBg},
		{"diffAddedDimmed", g.diffAddedDim},
		{"diffRemovedDimmed", g.diffRemovedDim},
		{"diffAddedWord", g.diffAddedWord},
		{"diffRemovedWord", g.diffRemovedWord},
	}, {
		{"userMessageBackground", g.messageBg},
		{"userMessageBackgroundHover", g.messageHoverBg},
		{"selectionBg", g.border},
		{"bashMessageBackgroundColor", g.toolBg},
		{"memoryBackgroundColor", g.toolBg},
	}, {
		{"rate_limit_fill", accent},
		{"rate_limit_empty", g.border},
	}, {
		// The rest of the agent colors keep Claude Code's own until conn
		// has agents of its own to tell apart.
		{"orange_FOR_SUBAGENTS_ONLY", accent},
		{"cyan_FOR_SUBAGENTS_ONLY", cyan},
	}}
}

// claudeThemeJSON is the theme as Claude Code reads it, written in the
// handoff's own order and grouping rather than sorted, so the file can
// be read against the handoff line for line.
func claudeThemeJSON(g ground) string {
	var b strings.Builder
	fmt.Fprintf(&b, "{\n  \"name\": \"Conn\",\n  \"base\": %q,\n  \"overrides\": {\n", g.claudeBase())
	groups := claudeTheme(g)
	for i, g := range groups {
		for j, t := range g {
			last := i == len(groups)-1 && j == len(g)-1
			comma := ","
			if last {
				comma = ""
			}
			fmt.Fprintf(&b, "    %q: %q%s\n", t.name, t.color, comma)
		}
		if i < len(groups)-1 {
			b.WriteString("\n")
		}
	}
	b.WriteString("  }\n}\n")
	return b.String()
}

// claudeBase is the base the theme sits on: the ansi theme Claude Code
// comes with for the ground conn is on, so a token conn says nothing
// about falls through to a slot laid out for that ground.
func (g ground) claudeBase() string {
	if g.dark() {
		return "dark-ansi"
	}
	return "light-ansi"
}

// writeClaudeTheme writes the theme for a ground where Claude Code
// looks for it, and answers the path it wrote.
func writeClaudeTheme(home string, g ground) (string, error) {
	dir := filepath.Join(home, claudeDir, "themes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "conn.json")
	return path, os.WriteFile(path, []byte(claudeThemeJSON(g)), 0o644)
}

// refreshClaudeTheme rewrites conn's theme for Claude Code if it has
// already been written once, so a server that settles on a mode never
// leaves the file behind on the mode it was last written under. It
// writes nothing where `conn theme claude` has never run - that command
// is still what puts the file there the first time.
func refreshClaudeTheme(home string, g ground) {
	path := filepath.Join(home, claudeDir, "themes", "conn.json")
	if _, err := os.Stat(path); err != nil {
		return
	}
	_, _ = writeClaudeTheme(home, g)
}

// The themes Claude Code comes with. A settings file on one of these,
// or on none at all, is one conn can offer to point at itself; a custom
// theme is somebody's own, and conn leaves it alone.
var builtinThemes = map[string]bool{
	"": true, "dark": true, "light": true, "dark-ansi": true, "light-ansi": true,
	"dark-daltonized": true, "light-daltonized": true,
}

// themeInUse is the theme named in Claude Code's settings, and whether
// the file is there to be read at all.
func themeInUse(home string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(home, claudeDir, "settings.json"))
	if err != nil {
		return "", false
	}
	var s struct {
		Theme string `json:"theme"`
	}
	if json.Unmarshal(b, &s) != nil {
		return "", false
	}
	return s.Theme, true
}

// themeKey finds the theme conn would rewrite: the one place in the
// settings file where the key is set. Conn edits that value in the text
// rather than writing the file back out, so the order and the shape of
// everything else in it are the user's still.
var themeKey = regexp.MustCompile(`"theme"\s*:\s*"[^"]*"`)

// useClaudeTheme points Claude Code's settings at conn's theme.
func useClaudeTheme(home string) error {
	path := filepath.Join(home, claudeDir, "settings.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	found := themeKey.FindAllIndex(b, -1)
	if len(found) != 1 {
		return fmt.Errorf(`%s names "theme" %d times; conn will not guess which`, path, len(found))
	}
	out := themeKey.ReplaceAll(b, []byte(`"theme": "`+claudeThemeRef+`"`))
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, info.Mode().Perm())
}
