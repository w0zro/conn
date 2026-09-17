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
func claudeTheme() [][]token {
	var (
		yellow, cyan  = scheme[3], scheme[6]
		faint, accent = faintHex, cursorHex
		ink           = hex(inkColor)
	)
	return [][]token{{
		{"claude", accent},
		{"claudeShimmer", shimmerHex},
	}, {
		{"text", ink},
		{"inverseText", hex(groundColor)},
		{"inactive", grayHex},
		{"subtle", faint},
		{"suggestion", grayHex},
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
		{"permissionShimmer", shimmerHex},
		{"planMode", cyan},
		{"autoAccept", yellow},
		{"bashBorder", parchmentHex},
		{"ide", cyan},
		{"fastMode", yellow},
	}, {
		{"diffAdded", diffAddedBg},
		{"diffRemoved", diffRemovedBg},
		{"diffAddedDimmed", diffAddedDim},
		{"diffRemovedDimmed", diffRemovedDim},
		{"diffAddedWord", diffAddedWord},
		{"diffRemovedWord", diffRemovedWord},
	}, {
		{"userMessageBackground", messageBg},
		{"userMessageBackgroundHover", messageHoverBg},
		{"selectionBg", borderHex},
		{"bashMessageBackgroundColor", toolBg},
		{"memoryBackgroundColor", toolBg},
	}, {
		{"rate_limit_fill", accent},
		{"rate_limit_empty", borderHex},
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
func claudeThemeJSON() string {
	var b strings.Builder
	fmt.Fprintf(&b, "{\n  \"name\": \"Conn\",\n  \"base\": %q,\n  \"overrides\": {\n", themeBase)
	groups := claudeTheme()
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

// writeClaudeTheme writes the theme where Claude Code looks for it, and
// answers the path it wrote.
func writeClaudeTheme(home string) (string, error) {
	dir := filepath.Join(home, claudeDir, "themes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "conn.json")
	return path, os.WriteFile(path, []byte(claudeThemeJSON()), 0o644)
}

// refreshClaudeTheme rewrites conn's theme for Claude Code if it has
// already been written once, so a server that settles on a mode never
// leaves the file behind on the mode it was last written under. It
// writes nothing where `conn theme claude` has never run - that command
// is still what puts the file there the first time.
func refreshClaudeTheme(home string) {
	path := filepath.Join(home, claudeDir, "themes", "conn.json")
	if _, err := os.Stat(path); err != nil {
		return
	}
	_, _ = writeClaudeTheme(home)
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
