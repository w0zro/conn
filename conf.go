package main

import (
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// conn is a tmux client. The terminal attaches to conn's private server the
// way any tmux client does, and tmux draws the buffers, holds the prefix and
// keeps the status line; conn is the program in the home window's top pane
// — the tabline, and the views it draws beneath it — and the commands the
// prefix's chords run. The buffer with focus is the pane under the tabline;
// the rest wait in windows of their own. What makes the server conn's rather
// than a stock tmux is this configuration, written at every launch and
// sourced into a server already running, so the bindings are always the
// build's.

// confPath is where the configuration is written: beside the socket, in
// the state directory.
func confPath() string {
	return filepath.Join(filepath.Dir(socketPath()), "tmux.conf")
}

// chromeRows is the height of conn's own pane while a buffer is shown under
// it: the tabline, and the buffer's heading. Over them tmux draws one row
// more, the pane's border status, which carries the orange edge over the
// focused tab; the layout holds all three through every resize.
const chromeRows = 2

// tmuxConf is the configuration for conn's server. conn is the path of this
// build, which the chords run; the path is quoted so a directory with a
// space in its name still finds it.
func tmuxConf(conn string, scrollback int) string {
	exe := shellQuote(conn)
	run := func(args string) string {
		return `run-shell "` + exe + ` ` + args + `"`
	}
	var b strings.Builder
	w := func(lines ...string) {
		for _, l := range lines {
			b.WriteString(l)
			b.WriteByte('\n')
		}
	}

	w("# Written by conn at every launch; edits do not survive one.",
		"",
		"# The keys. ctrl-space is the prefix, and each chord keeps its letter's",
		"# meaning: p and / open the finder, - and . the everything view, j and",
		"# k the next and previous tab, enter the next thing owed, ctrl-space",
		"# again the buffer before this one, x and X the kill preview of the",
		"# buffer with focus, e its environment, s a r t b l a shell, an agent,",
		"# the plan, the tests, the build, the lint in the focused pane's",
		"# directory, and , the next kind of agent. ctrl-p alone opens the",
		"# finder from any buffer: it is the front door. Every chord tmux would",
		"# otherwise bind is unbound first; the root table is left as tmux has",
		"# it, which is the mouse, plus that one key.",
		"set -g prefix C-Space",
		"unbind -a",
		"bind C-Space "+run("back"),
		"bind - "+run("home"),
		"bind . "+run("home"),
		"bind j "+run("next"),
		"bind k "+run("prev"),
		"bind Enter "+run("jump"),
		"bind p "+run("finder '#{client_name}'"),
		"bind C-p "+run("finder '#{client_name}'"),
		"bind / "+run("finder '#{client_name}'"),
		"bind -n C-p "+run("finder '#{client_name}'"),
		"bind ? "+run("keys '#{client_name}'"),
		"bind e "+run("env '#{pane_pid} #{client_name}'"),
		"bind x "+run("home x"),
		"bind X "+run("home X"),
		"bind s "+run("shell '#{pane_current_path}'"),
		"bind a "+run("agent '#{pane_current_path}'"),
		"bind , "+run("kind"),
		"bind r "+run("run '#{pane_current_path}'"),
		"bind t "+run("test '#{pane_current_path}'"),
		"bind b "+run("build '#{pane_current_path}'"),
		"bind l "+run("lint '#{pane_current_path}'"),
		"bind q detach-client",
		`bind R confirm-before -p "end the server, and every buffer it holds? (y/n)" kill-server`,
		"",
		"# The server. The config sets the transcript cap; windows follow the",
		"# client that last spoke; a program's copy reaches the clipboard; the",
		"# terminal's title is the focused pane. tmux handles the mouse, with",
		"# its own bindings: the wheel scrolls a pane's transcript, a drag",
		"# selects and copies, a click focuses the pane under it, and a program",
		"# that speaks mouse gets its own events.",
		"set -g mouse on",
		"set -g history-limit "+strconv.Itoa(scrollback),
		"set -g window-size latest",
		"set -g set-clipboard on",
		"set -g mode-keys vi",
		"set -g automatic-rename off",
		"set -g allow-rename off",
		"set -g set-titles on",
		`set -g set-titles-string "#{?#{@conn_nav},conn,#{?#{@conn_title},#{@conn_title},#{pane_current_command}}}"`,
		"set -g escape-time 10",
		"set -g focus-events on",
		"set -g default-terminal tmux-256color",
		`set -as terminal-features ",*:RGB"`,
		// Claude Code caps itself at 256 colors wherever TMUX is set, whatever
		// TERM and COLORTERM say, and paints the hangar's near-black grounds
		// as the nearest cube color, a saturated teal. This variable is its
		// own way out of the cap; every pane inherits it.
		"set-environment -g CLAUDE_CODE_TMUX_TRUECOLOR 1",
		// What a program says to the terminal around tmux — the progress
		// Claude Code reports while it works, its notification when it is
		// done — reaches it only wrapped for passing through, and only
		// where the server allows the wrapping. Every pane is told the
		// outer terminal's name when it opens (outerTerminal), so a
		// program knows there is one to speak to.
		"set -g allow-passthrough on",
		"set -g display-time 3000",
		// The clock the status line reads to know whether a chord's note
		// is still current: T: expands an option's strftime specifiers,
		// and this option is the one specifier the format needs.
		`set -g `+nowOption+` "%s"`,
		"",
		"# The ground. Every pane sits on the hangar's ground in its ink unless",
		"# the config keeps the terminal's for the shells; the borders are a",
		"# hairline two steps off it, and an overlay sits on the wash inside a",
		"# border of its own.",
	)
	if paintShells {
		w(`set -g window-style "bg=` + tp.ground + `,fg=` + tp.ink + `"`)
	}
	w(`set -g pane-border-style "fg=`+tp.chip+`,bg=`+tp.ground+`"`,
		`set -g pane-active-border-style "fg=`+tp.chip+`,bg=`+tp.ground+`"`,
		"set -g pane-border-indicators off",
		"set -g popup-border-lines single",
		`set -g popup-border-style "fg=`+tp.border+`,bg=`+tp.wash+`"`,
		`set -g popup-style "bg=`+tp.wash+`,fg=`+tp.ink+`"`,
		"",
		"# The home window: conn's tabline across the top at its height, the",
		"# buffer with focus filling the rest. The layout is re-applied on every",
		"# resize, so the buffer takes the window's growth.",
		"set -g main-pane-height "+strconv.Itoa(chromeRows+1),
		// The row over every pane is its border status: over conn's pane
		// it carries the orange edge above the focused tab, on the bar;
		// over a buffer it is the hairline under the heading, drawn long
		// and cut to the pane.
		"set -g pane-border-status top",
		`set -g pane-border-format "#{?#{@conn_nav},#{`+edgeOption+`},#[fg=`+tp.chip+`]`+strings.Repeat("─", 400)+`}"`,
		`set-hook -g window-resized 'if -F "#{@conn_home}" "select-layout main-horizontal"'`,
		"",
		"# The status line: the CONN chip, then one mode chip — the prefix",
		"# while a chord hangs, copy mode, else what the navigator names: an",
		"# answer owed, a run failed, a confirmation waiting, the everything",
		"# view — then what the navigator says, or — for a few seconds, over",
		"# it — what a chord said, and ? keys at the end. Nothing else lives",
		"# here. The window list tmux would draw is turned off: the windows",
		"# are where buffers wait, and the tabline is the list of them.",
		"set -g status on",
		"set -g status-position bottom",
		"set -g status-interval 1",
		"set -g status-justify left",
		"set -g status-left-length 400",
		`set -g status-style "bg=`+tp.wash+`,fg=`+tp.gray+`"`,
		`set -g status-left "`+statusLeft()+`"`,
		`set -g status-right "`+statusRight()+`"`,
		`set -g window-status-separator ""`,
		`set -g window-status-format ""`,
		`set -g window-status-current-format ""`,
		`set -g message-style "bg=`+tp.wash+`,fg=`+tp.amber+`,bold"`,
		`set -g message-command-style "bg=`+tp.wash+`,fg=`+tp.amber+`"`,
		`set -g mode-style "bg=`+tp.chip+`,fg=`+tp.ink+`"`,
	)
	return b.String()
}

// The status line's message slot is shared. The navigator writes msgOption
// and keeps it until its next key; a chord — a different process, done in
// a moment — writes noteOption with untilOption beside it, the second the
// note is stale, and the format shows the note over the message while the
// clock has not reached it. Nothing has to clear the note, and the
// navigator's message is back under it when it goes.
const (
	msgOption   = "@conn_msg"
	modeOption  = "@conn_mode"
	edgeOption  = "@conn_edge"
	noteOption  = "@conn_note"
	untilOption = "@conn_until"
	nowOption   = "@conn_now" // holds %s, so #{T:@conn_now} is the time
)

// noteFor is how long a chord's note stands: display-time's three seconds
// and one more, since the line ticks once a second and a note set late in
// one loses most of it.
const noteFor = 4 * time.Second

// messageSlot is the status line's message slot: a chord's note while one is
// current, else what the navigator said.
func messageSlot() string {
	return "#{?#{e|<:#{T:" + nowOption + "},#{" + untilOption + "}},#{" + noteOption + "},#{" + msgOption + "}}"
}

// statusRight is the status line's end: ? keys, in gray on the wash.
func statusRight() string {
	return "#[fg=" + tp.gray + ",bg=" + tp.wash + "] ? keys "
}

// statusLeft is the status line's format: conn's name, the mode, then the
// message. The name is first and always there — the line is where conn
// says its own. tmux knows two of the modes itself — the prefix, copy mode
// — and the navigator names the rest in @conn_mode, a chip in its color
// with the line washed one tone after it; with no mode to name the wash
// begins at once. The message after the mode is the navigator's, or a
// chord's note over it while the note is fresh.
func statusLeft() string {
	// A chip stands inside a conditional, where a comma would split the
	// alternatives, so the styles' commas are escaped.
	chip := func(color, word string) string {
		return strings.ReplaceAll(statusChip(color, word), ",", "#,")
	}
	wash := "#[fg=" + tp.gray + "#,bg=" + tp.wash + "#,fill=" + tp.wash + "]"
	mode := "#{?client_prefix," + chip(tp.amber, "PREFIX") +
		",#{?pane_in_mode," + chip(tp.teal, "COPY") +
		",#{?" + modeOption + ",#{" + modeOption + "}," + wash + "}}}"
	return brandChip() + mode + messageSlot()
}
