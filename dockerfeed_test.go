package main

import (
	"context"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// feedUnderTest is the feed with its stream, its list and its paces
// replaced: a pipe the test writes events to, a counter for the lists,
// and paces in milliseconds. The feed itself is the same feed.
// list is set here and not afterwards: the feed's goroutine reads it
// from the moment it starts, so a test that swapped it in later would be
// writing to a field already being read.
func feedUnderTest(t *testing.T, opened *atomic.Int32, list func() ([]container, bool)) (*dockerFeed, *io.PipeWriter) {
	t.Helper()
	if list == nil {
		list = func() ([]container, bool) { return []container{{id: "abc", service: "web"}}, false }
	}
	r, w := io.Pipe()
	f := &dockerFeed{
		msgs: make(chan tea.Msg, 1),
		poke: make(chan struct{}, 1),
		stop: make(chan struct{}),
		done: make(chan struct{}),
		events: func(context.Context) (io.ReadCloser, error) {
			opened.Add(1)
			return r, nil
		},
		list:      list,
		heartbeat: 40 * time.Millisecond,
		retry:     10 * time.Millisecond,
		settle:    15 * time.Millisecond,
	}
	t.Cleanup(f.close)
	go f.run()
	return f, w
}

// waitFor takes the feed's next word, or fails.
func waitFor(t *testing.T, f *dockerFeed, what string) dockerMsg {
	t.Helper()
	select {
	case msg := <-f.msgs:
		return msg.(dockerMsg)
	case <-time.After(2 * time.Second):
		t.Fatalf("waited for %s", what)
		return dockerMsg{}
	}
}

// The feed lists at the start, so the first rows are there without
// waiting on an event that may not come for hours.
func TestTheFeedListsBeforeAnythingHappens(t *testing.T) {
	var opened atomic.Int32
	f, _ := feedUnderTest(t, &opened, nil)
	if msg := waitFor(t, f, "the first list"); len(msg.containers) != 1 {
		t.Errorf("the first word carried %d containers", len(msg.containers))
	}
}

// A burst of events is one list. compose up starts several containers at
// once, and asking docker once per line would be a question per service
// for an answer that already holds them all.
func TestABurstOfEventsIsOneList(t *testing.T) {
	var opened atomic.Int32
	var lists atomic.Int32
	f, w := feedUnderTest(t, &opened, func() ([]container, bool) { lists.Add(1); return nil, false })
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
	select {
	case <-f.msgs: // the heartbeat may have spoken; that is its job
	default:
	}
	if got := lists.Load() - before; got > 2 {
		t.Errorf("five events in a burst asked docker %d times", got)
	}
}

// A poke asks now, without waiting for an event to say so: what a kill
// or a refresh needs.
func TestAPokeAsksNow(t *testing.T) {
	var opened atomic.Int32
	f, _ := feedUnderTest(t, &opened, nil)
	waitFor(t, f, "the first list")
	f.ask()
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

// Docker going quiet is carried with the list, so the view can say the
// services are as last seen rather than showing them as though they were
// this second's.
func TestTheFeedSaysWhenDockerDidNotAnswer(t *testing.T) {
	var opened atomic.Int32
	f, _ := feedUnderTest(t, &opened, func() ([]container, bool) { return []container{{id: "abc"}}, true })
	waitFor(t, f, "the first list")
	f.ask()
	for range 4 {
		if msg := waitFor(t, f, "a stalled word"); msg.stalled {
			return
		}
	}
	t.Error("the feed never said docker had gone quiet")
}

// The panel holds docker's word and merges it into the next reading, and
// says so under the rows while docker is quiet.
func TestAStalledDockerIsSaidUnderTheRows(t *testing.T) {
	m := newModel(plain)
	m.view, m.width, m.height = viewProcesses, 48, 30
	next, _ := m.Update(dockerMsg{containers: []container{{id: "abc", service: "web", state: "running"}}, stalled: true})
	m = next.(model)
	if !m.dockerStalled || len(m.containers) != 1 {
		t.Fatalf("the panel did not take docker's word: stalled %v, %d containers", m.dockerStalled, len(m.containers))
	}
	if text := texts(drawProcesses(m.processesReport(), m.cursor, 48, 30, plain)); !strings.Contains(text, "AS LAST SEEN") {
		t.Errorf("the view does not say docker is quiet:\n%s", text)
	}
	// And it stops saying so once docker answers again.
	next, _ = m.Update(dockerMsg{containers: []container{{id: "abc", service: "web", state: "running"}}})
	m = next.(model)
	if text := texts(drawProcesses(m.processesReport(), m.cursor, 48, 30, plain)); strings.Contains(text, "AS LAST SEEN") {
		t.Errorf("the view still says docker is quiet:\n%s", text)
	}
}

// Closing the feed takes docker events with it, and waits for it to go.
// conn starts that stream as a child, and a child outlives a parent that
// only asks it to stop and then exits.
func TestClosingTheFeedLetsGoOfTheStream(t *testing.T) {
	var opened atomic.Int32
	closed := make(chan struct{})
	r, _ := io.Pipe()
	f := &dockerFeed{
		msgs: make(chan tea.Msg, 1),
		poke: make(chan struct{}, 1),
		stop: make(chan struct{}),
		done: make(chan struct{}),
		events: func(context.Context) (io.ReadCloser, error) {
			opened.Add(1)
			return watchedStream{ReadCloser: r, closed: closed}, nil
		},
		list:      func() ([]container, bool) { return nil, false },
		heartbeat: time.Second,
		retry:     10 * time.Millisecond,
		settle:    10 * time.Millisecond,
	}
	go f.run()
	waitFor(t, f, "the first list")
	f.close()
	select {
	case <-closed:
	default:
		t.Error("close returned with the stream still open")
	}
	select {
	case <-f.done:
	default:
		t.Error("close returned before the feed had finished")
	}
}

// watchedStream says when it was closed.
type watchedStream struct {
	io.ReadCloser
	closed chan struct{}
}

func (s watchedStream) Close() error {
	select {
	case <-s.closed:
	default:
		close(s.closed)
	}
	return nil
}
