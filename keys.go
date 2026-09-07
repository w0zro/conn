package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// The keys, spelled out. They are asked for with ? — at the navigator, or
// with the prefix from any shell — and answer in a tmux popup over the
// whole window, whichever pane had focus: the navigator's own pane
// is a column, and a page needs the width. The popup runs this build as
// `conn page`, which draws the page, waits for a keystroke and goes.

// keyList is every key, in the order a reader wants them: the navigator's
// first, then the chords.
var keyList = [][2]string{
	{"↑↓ j k", "move"},
	{"J K", "next · previous shell"},
	{"enter", "open"},
	{"tab", "the next thing that needs you"},
	{"shift+tab", "back: the previous shell, or the list"},
	{"s", "shell"},
	{"a", "agent"},
	{"A", "continue a conversation"},
	{",", "the next kind of agent"},
	{"n", "new project"},
	{"r", "run"},
	{"t · b · l", "test · build · lint"},
	{"x · X", "kill · kill the tree"},
	{"/", "find a project · a process"},
	{"esc", "clear the filter"},
	{"space · -", "fold · unfold all"},
	{".", "all · running"},
	{"gg · G", "top · bottom"},
	{"U", "update conn"},
	{"R", "end the server, shells and all"},
	{"q", "leave; the shells keep running"},
	{"^spc -", "the navigator, from any shell"},
	{"^spc j k", "next · previous shell"},
	{"^spc ^spc", "back: the previous shell, or the list"},
	{"^spc enter", "the next thing that needs you"},
	{"^spc s a r t b l A", "shell · agent · run · test · build · lint, here · continue"},
	{"^spc ,", "the next kind of agent"},
	{"^spc /", "find from anywhere"},
	{"^spc q", "leave from anywhere"},
	{"^spc R", "end the server from anywhere"},
	{"^spc ?", "this"},
}

// keysPage is the page: a blank row, the keys under each other with their
// meanings aligned, a blank row. tmux draws the border around it.
func keysPage() []string {
	var keyw, descw int
	for _, k := range keyList {
		keyw = max(keyw, lipgloss.Width(k[0]))
		descw = max(descw, lipgloss.Width(k[1]))
	}
	lines := []string{""}
	for _, k := range keyList {
		lines = append(lines, " "+pad(itemStyle.Render(k[0]), keyw)+"  "+pad(hintStyle.Render(k[1]), descw)+" ")
	}
	return append(lines, "")
}

// showKeys shows the page in a popup over the client: sized to the page,
// or to the client when the client is smaller — tmux rejects a popup it
// cannot fit rather than cutting it — titled, and closing when the page
// does. The client has to be named: a command from outside tmux has none,
// and a popup with no client has no size to fit. A chord names the client
// that pressed it; the navigator, given none, takes the one that spoke
// last. exe is this build, quoted for the shell tmux runs the page under.
func showKeys(run runner, exe, client string) error {
	page := keysPage()
	width := 0
	for _, l := range page {
		width = max(width, lipgloss.Width(l))
	}
	return popup(run, client, " keys ", width+2, len(page)+2, shellQuote(exe)+" page")
}

// popup runs a command in a popup over the client, sized as asked or to
// the client when the client is smaller — tmux rejects a popup it cannot
// fit rather than cutting it — titled, and closing when the command
// does. A client of "" is the one that spoke last.
func popup(run runner, client, title string, width, height int, command string) error {
	if client == "" {
		var err error
		if client, err = latestClient(run); err != nil {
			return err
		}
	}
	if out, err := run("display-message", "-p", "-c", client, "#{client_width} #{client_height}"); err == nil {
		if f := strings.Fields(out); len(f) == 2 {
			if cw, err := strconv.Atoi(f[0]); err == nil && cw > 0 {
				width = min(width, cw)
			}
			if ch, err := strconv.Atoi(f[1]); err == nil && ch > 0 {
				height = min(height, ch)
			}
		}
	}
	_, err := run("display-popup", "-E", "-c", client, "-T", title,
		"-w", strconv.Itoa(width), "-h", strconv.Itoa(height), command)
	return err
}

// latestClient is the client that spoke last: the one a popup goes over
// when nothing named one.
func latestClient(run runner) (string, error) {
	out, err := run("list-clients", "-t", tmuxSession, "-F", "#{client_activity}\t#{client_name}")
	if err != nil {
		return "", err
	}
	latest, client := -1, ""
	for line := range strings.SplitSeq(out, "\n") {
		when, name, ok := strings.Cut(line, "\t")
		if t, err := strconv.Atoi(when); ok && err == nil && t > latest {
			latest, client = t, name
		}
	}
	if client == "" {
		return "", errors.New("no client to show it on")
	}
	return client, nil
}

// keysModel is the page as a program: drawn once, gone on the first key.
type keysModel struct{}

func (keysModel) Init() tea.Cmd { return nil }

func (k keysModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case tea.KeyPressMsg, tea.PasteMsg:
		return k, tea.Quit
	}
	return k, nil
}

func (keysModel) View() tea.View {
	v := tea.NewView(strings.Join(keysPage(), "\n"))
	v.AltScreen = true
	return v
}

// runKeys is `conn page`: the page, in the popup.
func runKeys() {
	if _, err := tea.NewProgram(keysModel{}).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "conn: %v\n", err)
		os.Exit(1)
	}
}
