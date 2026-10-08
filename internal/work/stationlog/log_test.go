package stationlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/w0zro/conn/internal/work"
)

var logNow = time.Date(2026, 10, 3, 14, 2, 11, 0, time.Local)

func logLabel(e work.Entry) string { return e.Command }

func logProjects(entries ...work.Entry) []work.Project {
	return []work.Project{{Path: "/Users/w0zro/projects/w0zro/conn", Entries: entries}}
}

func TestTheLogRecordsWhatThePanelWouldSay(t *testing.T) {
	began := logNow.Add(-time.Hour)
	was := logProjects(
		work.Entry{PID: 10, Kind: work.KindContact, Command: "claude", Status: work.StatusWorking, Started: began},
		work.Entry{PID: 11, Kind: work.KindShell, Command: "zsh", Status: work.StatusIdle, Started: began},
		work.Entry{PID: 12, Kind: work.KindRun, Command: "worker", Status: work.StatusActive, Started: began},
		work.Entry{PID: 13, Kind: work.KindShell, Command: "zsh", Status: work.StatusIdle, Started: began},
		work.Entry{PID: 15, Kind: work.KindRun, Command: "api", Status: work.StatusDown, Started: began},
		work.Entry{PID: 16, Kind: work.KindRun, Command: "node", Status: work.StatusActive, Started: began},
		work.Entry{PID: 17, Kind: work.KindEditor, Command: "vim", Status: work.StatusActive, Started: began},
	)
	now := logProjects(
		work.Entry{PID: 10, Kind: work.KindContact, Command: "claude", Status: work.StatusWaiting, Started: began},
		work.Entry{PID: 11, Kind: work.KindShell, Command: "zsh", Status: work.StatusActive, Started: began},
		work.Entry{PID: 12, Kind: work.KindRun, Command: "worker", Status: "EXIT 1", Fault: true, Started: began},
		work.Entry{PID: 14, Kind: work.KindService, Command: "postgres", Status: work.StatusActive, Started: logNow},
		work.Entry{PID: 15, Kind: work.KindRun, Command: "api", Status: work.StatusActive, Started: began},
	)
	got := Changes(was, now, logLabel, logNow)
	want := []string{"claude WAITING", "worker EXIT 1", "api ACTIVE", "node GONE"}
	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d:\n%v", len(got), len(want), got)
	}
	for i, e := range got {
		if s := e.Label + " " + string(e.Word); s != want[i] {
			t.Errorf("event %d is %q, want %q", i, s, want[i])
		}
		if e.Project != "/Users/w0zro/projects/w0zro/conn" || !e.At.Equal(logNow) {
			t.Errorf("event %d is filed under %q at %v", i, e.Project, e.At)
		}
	}
	// The shell running a command, the service the operator started,
	// the shell and the editor closed are not among them; the run that
	// was there and is not is.
	if got[3].PID != 16 {
		t.Errorf("the row that left is pid %d, want 16", got[3].PID)
	}
}

func TestAContactsTurnOverIsNewsAndItsTurnBegunIsNot(t *testing.T) {
	began := logNow.Add(-time.Hour)
	was := logProjects(
		work.Entry{PID: 10, Kind: work.KindContact, Command: "claude", Status: work.StatusWorking, Started: began},
		work.Entry{PID: 11, Kind: work.KindContact, Command: "claude", Status: work.StatusIdle, Started: began},
		work.Entry{PID: 12, Kind: work.KindContact, Command: "claude", Status: work.StatusWaiting, Started: began},
		work.Entry{PID: 20, Kind: work.KindService, Command: "node", Status: work.StatusActive, Started: began},
	)
	now := logProjects(
		work.Entry{PID: 10, Kind: work.KindContact, Command: "claude", Status: work.StatusIdle, Started: began},
		work.Entry{PID: 11, Kind: work.KindContact, Command: "claude", Status: work.StatusWorking, Started: began},
		work.Entry{PID: 12, Kind: work.KindContact, Command: "claude", Status: work.StatusIdle, Started: began},
		work.Entry{PID: 20, Kind: work.KindService, Command: "node", Status: work.StatusWorking, Started: began},
	)
	got := Changes(was, now, logLabel, logNow)
	if len(got) != 2 || got[0].PID != 10 || got[0].Word != work.StatusIdle || got[1].PID != 12 || got[1].Word != work.StatusIdle {
		t.Errorf("want the two turns over and nothing else, got %v", got)
	}
}

