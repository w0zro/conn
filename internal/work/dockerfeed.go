package work

import (
	"bufio"
	"context"
	"io"
	"os/exec"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Docker is a source of its own, the way the tmux server is: the
// containers are not read with the process table but beside it, and what
// docker last said is merged into every reading for nothing. So a docker
// that is slow — a desktop's machine waking from its pause — costs the
// first paint nothing, and a docker that is down costs the reading
// nothing either.
//
// And docker is asked when something has happened rather than on a
// clock. Its own stream of events says when a container starts, stops or
// dies, and each says to ask again; a slow heartbeat covers an event
// missed and a daemon that came up after conn did; a stream that ends —
// the daemon gone — is opened again after a wait.
//
// Asking on the reading's beat was the first way and it was wrong twice
// over. The reading waited on docker, so a wedged daemon held the whole
// process list behind it for as long as the question was given. And a
// question every two seconds is a machine never let pause: Docker
// Desktop idles its virtual machine when nothing asks it anything, and
// conn asking forever meant it never did — a laptop kept awake by the
// instrument watching it.

// DockerMsg carries what docker says of its containers, and whether it
// said it in time: stalled means the list is what it last said.
type DockerMsg struct {
	Containers []Container
	Stalled    bool
}

// DockerReadyMsg carries the feed, once it is running.
type DockerReadyMsg struct{ Feed *DockerFeed }

// DockerFeed is the subscription: a goroutine reading docker's events and
// answering each with a list, a heartbeat, and a poke.
type DockerFeed struct {
	Msgs chan tea.Msg
	Poke chan struct{}
	Stop chan struct{}
	// done is closed when the feed's goroutine has let go of its stream,
	// which is what close waits for.
	Done chan struct{}

	// The pieces a test replaces: the stream of events, the list, and the
	// paces. A feed whose stream is a pipe the test writes to, and whose
	// paces are milliseconds, is the same feed.
	Events    func(ctx context.Context) (io.ReadCloser, error)
	List      func() ([]Container, bool)
	Heartbeat time.Duration
	Retry     time.Duration
	Settle    time.Duration
}

// The feed's paces: the heartbeat that covers a missed event, the wait
// before a stream that ended is opened again, and the moment events are
// let settle before one list answers them all — a compose up starts
// several containers in a burst, and one list is enough for all of them.
const (
	dockerHeartbeat = 30 * time.Second
	dockerRetry     = 15 * time.Second
	dockerSettle    = 300 * time.Millisecond
)

// StartDocker begins the feed, or nothing where docker is not installed.
func StartDocker() tea.Msg {
	if DockerPath == "" {
		return DockerReadyMsg{}
	}
	f := &DockerFeed{
		Msgs:      make(chan tea.Msg, 1),
		Poke:      make(chan struct{}, 1),
		Stop:      make(chan struct{}),
		Done:      make(chan struct{}),
		Events:    dockerEvents,
		List:      readContainers,
		Heartbeat: dockerHeartbeat,
		Retry:     dockerRetry,
		Settle:    dockerSettle,
	}
	go f.run()
	return DockerReadyMsg{Feed: f}
}

// dockerEvents opens docker's stream of container events, one to a line.
func dockerEvents(ctx context.Context) (io.ReadCloser, error) {
	cmd := exec.CommandContext(ctx, DockerPath, "events", "--filter", "type=container", "--format", "{{.Status}}")
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &eventStream{ReadCloser: out, cmd: cmd}, nil
}

// eventStream is the stream with the process behind it, which closing the
// stream ends.
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

// NextDocker is the command that waits for the feed's next word.
func NextDocker(f *DockerFeed) tea.Cmd {
	if f == nil {
		return nil
	}
	return func() tea.Msg { return <-f.Msgs }
}

// Ask asks the feed to list again now, without waiting for an event to
// say so. A poke already pending is the same ask.
func (f *DockerFeed) Ask() {
	if f == nil {
		return
	}
	select {
	case f.Poke <- struct{}{}:
	default:
	}
}

// Close ends the feed and waits for it to let go of its stream. The wait
// is the point: docker events is a child conn started, and asking the
// goroutine to stop without waiting would race conn's own exit — the
// program would be gone before the kill went out, and the child would be
// left reparented to init, sitting until the next container event pushed
// a write down a pipe nobody holds.
//
// The wait is bounded all the same. Taking a moment to be tidy is worth
// it; hanging on the way out never is.
func (f *DockerFeed) Close() {
	if f == nil {
		return
	}
	close(f.Stop)
	select {
	case <-f.Done:
	case <-time.After(2 * time.Second):
	}
}

// run is the feed: a list at the start, then one for every burst of
// events, every heartbeat and every poke, and the stream opened again
// after a wait when it ends.
func (f *DockerFeed) run() {
	defer close(f.Done)
	f.tell()
	first := true
	for {
		ctx, cancel := context.WithCancel(context.Background())
		stream, err := f.Events(ctx)
		if err != nil {
			cancel()
			select {
			case <-time.After(f.Retry):
				continue
			case <-f.Stop:
				return
			}
		}
		// A stream opened again is a daemon back, or one that came up
		// after conn did: what it holds now is not what was last said.
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
		alive := f.follow(lines, ended)
		cancel()
		_ = stream.Close()
		if !alive {
			return
		}
		select {
		case <-time.After(f.Retry):
		case <-f.Stop:
			return
		}
	}
}

// follow answers the stream's events, the heartbeat and the pokes with a
// list each, until the stream ends — true — or the feed is closed.
func (f *DockerFeed) follow(lines, ended chan struct{}) bool {
	beat := time.NewTicker(f.Heartbeat)
	defer beat.Stop()
	var settle <-chan time.Time
	for {
		select {
		case <-lines:
			// A burst of events is one list, once it has settled.
			if settle == nil {
				settle = time.After(f.Settle)
			}
		case <-settle:
			settle = nil
			f.tell()
		case <-beat.C:
			f.tell()
		case <-f.Poke:
			f.tell()
		case <-ended:
			return true
		case <-f.Stop:
			return false
		}
	}
}

// tell lists the containers and says so. A word not yet heard is replaced
// by the newer: the panel wants what is true now, not the history of what
// docker has said.
func (f *DockerFeed) tell() {
	cs, stalled := f.List()
	msg := DockerMsg{Containers: cs, Stalled: stalled}
	select {
	case f.Msgs <- msg:
	default:
		select {
		case <-f.Msgs:
		default:
		}
		f.Msgs <- msg
	}
}
