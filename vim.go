package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// nvim is the other program conn holds that draws in its own hex rather
// than asking the terminal for a color by name, so conn's palette does
// not reach it either. conn theme vim writes it a colorscheme from the
// same table the server's palette comes from.
//
// Every color carries the slot it is, as well as its hex, so the scheme
// holds up in a terminal that was only ever given sixteen; the grounds
// that are no slot say NONE and take the pane's own, which in conn is
// the ground already.

// vimBackground is what the colorscheme tells nvim its own background
// is: dark or light, whichever ground applyMode last chose.
var vimBackground = "dark"

// A color as a colorscheme names it: what to draw it in, and which of
// the sixteen it is when sixteen is all there is.
type vimColor struct{ gui, cterm string }

// A highlight: the group, what it is drawn in, what it sits on, and how.
type hl struct {
	group  string
	fg, bg vimColor
	attr   string   // bold, undercurl, italic; empty for none
	sp     vimColor // what an undercurl is drawn in
}

// A run of highlights under a heading, the way the file reads.
type hlSection struct {
	title string
	hls   []hl
}

var noColor = vimColor{}

// vimColorscheme is the colorscheme, ready to be written.
func vimColorscheme() string {
	var (
		ground    = vimColor{hex(groundColor), "NONE"} // the pane's own
		lift      = vimColor{toolBg, "NONE"}           // a step off it
		border    = borderColor()                      // and another
		red       = slotColor(1)                       // an error
		green     = slotColor(2)                       // a string
		yellow    = slotColor(3)                       // a constant
		blue      = slotColor(4)                       // a keyword
		magenta   = slotColor(5)                       // a function
		cyan      = slotColor(6)                       // a type
		parchment = slotColor(7)                       // punctuation
		faint     = vimColor{faintHex, "8"}            // a line number
		orange    = slotColor(9)                       // a number, a mark
		varc      = slotColor(11)                      // a variable
		op        = slotColor(12)                      // an operator
		call      = slotColor(13)                      // a call
		param     = slotColor(14)                      // a parameter
		ink       = slotColor(15)                      // what is written
		gray      = vimColor{grayHex, strconv.Itoa(8)} // a comment
		chipOn    = vimColor{hex(groundColor), "NONE"} // words on a chip
		added     = vimColor{diffAddedBg, "NONE"}      // a diff's washes
		removed   = vimColor{diffRemovedBg, "NONE"}    //
		addedWord = vimColor{diffAddedWord, "NONE"}    //
		removedW  = vimColor{diffRemovedWord, "NONE"}  //
		changed   = vimColor{diffAddedDim, "NONE"}     //
	)

	sections := []hlSection{{
		"The page", []hl{
			{"Normal", ink, ground, "", noColor},
			{"NormalNC", ink, ground, "", noColor},
			{"NormalFloat", ink, lift, "", noColor},
			{"FloatBorder", border, lift, "", noColor},
			{"FloatTitle", orange, lift, "bold", noColor},
			{"Cursor", ground, orange, "", noColor},
			{"lCursor", ground, orange, "", noColor},
			{"TermCursor", ground, orange, "", noColor},
			{"CursorLine", noColor, lift, "", noColor},
			{"CursorColumn", noColor, lift, "", noColor},
			{"ColorColumn", noColor, lift, "", noColor},
			{"Visual", noColor, border, "", noColor},
			{"VisualNOS", noColor, border, "", noColor},
			{"LineNr", faint, noColor, "", noColor},
			{"CursorLineNr", parchment, lift, "bold", noColor},
			{"SignColumn", faint, noColor, "", noColor},
			{"FoldColumn", faint, noColor, "", noColor},
			{"Folded", gray, lift, "", noColor},
			{"NonText", border, noColor, "", noColor},
			{"Whitespace", border, noColor, "", noColor},
			{"EndOfBuffer", border, noColor, "", noColor},
			{"Conceal", faint, noColor, "", noColor},
			{"Directory", blue, noColor, "", noColor},
			{"Title", orange, noColor, "bold", noColor},
			{"MatchParen", orange, border, "bold", noColor},
			{"WinSeparator", border, noColor, "", noColor},
			{"VertSplit", border, noColor, "", noColor},
		},
	}, {
		"What conn spends the orange on: the thing that wants you", []hl{
			{"Search", chipOn, orange, "", noColor},
			{"IncSearch", chipOn, red, "", noColor},
			{"CurSearch", chipOn, red, "", noColor},
			{"Substitute", chipOn, red, "", noColor},
			{"Todo", chipOn, orange, "bold", noColor},
			{"QuickFixLine", noColor, border, "", noColor},
		},
	}, {
		"The chrome", []hl{
			{"StatusLine", ink, border, "", noColor},
			{"StatusLineNC", gray, lift, "", noColor},
			{"TabLine", gray, lift, "", noColor},
			{"TabLineSel", ink, border, "bold", noColor},
			{"TabLineFill", noColor, lift, "", noColor},
			{"WinBar", ink, noColor, "bold", noColor},
			{"WinBarNC", gray, noColor, "", noColor},
			{"Pmenu", ink, lift, "", noColor},
			{"PmenuSel", ink, border, "bold", noColor},
			{"PmenuSbar", noColor, lift, "", noColor},
			{"PmenuThumb", noColor, gray, "", noColor},
			{"WildMenu", ink, border, "bold", noColor},
			{"MsgArea", ink, noColor, "", noColor},
			{"ModeMsg", gray, noColor, "", noColor},
			{"MoreMsg", green, noColor, "", noColor},
			{"Question", cyan, noColor, "", noColor},
			{"ErrorMsg", red, noColor, "", noColor},
			{"WarningMsg", yellow, noColor, "", noColor},
		},
	}, {
		"The words: what a thing is, by the slot it is asked for by name", []hl{
			{"Comment", gray, noColor, "", noColor},
			{"Constant", yellow, noColor, "", noColor},
			{"String", green, noColor, "", noColor},
			{"Character", green, noColor, "", noColor},
			{"Number", orange, noColor, "", noColor},
			{"Float", orange, noColor, "", noColor},
			{"Boolean", yellow, noColor, "", noColor},
			{"Identifier", varc, noColor, "", noColor},
			{"Function", magenta, noColor, "", noColor},
			{"Statement", blue, noColor, "", noColor},
			{"Conditional", blue, noColor, "", noColor},
			{"Repeat", blue, noColor, "", noColor},
			{"Label", blue, noColor, "", noColor},
			{"Operator", op, noColor, "", noColor},
			{"Keyword", blue, noColor, "", noColor},
			{"Exception", red, noColor, "", noColor},
			{"PreProc", magenta, noColor, "", noColor},
			{"Include", magenta, noColor, "", noColor},
			{"Define", magenta, noColor, "", noColor},
			{"Macro", magenta, noColor, "", noColor},
			{"PreCondit", magenta, noColor, "", noColor},
			{"Type", cyan, noColor, "", noColor},
			{"StorageClass", cyan, noColor, "", noColor},
			{"Structure", cyan, noColor, "", noColor},
			{"Typedef", cyan, noColor, "", noColor},
			{"Special", parchment, noColor, "", noColor},
			{"SpecialChar", cyan, noColor, "", noColor},
			{"SpecialKey", faint, noColor, "", noColor},
			{"Tag", blue, noColor, "", noColor},
			{"Delimiter", parchment, noColor, "", noColor},
			{"SpecialComment", parchment, noColor, "", noColor},
			{"Debug", orange, noColor, "", noColor},
			{"Underlined", blue, noColor, "underline", noColor},
			{"Ignore", faint, noColor, "", noColor},
			{"Error", red, noColor, "", noColor},
		},
	}, {
		"A diff, laid on the ground, as conn lays one anywhere", []hl{
			{"DiffAdd", noColor, added, "", noColor},
			{"DiffDelete", faint, removed, "", noColor},
			{"DiffChange", noColor, changed, "", noColor},
			{"DiffText", noColor, addedWord, "", noColor},
			{"diffAdded", green, noColor, "", noColor},
			{"diffRemoved", red, noColor, "", noColor},
			{"diffChanged", yellow, noColor, "", noColor},
			{"diffFile", parchment, noColor, "bold", noColor},
			{"diffLine", gray, noColor, "", noColor},
			{"Added", noColor, added, "", noColor},
			{"Removed", noColor, removedW, "", noColor},
			{"Changed", noColor, changed, "", noColor},
		},
	}, {
		"What is wrong: its own scale, apart from the words", []hl{
			{"DiagnosticError", red, noColor, "", noColor},
			{"DiagnosticWarn", yellow, noColor, "", noColor},
			{"DiagnosticInfo", blue, noColor, "", noColor},
			{"DiagnosticHint", cyan, noColor, "", noColor},
			{"DiagnosticOk", green, noColor, "", noColor},
			{"DiagnosticUnderlineError", noColor, noColor, "undercurl", red},
			{"DiagnosticUnderlineWarn", noColor, noColor, "undercurl", yellow},
			{"DiagnosticUnderlineInfo", noColor, noColor, "undercurl", blue},
			{"DiagnosticUnderlineHint", noColor, noColor, "undercurl", cyan},
			{"DiagnosticVirtualTextError", red, noColor, "", noColor},
			{"DiagnosticVirtualTextWarn", yellow, noColor, "", noColor},
			{"DiagnosticVirtualTextInfo", faint, noColor, "", noColor},
			{"DiagnosticVirtualTextHint", faint, noColor, "", noColor},
			{"SpellBad", noColor, noColor, "undercurl", red},
			{"SpellCap", noColor, noColor, "undercurl", yellow},
			{"SpellRare", noColor, noColor, "undercurl", magenta},
			{"SpellLocal", noColor, noColor, "undercurl", cyan},
		},
	}, {
		"What the language server has to say", []hl{
			{"LspReferenceText", noColor, border, "", noColor},
			{"LspReferenceRead", noColor, border, "", noColor},
			{"LspReferenceWrite", noColor, border, "", noColor},
			{"LspSignatureActiveParameter", param, noColor, "bold", noColor},
			{"LspInlayHint", faint, noColor, "", noColor},
			{"LspCodeLens", faint, noColor, "", noColor},
		},
	}, {
		"Tree-sitter, where a call is told from a definition and a\n\" parameter from a variable", []hl{
			{"@comment", gray, noColor, "", noColor},
			{"@string", green, noColor, "", noColor},
			{"@string.escape", cyan, noColor, "", noColor},
			{"@string.special", cyan, noColor, "", noColor},
			{"@character", green, noColor, "", noColor},
			{"@number", orange, noColor, "", noColor},
			{"@boolean", yellow, noColor, "", noColor},
			{"@float", orange, noColor, "", noColor},
			{"@constant", yellow, noColor, "", noColor},
			{"@constant.builtin", orange, noColor, "", noColor},
			{"@constant.macro", magenta, noColor, "", noColor},
			{"@variable", varc, noColor, "", noColor},
			{"@variable.builtin", orange, noColor, "", noColor},
			{"@variable.parameter", param, noColor, "", noColor},
			{"@variable.member", param, noColor, "", noColor},
			{"@property", param, noColor, "", noColor},
			{"@field", param, noColor, "", noColor},
			{"@function", magenta, noColor, "", noColor},
			{"@function.builtin", magenta, noColor, "", noColor},
			{"@function.call", call, noColor, "", noColor},
			{"@function.method", magenta, noColor, "", noColor},
			{"@function.method.call", call, noColor, "", noColor},
			{"@constructor", magenta, noColor, "", noColor},
			{"@keyword", blue, noColor, "", noColor},
			{"@keyword.function", blue, noColor, "", noColor},
			{"@keyword.return", blue, noColor, "", noColor},
			{"@keyword.operator", op, noColor, "", noColor},
			{"@keyword.exception", red, noColor, "", noColor},
			{"@operator", op, noColor, "", noColor},
			{"@type", cyan, noColor, "", noColor},
			{"@type.builtin", cyan, noColor, "", noColor},
			{"@type.definition", cyan, noColor, "", noColor},
			{"@attribute", magenta, noColor, "", noColor},
			{"@module", cyan, noColor, "", noColor},
			{"@namespace", cyan, noColor, "", noColor},
			{"@label", blue, noColor, "", noColor},
			{"@punctuation.delimiter", parchment, noColor, "", noColor},
			{"@punctuation.bracket", parchment, noColor, "", noColor},
			{"@punctuation.special", op, noColor, "", noColor},
			{"@tag", blue, noColor, "", noColor},
			{"@tag.attribute", param, noColor, "", noColor},
			{"@tag.delimiter", parchment, noColor, "", noColor},
			{"@markup.heading", orange, noColor, "bold", noColor},
			{"@markup.link", blue, noColor, "", noColor},
			{"@markup.link.url", op, noColor, "underline", noColor},
			{"@markup.raw", green, noColor, "", noColor},
			{"@markup.list", parchment, noColor, "", noColor},
			{"@markup.strong", ink, noColor, "bold", noColor},
			{"@markup.italic", ink, noColor, "italic", noColor},
			{"@diff.plus", green, noColor, "", noColor},
			{"@diff.minus", red, noColor, "", noColor},
		},
	}}

	var b strings.Builder
	b.WriteString(`" conn.vim -- nvim in conn, in conn's own scheme.
" Written by ` + "`conn theme vim`" + `; edits do not keep.
"
" conn dresses the terminal it holds: every pane on the ground, in the
" ink, with the sixteen colors a program asks for by name. nvim asks for
" none of them -- it draws in its own hex -- so here is the same table
" again, as a colorscheme. Each color carries the slot it is as well, so
" this holds up where sixteen is all there is; a ground that is no slot
" says NONE and takes the pane's own, which in conn is the ground.
"
" conn picks one ground - dark or light - when a server rises, and holds
" it for that server's life; it does not follow the system after.

set background=` + vimBackground + `
hi clear
if exists('syntax_on')
  syntax reset
endif
let g:colors_name = 'conn'
`)
	for _, s := range sections {
		fmt.Fprintf(&b, "\n\" %s\n", s.title)
		for _, h := range s.hls {
			b.WriteString(h.line() + "\n")
		}
	}
	b.WriteString("\n\" A terminal opened in nvim gets the sixteen it would have had in a\n\" pane of the server.\n")
	for i, c := range scheme {
		fmt.Fprintf(&b, "let g:terminal_color_%d = '%s'\n", i, c)
	}
	return b.String()
}

