package main

import (
	"fmt"
	"strings"

	"github.com/w0zro/conn/internal/room"
	"github.com/w0zro/conn/internal/theme"
	"github.com/w0zro/conn/internal/tmux"
	"github.com/w0zro/conn/internal/work/claude"
)

// What the server wears, and how the station's line is worded in it.
// tmux writes a style down; which words go on the line, and in which
// of the theme's colors, is conn's, and is decided here.

// dressOf is the server's dress on a ground: the panes on the ground
// in the ink, the cursor in the accent, a selection on the border, the
// line on the border in the gray, and in copy mode the band's block and
// tmux's own keys on the bar.
func dressOf(g theme.Ground) room.Dress {
	return room.Dress{
		Ground: theme.Hex(g.Ground), Ink: theme.Hex(g.Ink), Accent: g.Accent,
		Border: g.Border, Gray: g.Gray, Surface: g.Surface,
		Scheme:   g.Scheme[:],
		CopyBand: statusBlock("COPY", g),
		CopyBar:  keyBar(copyHints, g),
	}
}

// serverConf is the server's configuration on a ground, with the panel
// key the operator set.
func serverConf(g theme.Ground) string {
	return room.Conf(room.PanelKey(), dressOf(g), claude.Env)
}

// attach puts this terminal on the server, bringing it up if it is
// down, and answers how the client exited.
//
// The server's mode - its theme, and the ground it is on - is what a
// mode file beside the socket says, or the terminal's own ground in
// conn's own theme the first time a server rises, written down so it
// holds across attaches. A flag says it instead, and says it whenever
// it is given: a server already up is put on the other ground, or in
// the other theme, where it stands, rather than keeping what it rose
// in until conn down. Claude Code and vim are dressed to match before
// anything in the server draws.
func attach(srv *room.Server, self, home string, o theme.Override) (int, error) {
	have, ok := theme.ReadModeFile(srv.Socket)
	want, asked := have, false
	switch {
	case !ok:
		want = theme.AskMode(o, home)
		_ = theme.WriteMode(srv.Socket, want)
	case o.Over(have) != have:
		want = o.Over(have)
		_ = theme.WriteMode(srv.Socket, want)
		asked = true
	}
	g := want.Wear()
	theme.RefreshClaudeTheme(home, g)
	theme.RefreshVimColorscheme(home, g)
	bg := ""
	if asked {
		bg = g.Surface
	}
	// The terminal takes black and the ink for its own before the
	// client has it, so the padding around the client is the terminal's
	// own edge, on either ground. The conn in the pane asks tmux for
	// the pane's own ground, which tmux keeps to the pane; the terminal
	// outside hears it from here. When the client is gone the terminal
	// is ours again, and the colors go back to its own.
	fmt.Print(oscColors(g))
	defer fmt.Print(oscOwnColors)
	return srv.Attach(self, home, serverConf(g), bg)
}

// oscColors asks the terminal to take the ink of a ground and a ground
// for its own, and oscOwnColors gives it its own back. What the terminal paints
// with the ground is the padding around the client, and the padding
// meets the panel and the key bar. It is black, and not the surface:
// the terminal's own edge, the same on every theme and on either
// ground, with the frame standing on it. tmux
// keeps what the conn in a pane asks for to the pane, so the terminal
// outside hears it from the conn that attached, which is also there to
// take it back. The cursor is the other way about: tmux does put the
// server's on the terminal, and leaves it there when the client goes,
// so conn asks for nothing and takes it back all the same.
func oscColors(g theme.Ground) string {
	return fmt.Sprintf("\x1b]10;%s\x1b\\\x1b]11;%s\x1b\\", theme.Hex(g.Ink), paddingHex)
}

// paddingHex is what the terminal is asked to paint around the client:
// black, on either ground.
const paddingHex = "#000000"

const oscOwnColors = "\x1b]110\x1b\\\x1b]111\x1b\\\x1b]112\x1b\\"

// copyHints is what the bar says while a pane is in copy mode: tmux's
// own keys in vi mode, which is the mode conn sets. The words have no
// comma in them, since the bar is written inside a conditional of the
// format's own and a comma there is the conditional's.
var copyHints = []keyHint{{"n N", "Next and previous"}, {"/ ?", "Search down and up"}, {"v y", "Select and copy"}, {"q", "Leave"}}

// keyBar is the hints as the bar writes them on a ground: each key in
// the ink and bold, what it does in the gray after it and in the lower
// case, so the key is the one thing that stands up in the row; three
// cells between one and the next, a cell in from the edge, on the
// surface, which is the bar's ground.
func keyBar(hints []keyHint, g theme.Ground) string {
	var b strings.Builder
	b.WriteString(" ")
	for i, h := range hints {
		if i > 0 {
			b.WriteString("   ")
		}
		b.WriteString(tmux.Styled(tmux.Style{FG: theme.Hex(g.Ink), BG: g.Surface, Bold: true}, h.key+" "))
		b.WriteString(tmux.Styled(tmux.Style{FG: g.Gray, BG: g.Surface}, strings.ToLower(h.does)))
	}
	return b.String()
}

// statusBlock is a mode as the status line wears it: the ground
// knocked out of a block of the orange, flush to the edge, the word
// keeping its own space inside. The ground and not a fixed white, so it
// inverts with everything else — the orange on paper is a dark brick,
// and black would go out on it.
//
// One color for every mode. The orange is "you, here" everywhere else
// in conn — the cursor is drawn in it, and so is the kind of the row
// the bay holds — and where the keys are is the same fact about the
// same operator. A color apiece was tried: copy mode in the blue, the
// question in the waiting color, the view in a teal. It made four
// colors the eye had to learn and then read, in a position whose whole
// job is to be seen rather than read, and the word in the block says
// which mode it is more plainly than a hue ever did. What the position
// has to carry is lit or dark, and the word answers the rest.
func statusBlock(word string, g theme.Ground) string {
	if word == "" {
		return ""
	}
	return tmux.Styled(tmux.Style{FG: theme.Hex(g.Ground), BG: g.Accent, Bold: true}, " "+word+" ")
}

// statusSay is what conn says on the key bar in words, a question
// armed: on the bar's own ground, the surface, in the parchment conn
// titles with, one space in where the keys begin.
func statusSay(text string, g theme.Ground) string {
	return tmux.Styled(tmux.Style{FG: g.Parchment, BG: g.Surface}, " "+text)
}

// statusWord is a word on the line's own ground: the wordmark in the
// ink and bold, or a figure in the gray.
func statusWord(text, color string, bold bool, g theme.Ground) string {
	return tmux.Styled(tmux.Style{FG: color, BG: g.Border, Bold: bold}, text)
}
