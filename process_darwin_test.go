package main

import (
	"os"
	"testing"
	"time"
)

// A listing that does not answer in time is an error, not an empty
// table: the processes view would otherwise read a machine on which
// nothing has a directory as true.
func TestAListingThatDoesNotAnswerIsAnError(t *testing.T) {
	if _, err := listingWithin(50*time.Millisecond, "sleep", "5"); err == nil {
		t.Error("a listing that hung came back as a table")
	}
	if out, err := listingWithin(time.Second, "echo", "answered"); err != nil || out != "answered\n" {
		t.Errorf("a listing that answered: %q, %v", out, err)
	}
	if _, err := listingWithin(time.Second, "no-such-program-anywhere"); err == nil {
		t.Error("a program that is not there came back as a table")
	}
}

// Every process the kernel lists has a name, the one it keeps for it:
// a process that has ended and not been collected is listed by lsof
// for no directory, and so for no command, and its arguments cannot be
// read, and it is still a row with a name and not a dot.
func TestEveryProcessHasAName(t *testing.T) {
	procs, err := readProcesses(os.Getuid())
	if err != nil {
		t.Skip("the table could not be read: " + err.Error())
	}
	for _, p := range procs {
		if p.command == "" {
			t.Errorf("pid %d has no name", p.pid)
		}
	}
}
