package theme

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
// for none of them. It writes its own hex, and tmux passes truecolor
// through untouched, so it is the one program in a pane that conn's
// palette cannot reach. conn theme claude writes a theme for it.
//
// The theme sits on Claude Code's ansi base, so a token conn says
// nothing about falls through to the slot the pane already has. A token
// whose meaning is slot-shaped is written as a reference to the slot,
// and stays in lockstep with the palette by construction; a token that
// wants one of conn's own colors is written from the same table the
// palette comes from. Only the grounds no slot has a name for, the
// washes under a diff and the band behind a message, are spelled out.
//
// Claude Code's own syntax coloring is not a theme's to set: it is a
// fixed map onto the sixteen, which conn has already dressed. Code in a
// pane is conn's colors either way.

// Where Claude Code keeps what conn writes and what it reads back.
const (
	claudeDir      = ".claude"
	ClaudeThemeRef = "custom:conn"
)

// A token and what conn would have it drawn in.
type token struct{ Name, Color string }

// ClaudeTheme is the theme, in the order the handoff lays it out: the
// mark, the words, the words that carry a verdict, the chrome that says
// what Claude may do, the diffs, the bars, the meter, and the two agent
// colors conn has an opinion about.
func ClaudeTheme(g Ground) [][]token {
	var (
		yellow, cyan  = g.Scheme[3], g.Scheme[6]
		faint, accent = g.Faint, g.Accent
		ink           = Hex(g.Ink)
	)
	return [][]token{{
		{"claude", g.AtWork},
		{"claudeShimmer", g.AtWorkShimmer},
	}, {
		{"text", ink},
		{"inverseText", Hex(g.Ground)},
		{"inactive", g.Gray},
		// subtle is the only color Claude Code draws the > before a
		// message of yours in, so it takes the accent, and with it the
		// rest it marks: a queued message, a rejection, the usage
		// figures, a task's state.
		{"subtle", accent},
		{"suggestion", g.Gray},
		{"remember", cyan},
	}, {
		// A verdict is slot-shaped: it follows the pane's own sixteen.
		{"success", "ansi:green"},
		{"error", "ansi:red"},
		{"warning", "ansi:yellow"},
		{"merged", "ansi:magenta"},
	}, {
		// The accent is "you, here": the one dialog that stops and waits
		// for you. A mode is quiet unless it changes what Claude may do.
		{"promptBorder", faint},
		{"permission", accent},
		{"permissionShimmer", g.Shimmer},
		{"planMode", cyan},
		{"autoAccept", yellow},
		{"bashBorder", g.Parchment},
		{"ide", cyan},
		{"fastMode", yellow},
	}, {
		{"diffAdded", g.DiffAddedBg},
		{"diffRemoved", g.DiffRemovedBg},
		{"diffAddedDimmed", g.DiffAddedDim},
		{"diffRemovedDimmed", g.DiffRemovedDim},
		{"diffAddedWord", g.DiffAddedWord},
		{"diffRemovedWord", g.DiffRemovedWord},
	}, {
		{"userMessageBackground", g.MessageBg},
		{"userMessageBackgroundHover", g.MessageHoverBg},
		// A selection is the row under the cursor's ground, as the panel
		// and copy mode lay it.
		{"selectionBg", g.Chosen},
		{"bashMessageBackgroundColor", g.ToolBg},
		{"memoryBackgroundColor", g.ToolBg},
	}, {
		{"rate_limit_fill", accent},
		{"rate_limit_empty", g.Border},
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
func claudeThemeJSON(g Ground) string {
	var b strings.Builder
	fmt.Fprintf(&b, "{\n  \"name\": \"Conn\",\n  \"base\": %q,\n  \"overrides\": {\n", g.claudeBase())
	groups := ClaudeTheme(g)
	for i, grp := range groups {
		for j, t := range grp {
			comma := ","
			if i == len(groups)-1 && j == len(grp)-1 {
				comma = ""
			}
			fmt.Fprintf(&b, "    %q: %q%s\n", t.Name, t.Color, comma)
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
func (g Ground) claudeBase() string {
	if g.Dark() {
		return "dark-ansi"
	}
	return "light-ansi"
}

// claudeThemePath is where Claude Code looks for the theme.
func claudeThemePath(home string) string {
	return filepath.Join(home, claudeDir, "themes", "conn.json")
}

// WriteClaudeTheme writes the theme for a ground where Claude Code
// looks for it, and answers the path it wrote.
func WriteClaudeTheme(home string, g Ground) (string, error) {
	path := claudeThemePath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, []byte(claudeThemeJSON(g)), 0o644)
}

// RefreshClaudeTheme rewrites conn's theme for Claude Code if it has
// been written once, so a server on a ground never leaves the file
// behind on the one it was last written under. It writes nothing where
// conn theme claude has never run: that command is what puts the file
// there the first time.
func RefreshClaudeTheme(home string, g Ground) {
	if _, err := os.Stat(claudeThemePath(home)); err != nil {
		return
	}
	_, _ = WriteClaudeTheme(home, g)
}

// BuiltinThemes are the themes Claude Code comes with. A settings file
// on one of these, or on none at all, is one conn can offer to point
// at itself; a custom theme is somebody's own, and conn leaves it.
var BuiltinThemes = map[string]bool{
	"": true, "dark": true, "light": true, "dark-ansi": true, "light-ansi": true,
	"dark-daltonized": true, "light-daltonized": true,
}

// ThemeInUse is the theme named in Claude Code's settings, and whether
// the file is there to be read at all.
func ThemeInUse(home string) (string, bool) {
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

// themeKey finds the one place in the settings file where the theme is
// set. conn edits that value in the text rather than writing the file
// back out, so the order and the shape of everything else in it stay
// the user's.
var themeKey = regexp.MustCompile(`"theme"\s*:\s*"[^"]*"`)

// UseClaudeTheme points Claude Code's settings at conn's theme.
func UseClaudeTheme(home string) error {
	path := filepath.Join(home, claudeDir, "settings.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	found := themeKey.FindAllIndex(b, -1)
	if len(found) != 1 {
		return fmt.Errorf(`%s names "theme" %d times; conn will not guess which`, path, len(found))
	}
	out := themeKey.ReplaceAll(b, []byte(`"theme": "`+ClaudeThemeRef+`"`))
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, info.Mode().Perm())
}
