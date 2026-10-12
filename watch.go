package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/fsnotify/fsnotify"
	"github.com/w0zro/conn/internal/tmux"
)

// conn reads the machine when it changes, not on a clock. What it reads
// says when it has changed: Claude Code writes a file for each instance
// as its state changes, and a transcript as a conversation goes; the
// file system says so. tmux says when a window opens or closes, and,
// to a client subscribed to a format, when the format's value changes:
// what a pane runs, where, and how what conn u started ended.

// changed is word that something conn reads has changed.
type changed struct{}

// settle is how long a change is given for the changes that come with
// it, so that a burst of them is read once.
const settle = 150 * time.Millisecond

// watch sends changed to the panel whenever something it reads
// changes, until ctx is done.
func watch(ctx context.Context, send func(tea.Msg), srv *tmux.Server) {
	poke := make(chan struct{}, 1)
	nudge := func() {
		select {
		case poke <- struct{}{}:
		default:
		}
	}
	go watchFiles(ctx, nudge)
	go watchTmux(ctx, nudge, srv)
	for {
		select {
		case <-ctx.Done():
			return
		case <-poke:
		}
		time.Sleep(settle)
		select {
		case <-poke:
		default:
		}
		send(changed{})
	}
}

// watchFiles watches Claude Code's instance files and its transcripts.
// A transcript folder made after the watch begins is watched once the
// file system says it was made.
func watchFiles(ctx context.Context, nudge func()) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	defer func() { _ = w.Close() }()
	projects := filepath.Join(claudeDir(), "projects")
	_ = w.Add(filepath.Join(claudeDir(), "sessions"))
	_ = w.Add(projects)
	if dirs, err := os.ReadDir(projects); err == nil {
		for _, d := range dirs {
			if d.IsDir() {
				_ = w.Add(filepath.Join(projects, d.Name()))
			}
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-w.Events:
			if !ok {
				return
			}
			if e.Has(fsnotify.Create) && filepath.Dir(e.Name) == projects {
				if info, err := os.Stat(e.Name); err == nil && info.IsDir() {
					_ = w.Add(e.Name)
				}
			}
			nudge()
		case _, ok := <-w.Errors:
			if !ok {
				return
			}
		}
	}
}

// listenAfter is when the machine is read again after tmux tells of a
// change, for the ports a server opens once it has started.
var listenAfter = []time.Duration{time.Second, 3 * time.Second, 10 * time.Second}

// paneWatch is the format tmux is asked to say the changes of, for
// every pane: what runs in it, where, whether it has ended, and the
// marks conn's verbs set.
const paneWatch = "#{pane_current_command} #{pane_current_path} #{pane_dead} #{" + exitMark + "} #{" + sessionMark + "}"

// watchTmux keeps a control-mode client on conn's session, which tmux
// tells of windows opened and closed and of every change to paneWatch.
// The client takes no part in the size of the windows and is sent no
// output.
func watchTmux(ctx context.Context, nudge func(), srv *tmux.Server) {
	leftWatching(srv)
	cmd := exec.CommandContext(ctx, srv.Tmux, "-S", srv.Socket, "-C", "attach-session", "-t", sessionName, "-f", "ignore-size,no-output")
	cmd.Env = tmux.WithoutTmux(os.Environ())
	in, err := cmd.StdinPipe()
	if err != nil {
		return
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	if err := cmd.Start(); err != nil {
		return
	}
	defer func() { _ = cmd.Wait() }()
	// Quoted: unquoted, tmux reads the # of a format as a comment.
	if _, err := fmt.Fprintf(in, "refresh-client -B 'conn:%%*:%s'\n", paneWatch); err != nil {
		return
	}
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		if tells(sc.Text()) {
			nudge()
			// A server listens a while after it starts, and nothing
			// says when: what a pane runs having changed, the machine is
			// read again after it has had time to.
			for _, d := range listenAfter {
				time.AfterFunc(d, nudge)
			}
		}
	}
}

// tells says whether a line from a control-mode client is tmux telling
// of a change, rather than answering a command or passing on output.
func tells(line string) bool {
	if !strings.HasPrefix(line, "%") {
		return false
	}
	word, _, _ := strings.Cut(line, " ")
	switch word {
	case "%begin", "%end", "%error", "%output", "%extended-output", "%exit":
		return false
	}
	return true
}
