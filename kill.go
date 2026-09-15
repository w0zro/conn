package main

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// x asks a process to end. What it is asked with depends on what it is:
// an interactive shell — bash proven, zsh proven harder still — ignores
// SIGTERM outright, and zsh ignores SIGHUP too, so a bare shell with
// nothing running in it, the only entry a shell is ever its own, is
// killed outright instead; there is nothing in it to give the chance to
// save. Anything a shell is running — an editor, a build, a contact —
// gets SIGTERM, the ask a well-behaved command answers by saving,
// flushing and tearing its own children down, the way it would ending
// on its own. The cursor's entry is already the leaf the processes view
// covers a shell with, so this is always the command alone: the shell
// it runs in, if any, is left at its prompt rather than taken with it.
//
// x arms a kill rather than sending one: the next key either confirms
// it — x, y or enter — or cancels it, whatever it is, so nothing else
// binds while the question is on the status line.

// pendingKill is a kill x has asked for and not yet answered.
type pendingKill struct {
	pid     int
	command string
	sig     syscall.Signal
	prompt  string // the question, as the status line puts it
	// The container to stop, where the row is one. A container is not a
	// process of this machine and has no pid to signal: docker holds it,
	// and docker is asked to let it go.
	container string
}

// killSignal is what x sends a kind of entry: SIGKILL for a bare
// shell, since asking it nicely does not work; SIGTERM for anything it
// runs.
func killSignal(kind string) syscall.Signal {
	if kind == kindShell {
		return syscall.SIGKILL
	}
	return syscall.SIGTERM
}

// killGrace is how long conn waits after a signal before reading the
// process table again, so the row is not read a moment too soon, the
// process still in it.
const killGrace = 400 * time.Millisecond

// killedMsg is what became of a kill once it was sent.
type killedMsg struct {
	command string
	pid     int
	sig     syscall.Signal
}

// signal sends a process a signal, refusing what should never be
// signalled and naming the failures worth saying plainly.
func signal(pid int, sig syscall.Signal) error {
	switch {
	case pid <= 1:
		return errors.New("refusing to signal pid " + strconv.Itoa(pid))
	case pid == os.Getpid():
		return errors.New("refusing to signal conn itself")
	}
	err := syscall.Kill(pid, sig)
	switch {
	case errors.Is(err, syscall.ESRCH):
		return errors.New("already gone")
	case errors.Is(err, syscall.EPERM):
		return errors.New("not permitted")
	}
	return err
}

// killPrompt asks the question x arms, for the status line beside
// CONFIRM, where a whole window's width can hold it: kill, for a bare
// shell that has nothing to lose by it, end for anything asked more
// gently.
func killPrompt(command string, pid int, sig syscall.Signal) string {
	verb := "end"
	if sig == syscall.SIGKILL {
		verb = "kill"
	}
	return strings.ToUpper(verb + " " + command + " " + strconv.Itoa(pid) + " · x confirms · any other key cancels")
}

// stopPrompt is the question for a container. It says stop, which is
// docker's own word and the true one: docker asks the container to go
// and waits before insisting, where a kill is a signal and an instant.
// And it names the service rather than a pid, a container having none
// that means anything here.
func stopPrompt(service string) string {
	return strings.ToUpper("stop " + service + " · x confirms · any other key cancels")
}
