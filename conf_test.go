package main

import (
	"strings"
	"testing"
)

func TestTheConfigurationBindsTheChordsToThisBuild(t *testing.T) {
	// The chords run the binary the launcher was, by its full path, quoted
	// so a directory with a space or a quote in its name still finds it.
	conf := tmuxConf("/opt/my tools/it's/conn", 4242)
	for _, want := range []string{
		"set -g prefix C-Space",
		"unbind -a",
		"set -g mouse on",
		`bind - run-shell "'/opt/my tools/it'\''s/conn' home"`,
		`bind . run-shell "'/opt/my tools/it'\''s/conn' home"`,
		`bind x run-shell "'/opt/my tools/it'\''s/conn' home x"`,
		`bind s run-shell "'/opt/my tools/it'\''s/conn' shell '#{pane_current_path}'"`,
		`bind A run-shell "'/opt/my tools/it'\''s/conn' home A"`,
		`bind , run-shell "'/opt/my tools/it'\''s/conn' kind"`,
		`bind l run-shell "'/opt/my tools/it'\''s/conn' next"`,
		`bind 9 run-shell "'/opt/my tools/it'\''s/conn' nth 9"`,
		`bind C-Space run-shell "'/opt/my tools/it'\''s/conn' back"`,
		`bind ? run-shell "'/opt/my tools/it'\''s/conn' keys '#{client_name}'"`,
		// The finder is the front door: the prefix's p, from any buffer.
		`bind p run-shell "'/opt/my tools/it'\''s/conn' finder '#{client_name}'"`,
		"bind q detach-client",
		"set -g history-limit 4242",
		"set -g automatic-rename off",
		// The home window is conn's tabline over the buffer with focus.
		"set -g main-pane-height 3",
		"set -g pane-border-status bottom",
		`set -g pane-border-format "#{?#{@conn_nav},#{@conn_heading},}"`,
		`set-hook -g window-resized 'if -F "#{@conn_home}" "select-layout main-horizontal"'`,
		// The hangar under every pane, and the chip's hairline between them.
		`set -g window-style "bg=` + tp.ground + `,fg=` + tp.ink + `"`,
		`set -g pane-border-style "fg=` + tp.ground + `,bg=` + tp.ground + `"`,
		`set -g popup-style "bg=` + tp.wash + `,fg=` + tp.ink + `"`,
		`set -g status-left "` + statusLeft() + `"`,
		// The status line begins with conn's name, on the orange, before
		// any mode: it is the one inverted ground on screen.
		`set -g status-left "#[fg=` + tp.ground + `,bg=` + tp.orange + `,bold] CONN #{?client_prefix,`,
		// Then the mode — tmux's own, then the navigator's — and the
		// navigator's message.
		"#{?client_prefix,", "#{?pane_in_mode,",
		"#{?@conn_mode,#{@conn_mode},",
		// The message slot: a chord's note while the clock has not reached
		// its end, else the navigator's message. The clock is an option
		// holding %s, which T: expands.
		"#{?#{e|<:#{T:@conn_now},#{@conn_until}},#{@conn_note},#{@conn_msg}}",
		`set -g @conn_now "%s"`,
		// Each mode is a chip in its color on the chip's ground with the
		// rest of the line washed, the commas escaped for the conditional.
		"#[fg=" + tp.amber + "#,bg=" + tp.chip + "#,bold] PREFIX #[fg=" + tp.gray + "#,bg=" + tp.wash + "#,fill=" + tp.wash + "]",
		"#[fg=" + tp.teal + "#,bg=" + tp.chip + "#,bold] COPY ",
		// The right end is ? keys, and nothing else.
		`set -g status-right "#[fg=` + tp.gray + `,bg=` + tp.wash + `] ? keys "`,
	} {
		if !strings.Contains(conf, want) {
			t.Errorf("the configuration lacks %q", want)
		}
	}
	// tmux keeps the root table for its mouse bindings; conn adds nothing
	// to it, so every key reaches the buffer with focus but the prefix.
	if strings.Contains(conf, "-T root") || strings.Contains(conf, "bind -n") {
		t.Error("the configuration touches the root table by name")
	}
}

func TestTheTerminalThemeLeavesTheShellsGround(t *testing.T) {
	t.Cleanup(func() { applyTheme("") })
	applyTheme("terminal")
	if strings.Contains(tmuxConf("/opt/conn", 100), "window-style") {
		t.Error("theme terminal should leave the shells' ground to the terminal")
	}
	applyTheme("anything else")
	if !strings.Contains(tmuxConf("/opt/conn", 100), "window-style") {
		t.Error("anything else is the hangar")
	}
}
