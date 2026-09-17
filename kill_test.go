package main

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

// signal refuses init and conn itself outright, whatever permission
// would otherwise allow.
func TestSignalRefusesInitAndItself(t *testing.T) {
	if err := signal(1, syscall.SIGTERM); err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Errorf("pid 1: %v", err)
	}
	if err := signal(os.Getpid(), syscall.SIGTERM); err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Errorf("conn's own pid: %v", err)
	}
}

// signal on a process already gone says so plainly rather than handing
// back syscall's own words for it.
func TestSignalOnWhatIsAlreadyGoneSaysSo(t *testing.T) {
	cmd := exec.Command("sleep", "0.01")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	_ = cmd.Wait() // gone now, and reaped, so the pid answers ESRCH

	if err := signal(pid, syscall.SIGTERM); err == nil || err.Error() != "already gone" {
		t.Errorf("signal on a reaped pid: %v", err)
	}
}

// signal on a live process of the caller's own ends it with the signal
// asked for.
func TestSignalEndsALiveProcess(t *testing.T) {
	cmd := exec.Command("sleep", "5")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill() }()

	if err := signal(cmd.Process.Pid, syscall.SIGTERM); err != nil {
		t.Fatalf("signal: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("the process did not end on SIGTERM")
	}
}

// killSignal is SIGKILL for a bare shell — asking it nicely does not
// work, proven by hand against bash and zsh both — and SIGTERM for
// anything it runs.
func TestKillSignalIsKillForAShellAndTermForWhatItRuns(t *testing.T) {
	if got := killSignal(kindShell); got != syscall.SIGKILL {
		t.Errorf("killSignal(shell) = %v", got)
	}
	for _, kind := range []string{kindContact, kindEditor, kindRun} {
		if got := killSignal(kind); got != syscall.SIGTERM {
			t.Errorf("killSignal(%s) = %v", kind, got)
		}
	}
}

// The question is the command conn is about to run, as it would be
// typed, with what it is about beside it and y or n after it: the
// signal by name, so a bare shell's KILL is seen to differ from
// everything else's TERM; docker stop with the id docker is told;
// tmux's kill-pane with the pane.
func TestTheQuestionIsTheCommand(t *testing.T) {
	if got := killPrompt("claude", 4242, syscall.SIGTERM); got != "kill -TERM 4242 · claude? (y/n)" {
		t.Errorf("killPrompt, term: %q", got)
	}
	if got := killPrompt("sh", 4242, syscall.SIGKILL); got != "kill -KILL 4242 · sh? (y/n)" {
		t.Errorf("killPrompt, kill: %q", got)
	}
	if got := stopPrompt("abc123", "web"); got != "docker stop abc123 · web? (y/n)" {
		t.Errorf("stopPrompt: %q", got)
	}
	if got := closePrompt("%3", "quick"); got != "kill-pane %3 · quick? (y/n)" {
		t.Errorf("closePrompt: %q", got)
	}
	if got := interruptPrompt("%8", "app"); got != "tmux send-keys -t %8 C-c · app? (y/n)" {
		t.Errorf("interruptPrompt: %q", got)
	}
}
