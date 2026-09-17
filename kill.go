package main

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
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
// it — y — or cancels it, whatever it is, so nothing else binds while
// the question is on the status line. That is tmux's own confirmation,
// the one a hand that has killed a pane there already knows.
//
// The question is the command conn is about to run, spelled as it
// would be typed: kill with the signal by name, docker stop, tmux's
// kill-pane. It says what ending a process is — a signal — which
// signal conn chose for this one and so why a bare shell gets a
// different one, and the spelling to do it by hand. A question that
// said end or kill in conn's own words hid all three.

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
	// The pane to close, where the row is a declared process. One that
	// has ended holds its pane for its output: there is nothing left to
	// signal, and the pane is what goes. One still up is signalled, and
	// its pane goes once it has recorded the end, so that the row is
	// DOWN in one move: an end the operator asked for has nothing in it
	// to read.
	pane string
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

// closeWait is how long conn gives a declared process to answer the
// signal before leaving its pane standing, and closePoll how often it
// looks for the end meanwhile. It is well past docker's own patience:
// a docker compose up asked to end stops each service and gives it ten
// seconds before insisting, and a stack ended this way should go down
// whole and its pane with it. A process that has not gone in this time
// is not answering, and its pane and row say so rather than the pane
// being pulled from under it.
const (
	closeWait = 30 * time.Second
	closePoll = 100 * time.Millisecond
)

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
// CONFIRM, where a whole window's width can hold it: the kill command
// with its signal, and beside it the program it is about, which the
// command does not say.
func killPrompt(command string, pid int, sig syscall.Signal) string {
	return question("kill -"+signalName(sig)+" "+strconv.Itoa(pid), command)
}

// signalName is a signal as kill spells it: TERM, KILL.
func signalName(sig syscall.Signal) string {
	switch sig {
	case syscall.SIGKILL:
		return "KILL"
	case syscall.SIGTERM:
		return "TERM"
	case syscall.SIGINT:
		return "INT"
	case syscall.SIGHUP:
		return "HUP"
	}
	return strings.ToUpper(strings.TrimPrefix(unix.SignalName(sig), "SIG"))
}

// closePrompt is the question for a declared process that has ended
// and holds its pane: tmux's own kill-pane, since what goes is the
// pane and its output, the process being over already.
func closePrompt(pane, name string) string {
	return question("kill-pane "+pane, name)
}

// stopPrompt is the question for a container: docker stop, which asks
// the container to go and waits before insisting, where a kill is a
// signal and an instant. The id is what docker is told; the service is
// what the row is called.
func stopPrompt(id, service string) string {
	return question("docker stop "+id, service)
}

// question is a command about to be run, as tmux puts one: the
// command, what it is about where the command does not say, and y or
// n. The command keeps the case it would be typed in.
func question(command, about string) string {
	return join(" · ", command, about) + "? (y/n)"
}
