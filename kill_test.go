package main

import (
	"errors"
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
	defer cmd.Process.Kill()

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
	for _, kind := range []string{kindAgent, kindEditor, kindRun} {
		if got := killSignal(kind); got != syscall.SIGTERM {
			t.Errorf("killSignal(%s) = %v", kind, got)
		}
	}
}

func TestKillPromptAndNote(t *testing.T) {
	if got := killPrompt("claude", 4242, syscall.SIGTERM); got != "END CLAUDE 4242 · X CONFIRMS · ANY OTHER KEY CANCELS" {
		t.Errorf("killPrompt, term: %q", got)
	}
	if got := killPrompt("sh", 4242, syscall.SIGKILL); got != "KILL SH 4242 · X CONFIRMS · ANY OTHER KEY CANCELS" {
		t.Errorf("killPrompt, kill: %q", got)
	}
	// The notes answer in the verb the question asked with, and no
	// signal's name reaches the row.
	sent := killedMsg{command: "claude", pid: 4242, sig: syscall.SIGTERM}
	if got := killNote(sent); got != "ASKED CLAUDE 4242 TO END" {
		t.Errorf("killNote, sent: %q", got)
	}
	killed := killedMsg{command: "sh", pid: 4242, sig: syscall.SIGKILL}
	if got := killNote(killed); got != "KILLED SH 4242" {
		t.Errorf("killNote, killed: %q", got)
	}
	failed := killedMsg{command: "claude", pid: 4242, sig: syscall.SIGTERM, err: errors.New("already gone")}
	if got := killNote(failed); got != "COULD NOT END CLAUDE: ALREADY GONE" {
		t.Errorf("killNote, failed: %q", got)
	}
}
