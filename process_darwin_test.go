package main

import (
	"testing"
	"time"
)

// A listing that does not answer in time is an error, not an empty
// table: the watch would otherwise read a machine on which nothing has
// a directory as true.
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
