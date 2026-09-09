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

// The keys, spelled out. They are asked for with ? — at conn, or with the
// prefix from any buffer — and answer in a tmux popup over the whole
// window, whichever pane had focus. The popup runs this build as `conn
// page`, which draws the page, waits for a keystroke and goes. The
// footnotes at the foot of every view teach the few that matter there;
// this is the whole of them.

// keyList is every key, in the order a reader wants them: conn's own
// first, then the chords.
var keyList = [][2]string{
	{"↑↓ j k", "move"},
	{"J K", "next · previous buffer"},
	{"enter", "open the buffer"},
	{"tab", "the next thing owed"},
	{"shift+tab", "back: the previous buffer, or everything"},
	{"p ^p", "the finder: open, start, or make a project"},
	{"/", "narrow everything by anything a row says"},
	{"esc", "clear the filter · close everything"},
	{".", "everything · running · all"},
	{"space · -", "fold · unfold all"},
	{"gg · G", "top · bottom"},
	{"s", "shell"},
	{"a", "agent"},
	{",", "the next kind of agent"},
	{"r", "run a dead task again in place · run the plan"},
	{"t · b · l", "test · build · lint"},
	{"x · X", "preview a kill · of the tree"},
	{"e", "the environment, annotated"},
	{"gf", "open the last file:line in your editor"},
	{"U", "update conn"},
	{"R", "end the server, buffers and all"},
	{"q", "close a dead buffer · leave; the buffers keep running"},
	{"^p", "the finder, from any buffer"},
	{"^spc p /", "the finder"},
	{"^spc - .", "everything, from any buffer"},
	{"^spc j k", "next · previous buffer"},
	{"^spc ^spc", "back: the previous buffer, or everything"},
	{"^spc enter", "the next thing owed"},
	{"^spc x X", "preview a kill of this buffer · of its tree"},
	{"^spc s a r t b l", "shell · agent · run · test · build · lint, here"},
	{"^spc ,", "the next kind of agent"},
	{"^spc e", "the environment of the pane's process, annotated"},
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
// that pressed it; conn, given none, takes the one that spoke last. exe is
// this build, quoted for the shell tmux runs the page under.
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
// does. A client of "" is the one that spoke last. env is variables for
// the command, name=value each, over what the server would give it.
func popup(run runner, client, title string, width, height int, command string, env ...string) error {
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
	args := []string{"display-popup", "-E", "-c", client, "-T", title,
		"-w", strconv.Itoa(width), "-h", strconv.Itoa(height)}
	for _, kv := range env {
		args = append(args, "-e", kv)
	}
	_, err := run(append(args, command)...)
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
