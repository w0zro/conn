package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The colorscheme is one nvim can read: it names itself, it is one
// ground, and it writes no group twice.
func TestTheVimColorschemeIsAColorscheme(t *testing.T) {
	out := vimColorscheme()
	for _, s := range []string{"set background=dark", "hi clear", "let g:colors_name = 'conn'"} {
		if !strings.Contains(out, s) {
			t.Errorf("the colorscheme lacks %q", s)
		}
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "hi ") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 3 {
			continue // hi clear
		}
		g := f[1]
		if seen[g] {
			t.Errorf("%s is written twice", g)
		}
		seen[g] = true
	}
	if len(seen) < 100 {
		t.Errorf("only %d groups; the scheme should cover the editor", len(seen))
	}
	// A terminal opened in nvim gets the same sixteen.
	for i, c := range scheme {
		if want := "let g:terminal_color_" + strconv.Itoa(i) + " = '" + c + "'"; !strings.Contains(out, want) {
			t.Errorf("the colorscheme lacks %q", want)
		}
	}
}

// Every color in it is one conn draws, and each carries the slot it is,
// so the scheme holds up where sixteen is all there is.
func TestTheVimColorschemeIsDrawnFromConnsOwn(t *testing.T) {
	known := map[string]bool{
		"NONE": true, hex(groundColor): true, hex(inkColor): true, grayHex: true,
		borderHex: true, parchmentHex: true, cursorHex: true, toolBg: true, diffAddedBg: true, diffRemovedBg: true,
		diffAddedWord: true, diffRemovedWord: true, diffAddedDim: true,
	}
	slotOf := map[string]string{}
	for i, c := range scheme {
		known[c] = true
		slotOf[c] = strconv.Itoa(i)
	}
	for _, line := range strings.Split(vimColorscheme(), "\n") {
		if !strings.HasPrefix(line, "hi ") {
			continue
		}
		for _, f := range strings.Fields(line) {
			key, val, ok := strings.Cut(f, "=")
			if !ok || !strings.HasPrefix(key, "gui") || key == "gui" {
				continue
			}
			if !known[val] {
				t.Errorf("%s: %s is no color of conn's", line, val)
			}
			// A color that is one of the sixteen says which one on the
			// cterm side of the same line.
			if slot, isSlot := slotOf[val]; isSlot && key != "guisp" {
				want := strings.Replace(key, "gui", "cterm", 1) + "=" + slot
				if !strings.Contains(line, want) {
					t.Errorf("%s: %s is slot %s and does not say so", line, val, want)
				}
			}
		}
	}
}

// The roles are the ones conn's sixteen were laid out for, and the ones
// a shell theme leans on: a string is the green slot, a keyword the
// blue, a type the cyan, a call apart from a definition, a parameter
// apart from a variable.
func TestTheVimRolesFollowTheSlots(t *testing.T) {
	at := map[string]string{}
	for _, line := range strings.Split(vimColorscheme(), "\n") {
		if !strings.HasPrefix(line, "hi ") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 3 {
			continue // hi clear
		}
		at[f[1]] = strings.TrimPrefix(f[2], "guifg=")
	}
	for group, slot := range map[string]int{
		"String": 2, "Constant": 3, "Keyword": 4, "Function": 5, "Type": 6,
		"Number": 9, "Identifier": 11, "Operator": 12,
		"@function.call": 13, "@variable.parameter": 14, "Error": 1,
	} {
		if at[group] != scheme[slot] {
			t.Errorf("%s is %s, not slot %d (%s)", group, at[group], slot, scheme[slot])
		}
	}
	// A call is not a definition, and a parameter is not a variable.
	if at["@function.call"] == at["Function"] || at["@variable.parameter"] == at["Identifier"] {
		t.Error("the tier-two roles collapsed into their tier-one siblings")
	}
}

// What conn dresses elsewhere it dresses here the same way: the orange
// is the thing that wants you, a selection is the border color, and a
// diff's washes are the washes.
func TestTheVimSchemeAgreesWithTheRest(t *testing.T) {
	out := vimColorscheme()
	for _, want := range []string{
		"hi Search guifg=" + hex(groundColor) + " ctermfg=NONE guibg=" + cursorHex,
		"hi Visual guifg=NONE ctermfg=NONE guibg=" + borderHex,
		"hi DiffAdd guifg=NONE ctermfg=NONE guibg=" + diffAddedBg,
		"hi DiffDelete guifg=" + faintHex + " ctermfg=8 guibg=" + diffRemovedBg,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the colorscheme lacks %q", want)
		}
	}
	// No mode-like chrome takes the orange: it is for what wants you.
	for _, line := range strings.Split(out, "\n") {
		for _, g := range []string{"hi StatusLine ", "hi Pmenu ", "hi CursorLine ", "hi Comment "} {
			if strings.HasPrefix(line, g) && strings.Contains(line, cursorHex) {
				t.Errorf("%s takes the orange", strings.TrimSpace(g))
			}
		}
	}
}

