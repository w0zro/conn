package main

import (
	"bufio"
	"context"
	"io"
	"os/exec"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Docker is a source of its own, like the tmux server: the containers are
// not read with the process list but beside it, and merged into the tree
// as they arrive. What docker last said stands in every scan for
// nothing, so a docker that is slow — a desktop's machine resuming from
// its pause — costs the first paint nothing, and a docker that is down
// costs the scan nothing either. And docker is asked when something has
// happened rather than on a clock: its own stream of events says when a
// container starts, stops or dies, and each says to ask again; a slow
// heartbeat covers an event missed and a daemon that came up after conn
// did. A stream that ends — the daemon gone — is opened again, with a
// wait between tries.

// dockerMsg carries what docker says of its containers, and whether it
// said it in time: stalled means the list is what it last said.
type dockerMsg struct {
	containers []Proc
	stalled    bool
}

// dockerReadyMsg carries the feed, once it is running.
type dockerReadyMsg struct{ feed *dockerFeed }

// dockerFeed is the subscription: a goroutine reading docker's events and
// answering each, a heartbeat, and a poke with a list, on msgs.
type dockerFeed struct {
	msgs chan tea.Msg
	poke chan struct{}
	stop chan struct{}

	// The pieces a test replaces: the stream of events, the list, and
	// the paces.
	events    func(ctx context.Context) (io.ReadCloser, error)
	list      func() ([]Proc, bool)
	heartbeat time.Duration
	retry     time.Duration
	settle    time.Duration
}

// The feed's paces: the heartbeat that covers a missed event, the wait
// before a stream that ended is opened again, and the moment events are
// let settle before one list answers them all — a compose up starts
// several containers in a burst, and one list is enough.
const (
	dockerHeartbeat = 30 * time.Second
	dockerRetry     = 15 * time.Second
	dockerSettle    = 300 * time.Millisecond
)

// startDocker begins the feed, or nothing where docker is not installed.
func startDocker() tea.Msg {
	if dockerPath == "" {
		return dockerReadyMsg{}
	}
	f := &dockerFeed{
		msgs:      make(chan tea.Msg, 1),
		poke:      make(chan struct{}, 1),
		stop:      make(chan struct{}),
		events:    dockerEvents,
		list:      listContainers,
		heartbeat: dockerHeartbeat,
		retry:     dockerRetry,
		settle:    dockerSettle,
	}
	go f.run()
	return dockerReadyMsg{feed: f}
}

// dockerEvents opens docker's stream of container events, one a line.
func dockerEvents(ctx context.Context) (io.ReadCloser, error) {
	cmd := exec.CommandContext(ctx, dockerPath, "events", "--filter", "type=container", "--format", "{{.Status}}")
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &eventStream{ReadCloser: out, cmd: cmd}, nil
}

// eventStream is the stream with the process behind it, which closing
// the stream ends.
type eventStream struct {
	io.ReadCloser
	cmd *exec.Cmd
}

func (s *eventStream) Close() error {
	err := s.ReadCloser.Close()
	_ = s.cmd.Process.Kill()
	_ = s.cmd.Wait()
	return err
}

// nextDocker is the command that waits for the feed's next word.
func nextDocker(f *dockerFeed) tea.Cmd {
	if f == nil {
		return nil
	}
	return func() tea.Msg { return <-f.msgs }
}

// ask asks the feed to list again now — after a kill, after r — without
// waiting for an event to say so. A poke already pending is the same ask.
func (f *dockerFeed) ask() {
	if f == nil {
		return
	}
	select {
	case f.poke <- struct{}{}:
	default:
	}
}

// close ends the feed.
func (f *dockerFeed) close() {
	if f != nil {
		close(f.stop)
	}
}

// run is the feed: a list at the start, then one for every burst of
// events, every heartbeat, and every poke, and the stream opened again
// after a wait when it ends.
func (f *dockerFeed) run() {
	f.tell()
	first := true
	for {
		ctx, cancel := context.WithCancel(context.Background())
		stream, err := f.events(ctx)
		if err != nil {
			cancel()
			select {
			case <-time.After(f.retry):
				continue
			case <-f.stop:
				return
			}
		}
		// A stream opened again is a daemon back, or one that came up
		// after conn did: what it holds is not what was last said.
		if !first {
			f.tell()
		}
		first = false
		lines := make(chan struct{}, 1)
		ended := make(chan struct{})
		go func() {
			sc := bufio.NewScanner(stream)
			for sc.Scan() {
				select {
				case lines <- struct{}{}:
				default: // one pending says as much as several
				}
			}
			close(ended)
		}()
		if !f.follow(lines, ended) {
			cancel()
			_ = stream.Close()
			return
		}
		cancel()
		_ = stream.Close()
		select {
		case <-time.After(f.retry):
		case <-f.stop:
			return
		}
	}
}

// follow answers the stream's events, the heartbeat and the pokes with a
// list each, until the stream ends — true — or the feed is closed.
func (f *dockerFeed) follow(lines, ended chan struct{}) bool {
	beat := time.NewTicker(f.heartbeat)
	defer beat.Stop()
	var settle <-chan time.Time
	for {
		select {
		case <-lines:
			// A burst of events is one list, once they have settled.
			if settle == nil {
				settle = time.After(f.settle)
			}
		case <-settle:
			settle = nil
			f.tell()
		case <-beat.C:
			f.tell()
		case <-f.poke:
			f.tell()
		case <-ended:
			return true
		case <-f.stop:
			return false
		}
	}
}

// tell lists the containers and says so. A word not yet heard is
// replaced by the newer: the model wants the latest, not the history.
func (f *dockerFeed) tell() {
	cs, stalled := f.list()
	msg := dockerMsg{containers: cs, stalled: stalled}
	select {
	case f.msgs <- msg:
	default:
		select {
		case <-f.msgs:
		default:
		}
		f.msgs <- msg
	}
}
