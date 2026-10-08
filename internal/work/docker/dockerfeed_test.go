package docker

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"
)

func (s watchedStream) Close() error {
	select {
	case <-s.closed:
	default:
		close(s.closed)
	}
	return nil
}

// A burst of events is one list. compose up starts several containers at
// once, and asking docker once per line would be a question per service
// for an answer that already holds them all.
func TestABurstOfEventsIsOneList(t *testing.T) {
	var opened atomic.Int32
	var lists atomic.Int32
	// The heartbeat is kept out of it: with it at the test's usual
	// pace, a slow runner had it beat twice in the window a fast one
	// beat once, and the count said the burst had cost what it had not.
	f, w := pacedFeed(t, &opened, func() ([]Container, bool) { lists.Add(1); return nil, false }, time.Second)
	waitFor(t, f, "the first list")
	before := lists.Load()

	for range 5 {
		if _, err := w.Write([]byte("start\n")); err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, f, "the burst's list")
	// Past the settle, with no more events: nothing more is asked.
	time.Sleep(60 * time.Millisecond)
	if got := lists.Load() - before; got != 1 {
		t.Errorf("five events in a burst asked docker %d times", got)
	}
}

// A poke asks now, without waiting for an event to say so: what a kill
// or a refresh needs.
func TestAPokeAsksNow(t *testing.T) {
	var opened atomic.Int32
	f, _ := feedUnderTest(t, &opened, nil)
	waitFor(t, f, "the first list")
	f.Ask()
	waitFor(t, f, "the poke's list")
}

// A stream that ends is a daemon gone. It is opened again after a wait,
// and listed again when it comes back, because what a daemon that is
// back holds is not what it last said.
func TestAStreamThatEndsIsOpenedAgain(t *testing.T) {
	var opened atomic.Int32
	f, w := feedUnderTest(t, &opened, nil)
	waitFor(t, f, "the first list")
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, f, "the list after the stream came back")
	if got := opened.Load(); got < 2 {
		t.Errorf("the stream was opened %d times; it should be opened again when it ends", got)
	}
}

// Closing the feed takes docker events with it, and waits for it to go.
// conn starts that stream as a child, and a child outlives a parent that
// only asks it to stop and then exits.
func TestClosingTheFeedLetsGoOfTheStream(t *testing.T) {
	var opened atomic.Int32
	closed := make(chan struct{})
	r, _ := io.Pipe()
	f := &Feed{
		Lists: make(chan List, 1),
		Poke:  make(chan struct{}, 1),
		Stop:  make(chan struct{}),
		Done:  make(chan struct{}),
		Events: func(context.Context) (io.ReadCloser, error) {
			opened.Add(1)
			return watchedStream{ReadCloser: r, closed: closed}, nil
		},
		List:      func() ([]Container, bool) { return nil, false },
		Heartbeat: time.Second,
		Retry:     10 * time.Millisecond,
		Settle:    10 * time.Millisecond,
	}
	go f.run()
	waitFor(t, f, "the first list")
	f.Close()
	select {
	case <-closed:
	default:
		t.Error("close returned with the stream still open")
	}
	select {
	case <-f.Done:
	default:
		t.Error("close returned before the feed had finished")
	}
}

// The feed lists at the start, so the first rows are there without
// waiting on an event that may not come for hours.
func TestTheFeedListsBeforeAnythingHappens(t *testing.T) {
	var opened atomic.Int32
	f, _ := feedUnderTest(t, &opened, nil)
	if msg := waitFor(t, f, "the first list"); len(msg.Containers) != 1 {
		t.Errorf("the first word carried %d containers", len(msg.Containers))
	}
}

// Docker going quiet is carried with the list, so the view can say the
// services are as last seen rather than showing them as though they were
// this second's.
func TestTheFeedSaysWhenDockerDidNotAnswer(t *testing.T) {
	var opened atomic.Int32
	f, _ := feedUnderTest(t, &opened, func() ([]Container, bool) { return []Container{{ID: "abc"}}, true })
	waitFor(t, f, "the first list")
	f.Ask()
	for range 4 {
		if msg := waitFor(t, f, "a stalled word"); msg.Stalled {
			return
		}
	}
	t.Error("the feed never said docker had gone quiet")
}

// feedUnderTest is the feed with its stream, its list and its paces
// replaced: a pipe the test writes events to, a counter for the lists,
// and paces in milliseconds. The feed itself is the same feed.
// list is set here and not afterwards: the feed's goroutine reads it
// from the moment it starts, so a test that swapped it in later would be
// writing to a field already being read.
func feedUnderTest(t *testing.T, opened *atomic.Int32, list func() ([]Container, bool)) (*Feed, *io.PipeWriter) {
	t.Helper()
	return pacedFeed(t, opened, list, 40*time.Millisecond)
}

// pacedFeed is feedUnderTest with the heartbeat chosen: a test counting
// the lists a burst costs wants the heartbeat out of the count, since
// on a slow runner it beats twice in the time a fast one beats once.
func pacedFeed(t *testing.T, opened *atomic.Int32, list func() ([]Container, bool), heartbeat time.Duration) (*Feed, *io.PipeWriter) {
	t.Helper()
	if list == nil {
		list = func() ([]Container, bool) { return []Container{{ID: "abc", Service: "web"}}, false }
	}
	r, w := io.Pipe()
	f := &Feed{
		Lists: make(chan List, 1),
		Poke:  make(chan struct{}, 1),
		Stop:  make(chan struct{}),
		Done:  make(chan struct{}),
		Events: func(context.Context) (io.ReadCloser, error) {
			opened.Add(1)
			return r, nil
		},
		List:      list,
		Heartbeat: heartbeat,
		Retry:     10 * time.Millisecond,
		Settle:    15 * time.Millisecond,
	}
	t.Cleanup(f.Close)
	go f.run()
	return f, w
}

// waitFor takes the feed's next word, or fails.
func waitFor(t *testing.T, f *Feed, what string) List {
	t.Helper()
	select {
	case list := <-f.Lists:
		return list
	case <-time.After(2 * time.Second):
		t.Fatalf("waited for %s", what)
		return List{}
	}
}

// watchedStream says when it was closed.
type watchedStream struct {
	io.ReadCloser
	closed chan struct{}
}
