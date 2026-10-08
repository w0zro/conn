package work

import (
	"testing"
	"time"
)

// A row that was listening and is not, while its process lives, says
// CLOSED: the port it had been saying does not simply go off the row.
// One reading is not enough — a listing that did not come back would
// stamp every server on the machine — and the word stands for as long
// as the process does without a listener.
func TestAListenerGoneWhileTheProcessLives(t *testing.T) {
	began := time.Now().Add(-time.Hour)
	serving := []Project{{Path: "/w/a", Entries: []Entry{
		{PID: 300, Kind: KindRun, Command: "node vite", Started: began, Status: StatusActive, Ports: []string{"5173"}},
	}}}
	was := MarkClosed(serving, nil)
	if got := was[300]; !got.Started.Equal(began) || got.Lost != 0 {
		t.Fatalf("a row that serves is held as %+v", got)
	}
	closed := func() []Project {
		return []Project{{Path: "/w/a", Entries: []Entry{
			{PID: 300, Kind: KindRun, Command: "node vite", Started: began, Status: StatusActive},
		}}}
	}
	one := closed()
	was = MarkClosed(one, was)
	if e := one[0].Entries[0]; e.Status != StatusActive || e.Fault {
		t.Errorf("one reading without the port says %q", e.Status)
	}
	two := closed()
	was = MarkClosed(two, was)
	if e := two[0].Entries[0]; e.Status != StatusClosed || !e.Fault {
		t.Errorf("two readings without the port say %q, fault %v", e.Status, e.Fault)
	}
	three := closed()
	was = MarkClosed(three, was)
	if e := three[0].Entries[0]; e.Status != StatusClosed {
		t.Errorf("the word does not hold: %q", e.Status)
	}
	// And a listener bound again is a row at work: the word goes, and
	// the count it was stamped on starts over.
	back := serving
	was = MarkClosed(back, was)
	if e := back[0].Entries[0]; e.Status != StatusActive || e.Fault {
		t.Errorf("a port bound again says %q", e.Status)
	}
	again := closed()
	MarkClosed(again, was)
	if e := again[0].Entries[0]; e.Status != StatusActive {
		t.Errorf("the first reading after the port came back says %q", e.Status)
	}
}

// What conn does not watch this way: a contact, which is filed by what
// it asks of you and never by what it has open; a row that is over,
// which serves nothing; a row conn only reports, with no pid of its
// own; a pid come round again on another process; and a row that is
// already a fault or waiting, which has the word worth reading.
func TestWhatTheClosedWordLeavesAlone(t *testing.T) {
	began := time.Now().Add(-time.Hour)
	since := began.Add(time.Minute)
	served := map[int]ServingSeen{
		301: {Started: began, Lost: ClosedAfter},
		302: {Started: began, Lost: ClosedAfter},
		303: {Started: began, Lost: ClosedAfter},
		304: {Started: began, Lost: ClosedAfter},
		305: {Started: since, Lost: ClosedAfter},
		306: {Started: began, Lost: ClosedAfter},
	}
	projects := []Project{{Path: "/w/a", Entries: []Entry{
		{PID: 301, Kind: KindContact, Command: "claude", Started: began, Status: StatusWorking},
		{PID: 302, Kind: KindRun, Command: "node vite", Started: began, Status: StatusEnded, Fault: true},
		{PID: 0, Kind: KindService, Command: "web", Container: "abc", Status: StatusActive},
		{PID: 304, Kind: KindRun, Command: "node vite", Started: began, Status: StatusStopped, Fault: true},
		{PID: 305, Kind: KindRun, Command: "node vite", Started: began, Status: StatusActive},
		{PID: 306, Kind: KindContact, Command: "claude", Started: began, Status: StatusWaiting},
	}}}
	MarkClosed(projects, served)
	for _, e := range projects[0].Entries {
		if e.Status == StatusClosed {
			t.Errorf("%d says CLOSED", e.PID)
		}
	}
}
