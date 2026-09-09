package main

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// An overlay — the finder, the environment — sits over the window: the
// workspace behind it dimmed to a shadow of itself, and over that a box on
// the wash inside a border, exactly as tall as what it has to say. tmux's
// popup is the canvas: borderless, the window's whole width and height
// but the status line, so the page draws the backdrop and the box itself
// rather than living inside a frame tmux drew.

// overlayShare is the box's share of the window's width.
const overlayShare = 85

// backdrop is the window behind an overlay, as the page draws it: every
// pane's contents where the pane is, and nothing else, all in faint —
// what is there, readable as context and not competing with the box.
func backdrop(run runner, client string, width, height int) []string {
	rows := make([]string, height)
	out, err := run("display-message", "-p", "-c", client, "#{window_id}")
	if err != nil {
		return rows
	}
	win := strings.TrimSpace(out)
	out, err = run("list-panes", "-t", win, "-F", "#{pane_id}\t#{pane_top}\t#{pane_left}\t#{pane_width}\t#{pane_height}")
	if err != nil {
		return rows
	}
	for line := range strings.SplitSeq(out, "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 5 {
			continue
		}
		top, _ := strconv.Atoi(f[1])
		left, _ := strconv.Atoi(f[2])
		w, _ := strconv.Atoi(f[3])
		h, _ := strconv.Atoi(f[4])
		text, err := run("capture-pane", "-p", "-t", f[0])
		if err != nil {
			continue
		}
		for i, l := range strings.Split(text, "\n") {
			if i >= h || top+i >= height || top+i < 0 {
				break
			}
			l = truncateTail(strings.TrimRight(ansi.Strip(l), " "), w)
			row := rows[top+i]
			row = pad(row, left)
			rows[top+i] = pad(row+l, left+w)
		}
	}
	for i := range rows {
		rows[i] = truncateTail(rows[i], width)
	}
	return rows
}

// overlay draws the page: the backdrop in faint on the ground, and the
// box over it from its second row — the border in the overlay's edge
// color, the inside on the wash. The box's lines are its content, each
// already as wide as the box's inside; the box is as tall as they are.
func overlay(bg []string, box []string, width, height, boxWidth int) string {
	boxWidth = min(boxWidth, width-2)
	left := (width - boxWidth) / 2
	inside := boxWidth - 2
	wash := lipgloss.NewStyle().Background(lipgloss.Color(colorWash))
	edge := wash.Inherit(lipgloss.NewStyle().Foreground(lipgloss.Color(colorBorder)))
	var frame []string
	frame = append(frame, edge.Render("┌"+strings.Repeat("─", inside)+"┐"))
	for _, l := range box {
		frame = append(frame, edge.Render("│")+wash.Render(pad(truncateStyled(l, inside, false), inside))+edge.Render("│"))
	}
	frame = append(frame, edge.Render("└"+strings.Repeat("─", inside)+"┘"))

	top := 1
	if len(frame) > height-top {
		top = max(height-len(frame), 0)
	}
	lines := make([]string, height)
	for i := range lines {
		row := ""
		if i < len(bg) {
			row = bg[i]
		}
		if j := i - top; j >= 0 && j < len(frame) {
			before, _ := cutColumns(row, left)
			before = pad(before, left)
			after := ""
			if lipgloss.Width(row) > left+boxWidth {
				_, after = cutColumns(row, left+boxWidth)
			}
			lines[i] = groundStyle.Inherit(faintStyle).Render(before) + frame[j] +
				groundStyle.Inherit(faintStyle).Render(pad(after, width-left-boxWidth))
			continue
		}
		lines[i] = groundStyle.Inherit(faintStyle).Render(pad(truncateTail(row, width), width))
	}
	return strings.Join(lines, "\n") + ansi.ResetStyle
}

// popupOver runs a page in a popup over the client's whole window — every
// row but the status line, borderless, on the ground — for the page to
// draw its own backdrop and box. client "" is the one that spoke last.
func popupOver(run runner, client, command string, env ...string) error {
	if client == "" {
		var err error
		if client, err = latestClient(run); err != nil {
			return err
		}
	}
	height := 24
	if out, err := run("display-message", "-p", "-c", client, "#{client_height}"); err == nil {
		if f := strings.Fields(out); len(f) > 0 {
			if ch, err := strconv.Atoi(f[len(f)-1]); err == nil && ch > 1 {
				height = ch - 1
			}
		}
	}
	args := []string{"display-popup", "-B", "-E", "-c", client, "-x", "0", "-y", "0",
		"-w", "100%", "-h", strconv.Itoa(height)}
	for _, kv := range env {
		args = append(args, "-e", kv)
	}
	_, err := run(append(args, command+" "+shellQuote(client))...)
	return err
}
