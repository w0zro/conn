package work

import (
	"reflect"
	"testing"
)

func TestAStacksWordsAreKnown(t *testing.T) {
	for line, want := range map[string]string{
		"panic: runtime error: invalid memory address":             "PANIC",
		"fatal error: all goroutines are asleep":                   "FATAL",
		"--- FAIL: TestTheRecordPaginates (0.00s)":                 "FAIL",
		"FAIL\tgithub.com/w0zro/conn\t0.774s":                      "FAIL",
		"FAIL":                                                     "FAIL",
		"thread 'main' panicked at src/main.rs:4:5:":               "PANIC",
		"Traceback (most recent call last):":                       "TRACEBACK",
		"TypeError: Cannot read properties of undefined":           "ERROR",
		"Error: listen EADDRINUSE: address already in use :::3000": "EADDRINUSE",
		"Error: Cannot find module './app'":                        "ERROR",
		"[nodemon] app crashed: bind EADDRINUSE 0.0.0.0:3000":      "EADDRINUSE",
		"OSError: [Errno 48] Address already in use":               "ERROR",
	} {
		if got, ok := SaidWord(line); !ok || string(got) != want {
			t.Errorf("%q says %q, want %q", line, got, want)
		}
	}
	for _, line := range []string{
		"error: pathspec 'main' did not match", // git's, in the lower case
		"ok  \tgithub.com/w0zro/conn\t0.5s",
		"the panic was handled",
		"FAILED to load",
		"",
	} {
		if got, ok := SaidWord(line); ok {
			t.Errorf("%q says %q, want nothing", line, got)
		}
	}
	if !SaidWords["PANIC"] || !SaidWords["EADDRINUSE"] || SaidWords["WAITING"] {
		t.Error("SaidWords is the words a pane can say")
	}
}

func TestWhatAPaneSaidSinceItsLastTail(t *testing.T) {
	was := []string{"$ go test ./...", "ok  \tconn\t0.5s", "$ ", "", ""}
	now := []string{"$ go test ./...", "ok  \tconn\t0.5s", "$ go run .", "panic: boom", "goroutine 1 [running]:", "exit status 2", "$ ", ""}
	want := []string{"$ go run .", "panic: boom", "goroutine 1 [running]:", "exit status 2"}
	if got := NewLines(was, now); !reflect.DeepEqual(got, want) {
		t.Errorf("new lines are %q, want %q", got, want)
	}
	// Nothing new: the same tail again, the prompt still being written.
	if got := NewLines(now, now); len(got) != 0 {
		t.Errorf("the same tail again says %q as new", got)
	}
	// The first tail is seen, not said: nothing is new against nothing.
	if got := NewLines(nil, now); got != nil {
		t.Errorf("the first tail says %q as new", got)
	}
	// More than a tail's worth since: every line is new.
	fresh := []string{"a", "b", "c", "$ ", ""}
	if got := NewLines(was, fresh); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("a tail with no trace of the last says %q", got)
	}
}