// slotColor is one of the sixteen, and knows which one it is.
func slotColor(i int) vimColor {
	return vimColor{scheme[i], strconv.Itoa(i)}
}

// borderColor is the border: a pane's edge, a selection, the band
// behind the status line. On dark it is slot 0, the darkest thing there
// is and so the quietest edge. On light it cannot be: light's slot 0 is
// black, which is what a program writing ANSI-0 means by ordinary text,
// and a status line drawn on it was a black bar across a pale page. The
// border on light is a color no slot has a name for — see mode.go — so
// it is written out and says NONE for a terminal with only sixteen,
// which takes the pane's own ground there rather than the wrong one.
func borderColor() vimColor {
	if darkMode {
		return slotColor(0)
	}
	return vimColor{borderHex, "NONE"}
}

// line is the highlight as a colorscheme writes it.
func (h hl) line() string {
	parts := []string{"hi", h.group}
	add := func(gui, cterm string, c vimColor) {
		if c == (vimColor{}) {
			parts = append(parts, gui+"=NONE", cterm+"=NONE")
			return
		}
		parts = append(parts, gui+"="+c.gui, cterm+"="+c.cterm)
	}
	add("guifg", "ctermfg", h.fg)
	add("guibg", "ctermbg", h.bg)
	attr := h.attr
	if attr == "" {
		attr = "NONE"
	}
	parts = append(parts, "gui="+attr, "cterm="+attr)
	if h.sp != (vimColor{}) {
		parts = append(parts, "guisp="+h.sp.gui)
	}
	return strings.Join(parts, " ")
}

