package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// A step from one color to another: the ends are the ends, and the
// middle is between them.
func TestAColorIsMixed(t *testing.T) {
	a, b := "#000000", "#FFFFFF"
	if got := mix(a, b, 0); got != "#000000" {
		t.Errorf("no step: %s", got)
	}
	if got := mix(a, b, 1); got != "#FFFFFF" {
		t.Errorf("the whole step: %s", got)
	}
	if got := mix(a, b, 0.5); got != "#808080" {
		t.Errorf("half a step: %s", got)
	}
	if got := rgbString("#7FA7C9"); got != "rgb(127,167,201)" {
		t.Errorf("as a theme writes it: %s", got)
	}
}

// The theme conn prints is Claude Code's own shape, and every color in
// it is one conn draws: the sixteen, the console's grounds and ranks,
// or a step between two of those.
func TestTheClaudeThemeIsConnsOwn(t *testing.T) {
	out, err := claudeThemeJSON()
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Name      string            `json:"name"`
		Base      string            `json:"base"`
		Overrides map[string]string `json:"overrides"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not a theme file: %v", err)
	}
	if got.Name != "conn" || got.Base != "dark" {
		t.Errorf("named %q on %q", got.Name, got.Base)
	}
	for k, v := range got.Overrides {
		if !strings.HasPrefix(v, "rgb(") || !strings.HasSuffix(v, ")") {
			t.Errorf("%s is not written as a color: %s", k, v)
		}
	}
	// The page is the console's, not a theme's own.
	for k, want := range map[string]string{
		"background":  rgbString(hex(groundColor)),
		"text":        rgbString(hex(inkColor)),
		"inverseText": rgbString(hex(groundColor)),
		"selectionBg": rgbString(borderHex),
	} {
		if got.Overrides[k] != want {
			t.Errorf("%s is %s, not %s", k, got.Overrides[k], want)
		}
	}
}

// Orange is what conn spends on the thing that wants you. A mode is on
// all day, so no mode may take it; an error may.
func TestOrangeIsNotSpentOnAMode(t *testing.T) {
	th := claudeTheme()
	orange, owed := scheme[9], scheme[1]
	for _, mode := range []string{"planMode", "autoAccept", "skill", "merged", "effortUltra", "fastMode"} {
		if c := th[mode]; c == orange || c == owed {
			t.Errorf("%s takes the orange: %s", mode, c)
		}
	}
	if th["error"] != owed {
		t.Errorf("an error is %s, not the red", th["error"])
	}
	if th["claude"] != orange {
		t.Errorf("the mark is %s, not the orange", th["claude"])
	}
}

// An agent is told apart from another by its color, so no two of them
// may be the same.
func TestTheAgentColorsAreApart(t *testing.T) {
	th := claudeTheme()
	seen := map[string]string{}
	for _, k := range []string{
		"red_FOR_SUBAGENTS_ONLY", "orange_FOR_SUBAGENTS_ONLY", "yellow_FOR_SUBAGENTS_ONLY",
		"green_FOR_SUBAGENTS_ONLY", "cyan_FOR_SUBAGENTS_ONLY", "blue_FOR_SUBAGENTS_ONLY",
		"purple_FOR_SUBAGENTS_ONLY", "pink_FOR_SUBAGENTS_ONLY",
	} {
		c, ok := th[k]
		if !ok {
			t.Errorf("no color for %s", k)
			continue
		}
		if was, dup := seen[c]; dup {
			t.Errorf("%s is %s again: %s", k, was, c)
		}
		seen[c] = k
	}
}

// conn dresses one program it holds, and says so for any other.
func TestConnDressesClaudeAndSaysSoOtherwise(t *testing.T) {
	if _, err := themeFor([]string{"claude"}); err != nil {
		t.Errorf("no theme for claude: %v", err)
	}
	for _, args := range [][]string{{}, {"vim"}, {"claude", "dark"}} {
		if _, err := themeFor(args); err == nil {
			t.Errorf("conn theme %v gave a theme", args)
		}
	}
}
