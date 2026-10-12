package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/w0zro/conn/internal/tmux"
)

// / searches what a project's processes have said: every line the panes
// conn holds in it still keep, newest first, under the name of the row
// each pane is. enter goes to the pane in tmux's own copy mode, on the
// match, the search in hand, so n and N go on from there as in any
// pane. A capital in what is typed makes the search mind case, as tmux
// does.

// A written is one pane's lines, as read for the search: the row it is,
// by name, and its lines as copy mode counts them, unjoined.
type written struct {
	pane  pane
	name  string
	lines []string
}

// A hit is a line that says what was searched for: which pane, the
// line, where in it the text begins, and which saying of the text it
// is, counted back from the pane's end, which is what the pane is told
// to land on (see tmux.Land).
type hit struct {
	said int
	line string
	at   int
	k    int
}

// hits is the lines that say the text: the panes in their order, each
// pane's lines newest first. Nothing typed finds nothing.
func hits(text string, panes []written) []hit {
	if text == "" {
		return nil
	}
	fold := strings.ToLower(text) == text
	var out []hit
	for i, p := range panes {
		k := 0
		for j := len(p.lines) - 1; j >= 0; j-- {
			at := indexFold(p.lines[j], text, fold)
			if at < 0 {
				continue
			}
			// A line that says the text more than once is one hit, on
			// its first saying, which is the last a search back reaches.
			n := sayings(p.lines[j], text, fold)
			out = append(out, hit{said: i, line: p.lines[j], at: at, k: k + n - 1})
			k += n
		}
	}
	return out
}

// sayings is how many times a line says the text, counted as tmux's
// search counts: every place it begins.
func sayings(line, text string, fold bool) int {
	if fold {
		line = strings.ToLower(line)
	}
	n := 0
	for i := 0; i+len(text) <= len(line); i++ {
		if line[i:i+len(text)] == text {
			n++
		}
	}
	return n
}

// indexFold is where text begins in line, or -1: as typed, or with case
// ignored where what was typed has no capital in it.
func indexFold(line, text string, fold bool) int {
	if !fold {
		return strings.Index(line, text)
	}
	return strings.Index(strings.ToLower(line), text)
}

// saidRead is the panes' lines, read off the loop.
type saidRead struct {
	root  string
	panes []written
}

// readSaid reads the lines of the panes of a project's rows.
func readSaid(srv *tmux.Server, root string, rows []row) tea.Cmd {
	type named struct {
		p    pane
		name string
	}
	var ps []named
	for _, r := range rows {
		if r.pane == nil {
			continue
		}
		name := paneName(*r.pane)
		if r.conv != nil {
			name = or(r.conv.Title, name)
		} else if r.pane.Session != "" {
			name = "new conversation"
		}
		ps = append(ps, named{*r.pane, name})
	}
	return func() tea.Msg {
		out := saidRead{root: root}
		for _, n := range ps {
			lines, _, err := srv.Scrollback(n.p.ID)
			if err != nil {
				continue
			}
			out.panes = append(out.panes, written{pane: n.p, name: n.name, lines: lines})
		}
		return out
	}
}

// land goes to a hit's pane and puts it in copy mode on the hit.
func land(srv *tmux.Server, p pane, k int, text string) tea.Cmd {
	return func() tea.Msg {
		if err := goTo(srv, p); err != nil {
			return acted(err.Error())
		}
		if err := srv.Land(p.ID, k, text); err != nil {
			return acted(err.Error())
		}
		return acted("")
	}
}

// around is a line fitted to the room with the match in view: the whole
// line where it fits, and otherwise from a little before the match, the
// cut start marked. It answers where the match now begins, in bytes.
func around(line string, at, room int) (string, int) {
	line = strings.TrimRight(line, " ")
	at = min(max(at, 0), len(line))
	if ansi.StringWidth(line) <= room || ansi.StringWidth(line[:at]) < room-16 {
		return ansi.Truncate(line, room, "…"), at
	}
	from := at
	for n := 0; from > 0 && n < 8; n++ {
		from--
		for from > 0 && line[from]&0xC0 == 0x80 {
			from--
		}
	}
	cut := "…" + line[from:]
	return ansi.Truncate(cut, room, "…"), len("…") + at - from
}