func TestALineSaysWhatItIsOnOrHowLongItStood(t *testing.T) {
	began := logNow.Add(-3 * time.Hour)
	was := logProjects(
		work.Entry{PID: 10, Kind: work.KindContact, Command: "claude", Status: work.StatusWorking, Since: logNow.Add(-6 * time.Minute), Started: began},
		work.Entry{PID: 11, Kind: work.KindContact, Command: "claude", Status: work.StatusWaiting, Since: logNow.Add(-9 * time.Minute), Started: began},
		work.Entry{PID: 12, Kind: work.KindContact, Command: "claude", Status: work.StatusWorking, Since: logNow.Add(-4 * time.Second), Started: began},
		work.Entry{PID: 13, Kind: work.KindRun, Command: "api", Status: work.StatusActive, Started: began},
		work.Entry{PID: 14, Kind: work.KindRun, Command: "worker", Status: work.StatusDown, Since: logNow.Add(-2 * time.Minute), Started: began},
		work.Entry{PID: 15, Kind: work.KindRun, Command: "node", Status: work.StatusActive, Started: logNow.Add(-125 * time.Minute)},
		work.Entry{PID: 16, Kind: work.KindContact, Command: "claude", Status: work.StatusWorking, Started: began},
	)
	now := logProjects(
		work.Entry{PID: 10, Kind: work.KindContact, Command: "claude", Status: work.StatusIdle, Started: began},
		work.Entry{PID: 11, Kind: work.KindContact, Command: "claude", Status: work.StatusIdle, Started: began},
		work.Entry{PID: 12, Kind: work.KindContact, Command: "claude", Status: work.StatusWaiting, Asking: "Allow Bash: rm -rf node_modules?\nThis is the second line", Started: began},
		work.Entry{PID: 13, Kind: work.KindRun, Command: "api", Status: "EXIT 1", Fault: true, Started: began},
		work.Entry{PID: 14, Kind: work.KindRun, Command: "worker", Status: work.StatusActive, Started: began},
		work.Entry{PID: 16, Kind: work.KindContact, Command: "claude", Status: work.StatusIdle, Started: began},
	)
	got := map[int]string{}
	for _, e := range Changes(was, now, logLabel, logNow) {
		got[e.PID] = string(e.Word) + " | " + e.Note
	}
	want := map[int]string{
		10: "IDLE | took 6 min",
		11: "IDLE | waited 9 min",
		12: "WAITING | Allow Bash: rm -rf node_modules?",
		13: "EXIT 1 | ran 3h 00m",
		14: "ACTIVE | down 2 min",
		15: "GONE | ran 2h 05m",
		16: "IDLE | ", // conn never saw when its turn began
	}
	for pid, w := range want {
		if got[pid] != w {
			t.Errorf("pid %d: %q, want %q", pid, got[pid], w)
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %d lines, want %d: %v", len(got), len(want), got)
	}
}

func TestAPidComeRoundAgainIsANewRow(t *testing.T) {
	was := logProjects(work.Entry{PID: 10, Kind: work.KindRun, Command: "worker", Status: work.StatusActive, Started: logNow.Add(-time.Hour)})
	now := logProjects(work.Entry{PID: 10, Kind: work.KindRun, Command: "worker", Status: work.StatusActive, Started: logNow})
	got := Changes(was, now, logLabel, logNow)
	if len(got) != 1 || got[0].Word != Gone {
		t.Errorf("want the old worker gone and the new one unremarked, got %v", got)
	}
}

func TestTheLogIsAFileOfTabbedLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conn", "log")
	events := []Event{
		{At: logNow, Project: "/Users/w0zro/projects/w0zro/conn", Label: "claude", PID: 10, Word: work.StatusWaiting, Note: "Allow Bash: rm -rf?"},
		{At: logNow.Add(time.Second), Project: "/Users/w0zro/projects/web", Label: "go\ttest ./...", PID: 12, Word: "EXIT 1"},
	}
	if err := Append(path, events); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "2026-10-03 14:02:11\t/Users/w0zro/projects/w0zro/conn\tclaude\t10\tWAITING\tAllow Bash: rm -rf?\n" +
		"2026-10-03 14:02:12\t/Users/w0zro/projects/web\tgo test ./...\t12\tEXIT 1\n"
	if string(b) != want {
		t.Errorf("the file reads:\n%s\nwant:\n%s", b, want)
	}
	// A line that is not the log's — a note somebody left in the file —
	// is passed over; the rest read back as they were written.
	if err := os.WriteFile(path, append(b, "a note\n"...), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Read(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != events[0] || got[1].Label != "go test ./..." || got[1].Word != "EXIT 1" {
		t.Errorf("read back %v", got)
	}
	// The last so many, oldest first.
	if got, _ := Read(path, 1); len(got) != 1 || got[0].PID != 12 {
		t.Errorf("the last line is %v", got)
	}
	// No file is no log.
	if got, err := Read(filepath.Join(t.TempDir(), "none"), 0); err != nil || got != nil {
		t.Errorf("a missing file reads %v, %v", got, err)
	}
}

func TestTheLogKeepsALogAndNotAnArchive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log")
	line := Event{At: logNow, Project: "/p", Label: strings.Repeat("x", 80), PID: 1, Word: work.StatusIdle}
	var events []Event
	for len(events)*len(line.Line()) < logCap {
		events = append(events, line)
	}
	if err := Append(path, events); err != nil {
		t.Fatal(err)
	}
	last := Event{At: logNow, Project: "/p", Label: "last", PID: 2, Word: Gone}
	if err := Append(path, []Event{last}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > logCap*3/5 || info.Size() < logCap*2/5 {
		t.Errorf("the file is %d bytes after the trim, want about half of %d", info.Size(), logCap)
	}
	got, err := Read(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || got[len(got)-1] != last || got[0] != line {
		t.Errorf("the trim cut a line: first %v, last %v", got[0], got[len(got)-1])
	}
}

func TestAFaultIsKnownByItsWord(t *testing.T) {
	for word, want := range map[work.Status]bool{work.StatusStopped: true, work.StatusEnded: true, work.StatusClosed: true, "EXIT 1": true, work.StatusWaiting: false, work.StatusDown: false, Gone: false, work.StatusIdle: false} {
		if work.Faulty(word) != want {
			t.Errorf("Faulty(%q) = %v", word, !want)
		}
	}
}
