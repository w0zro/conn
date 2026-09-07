package main

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// fakeFeed is a feed over a pipe for events, a list that counts its
// calls, and paces that suit a test.
func fakeFeed(t *testing.T) (*dockerFeed, io.WriteCloser, *atomic.Int32) {
	t.Helper()
	r, w := io.Pipe()
	var lists atomic.Int32
	f := &dockerFeed{
		msgs: make(chan tea.Msg, 1),
		poke: make(chan struct{}, 1),
		stop: make(chan struct{}),
		events: func(context.Context) (io.ReadCloser, error) {
			return r, nil
		},
		list: func() ([]Proc, bool) {
			n := lists.Add(1)
			return []Proc{{PID: -int(n), Command: "c", Dir: globalPlace, Container: &Container{ID: "abc", State: "running"}}}, false
		},
		heartbeat: 200 * time.Millisecond,
		retry:     50 * time.Millisecond,
		settle:    20 * time.Millisecond,
	}
	t.Cleanup(func() { f.close(); _ = w.Close() })
	go f.run()
	return f, w, &lists
}

func word(t *testing.T, f *dockerFeed) dockerMsg {
	t.Helper()
	select {
	case msg := <-f.msgs:
		return msg.(dockerMsg)
	case <-time.After(2 * time.Second):
		t.Fatal("the feed said nothing")
		return dockerMsg{}
	}
}

func TestTheFeedListsAtTheStartAndOnEveryBurstOfEvents(t *testing.T) {
	f, w, lists := fakeFeed(t)
	if got := word(t, f); len(got.containers) != 1 || lists.Load() != 1 {
		t.Fatalf("first word = %+v after %d lists; want one list at the start", got, lists.Load())
	}
	// A burst: three events, one list.
	if _, err := io.WriteString(w, "start\nstart\nstart\n"); err != nil {
		t.Fatal(err)
	}
	word(t, f)
	time.Sleep(60 * time.Millisecond)
	if n := lists.Load(); n != 2 {
		t.Errorf("lists = %d, want one for the burst", n)
	}
	// A poke: a list now.
	f.ask()
	word(t, f)
	if n := lists.Load(); n != 3 {
		t.Errorf("lists = %d, want one for the poke", n)
	}
}

func TestTheFeedBeatsAndOpensTheStreamAgain(t *testing.T) {
	f, w, lists := fakeFeed(t)
	word(t, f)
	// The heartbeat lists with no event at all.
	word(t, f)
	if n := lists.Load(); n < 2 {
		t.Errorf("lists = %d, want the heartbeat's", n)
	}
	// The stream ends; after the retry it is opened again, with a list.
	_ = w.Close()
	before := lists.Load()
	deadline := time.Now().Add(2 * time.Second)
	for lists.Load() == before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if lists.Load() == before {
		t.Error("the stream ending should be followed by another opening, and a list")
	}
}
