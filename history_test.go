package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestARecordedRunIsReadBackNewestFirst(t *testing.T) {
	stateDir(t)
	at := time.Date(2026, 9, 7, 12, 0, 0, 0, time.Local)
	for i, r := range []run{
		{Dir: "/p/app", Name: "test", Exit: "1", Summary: "3 failed", At: at, Took: 12},
		{Dir: "/p/app", Name: "build", Exit: "0", At: at.Add(time.Minute)},
		{Dir: "/p/other", Name: "test", Exit: "0", At: at.Add(2 * time.Minute)},
		{Dir: "/p/app", Name: "test", Exit: "0", Summary: "12 passed", At: at.Add(3 * time.Minute), Took: 90},
	} {
		if err := recordRun(r); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	got := pastRuns("/p/app", "test", 5)
	if len(got) != 2 || got[0].Summary != "12 passed" || got[1].Summary != "3 failed" {
		t.Fatalf("runs = %+v, want the two test runs of /p/app, newest first", got)
	}
	if !got[0].At.Equal(at.Add(3 * time.Minute)) {
		t.Errorf("at = %v, want the moment kept", got[0].At)
	}
	if got := describeRuns(got); got != glyphDone+" 1m · "+glyphFailed+" 12s" {
		t.Errorf("described = %q, want each run's mark and length", got)
	}
	if got := pastRuns("/p/app", "test", 1); len(got) != 1 {
		t.Errorf("asked for one, got %d", len(got))
	}
	if got := pastRuns("/p/none", "test", 5); got != nil {
		t.Errorf("a place that never ran = %+v, want nothing", got)
	}
}

func TestTheRunsFileIsCutBackOnceLong(t *testing.T) {
	stateDir(t)
	for i := range runsCap + 1 {
		if err := recordRun(run{Dir: "/p", Name: "t", Exit: "0", At: time.Now().Add(time.Duration(i) * time.Second)}); err != nil {
			t.Fatal(err)
		}
	}
	lines, err := readLines(runsPath())
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != runsKeep {
		t.Errorf("file holds %d lines, want it cut back to %d", len(lines), runsKeep)
	}
	// The latest survive the cut.
	if got := pastRuns("/p", "t", 1); len(got) != 1 || got[0].At.Before(time.Now().Add(time.Duration(runsCap-1)*time.Second)) {
		t.Errorf("latest = %+v, want the newest run kept", got)
	}
}

func TestAHalfWrittenLineIsSkipped(t *testing.T) {
	dir := stateDir(t)
	path := filepath.Join(dir, "conn", "runs.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"dir":"/p","name":"t","exit":"0","at":"2026-09-07T12:00:00Z"}`+"\n"+`{"dir":"/p","na`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := pastRuns("/p", "t", 5); len(got) != 1 {
		t.Errorf("runs = %+v, want the whole line and not the half", got)
	}
}

func TestAStartTokenReadsAsAMoment(t *testing.T) {
	got := parseStart("Fri Aug 9 10:00:00 2026")
	want := time.Date(2026, 8, 9, 10, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Errorf("parseStart = %v, want %v", got, want)
	}
	if !parseStart("").IsZero() || !parseStart("bad").IsZero() {
		t.Error("a token that does not read should be zero")
	}
	for d, want := range map[time.Duration]string{
		500 * time.Millisecond: "<1s", 12 * time.Second: "12s", 90 * time.Second: "1m", 2 * time.Hour: "2h",
	} {
		if got := shortTook(d); got != want {
			t.Errorf("shortTook(%v) = %q, want %q", d, got, want)
		}
	}
	if !strings.Contains(runsPath(), "conn") {
		t.Errorf("runsPath = %q, want it under conn's state", runsPath())
	}
}