// conn writes the colorscheme where nvim looks for one, and says so.
func TestConnWritesTheVimColorscheme(t *testing.T) {
	holdMode(t) // dressProgram puts conn on the ground its server is on
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	msg, ok := dressProgram([]string{"vim"}, home, nil)
	if !ok || !strings.Contains(msg, "nvim/colors/conn.vim") {
		t.Fatalf("not written: %q", msg)
	}
	b, err := os.ReadFile(filepath.Join(home, ".config", "nvim", "colors", "conn.vim"))
	if err != nil || !strings.Contains(string(b), "let g:colors_name = 'conn'") {
		t.Errorf("the file on disk: %v", err)
	}
	// nvim is the same program by either name.
	if _, ok := dressProgram([]string{"nvim"}, home, nil); !ok {
		t.Error("conn theme nvim was not taken")
	}
	// XDG_CONFIG_HOME is where it goes when it is set.
	other := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", other)
	if _, ok := dressProgram([]string{"vim"}, home, nil); !ok {
		t.Fatal("not written under XDG_CONFIG_HOME")
	}
	if _, err := os.Stat(filepath.Join(other, "nvim", "colors", "conn.vim")); err != nil {
		t.Errorf("XDG_CONFIG_HOME was not used: %v", err)
	}
}

// The colorscheme follows the ground the server settles on. conn writes
// it once, on whichever ground conn theme vim was run under; a server
// that comes up on the other one catches the file up, so the next nvim
// started in conn is drawn on the ground conn is actually on rather
// than the one it was on the day the file was written.
func TestTheVimColorschemeFollowsTheGround(t *testing.T) {
	holdMode(t)
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	path := filepath.Join(home, ".config", "nvim", "colors", "conn.vim")

	// Never written: refreshing writes nothing. conn theme vim is what
	// puts the file there, and a machine that never asked for one is not
	// given one behind its back.
	applyMode(true)
	refreshVimColorscheme(home)
	if _, err := os.Stat(path); err == nil {
		t.Fatal("refreshVimColorscheme wrote a file conn theme vim never had")
	}

	// Written on dark; the server comes up light; the file catches up.
	if _, err := writeVimColorscheme(home); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(path); err != nil || !strings.Contains(string(b), "set background=dark") {
		t.Fatalf("the file was not written dark: %v", err)
	}
	applyMode(false)
	refreshVimColorscheme(home)
	b, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(b), "set background=light") {
		t.Fatalf("the file did not follow the ground to light: %v", err)
	}
	// The ground is not only the word: the colors go with it.
	if strings.Contains(string(b), hex(darkGround)) {
		t.Error("the light colorscheme still carries the dark ground")
	}
}

// On light, the border is not slot 0. Light's slot 0 is black — what a
// program writing ANSI-0 means by ordinary text — and everything drawn
// on the border took it: the status line was a black bar across a pale
// page, and a selection was a black block. The border on light is the
// color the rest of conn draws it in, and the quietest text is the tier
// conn reads there rather than the slot that vanishes.
func TestTheLightColorschemeDrawsTheBorderAsTheRestOfConnDoes(t *testing.T) {
	holdMode(t)
	applyMode(false)
	out := vimColorscheme()
	for _, want := range []string{
		"hi StatusLine guifg=" + hex(inkColor) + " ctermfg=15 guibg=" + lightBorderHex + " ctermbg=NONE",
		"hi Visual guifg=NONE ctermfg=NONE guibg=" + lightBorderHex + " ctermbg=NONE",
		"hi WinSeparator guifg=" + lightBorderHex + " ctermfg=NONE",
		"hi LineNr guifg=" + lightFaintHex + " ctermfg=8",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the light colorscheme lacks %q", want)
		}
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "hi ") && strings.Contains(line, "guibg="+lightScheme[0]) {
			t.Errorf("a ground is drawn in light's black: %s", line)
		}
	}
	// And on dark the border is slot 0 still, as it always was.
	applyMode(true)
	if out := vimColorscheme(); !strings.Contains(out, "hi Visual guifg=NONE ctermfg=NONE guibg="+darkScheme[0]+" ctermbg=0") {
		t.Error("the dark colorscheme no longer draws a selection on slot 0")
	}
}