// writeVimColorscheme writes the colorscheme where nvim looks for one,
// and answers the path it wrote.
func writeVimColorscheme(home string) (string, error) {
	dir := filepath.Join(configHome(home), "nvim", "colors")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "conn.vim")
	return path, os.WriteFile(path, []byte(vimColorscheme()), 0o644)
}

// refreshVimColorscheme rewrites the colorscheme if it has already been
// written once, so a server that settles on a ground never leaves the
// file behind on the ground it was last written under. It writes
// nothing where `conn theme vim` has never run — that command is still
// what puts the file there the first time.
//
// An nvim already open keeps the colors it loaded; nvim reads a
// colorscheme once. The next one started in conn takes the new ground,
// which is what a ground that is fixed for a server's life needs: the
// ground changes when the server does, and the editors opened after it
// are the ones there are.
func refreshVimColorscheme(home string) {
	path := filepath.Join(configHome(home), "nvim", "colors", "conn.vim")
	if _, err := os.Stat(path); err != nil {
		return
	}
	_, _ = writeVimColorscheme(home)
}

// dressVim writes the colorscheme and says how nvim is to reach for it,
// which is nvim's own business and the user's: conn writes the colors.
func dressVim(home string) (string, bool) {
	path, err := writeVimColorscheme(home)
	if err != nil {
		return fmt.Sprintf("conn theme: %v\n", err), false
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Wrote conn's colorscheme for nvim to %s\n", tilde(path, home))
	b.WriteString("nvim takes it with `colorscheme conn`. CONN is set in the server, so a\n")
	b.WriteString("config can reach for it in conn and keep its own everywhere else.\n")
	return b.String(), true
}
