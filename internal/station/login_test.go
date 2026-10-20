package station

import "testing"

// Inside tmux, TERM_PROGRAM is tmux. The row said so twice and never
// said what was drawing the screen, so the client's own answer is
// asked of the server, which conn tells to keep it current.
func TestTheTerminalIsTheOneBeingLookedAt(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "ghostty")
	t.Setenv("TERM_PROGRAM_VERSION", "1.3.1")
	if name, ver := terminalProgram(nil); name != "ghostty" || ver != "1.3.1" {
		t.Errorf("outside tmux: %q %q", name, ver)
	}

	t.Setenv("TERM_PROGRAM", "tmux")
	t.Setenv("TERM_PROGRAM_VERSION", "3.5a")
	client := map[string]string{"TERM_PROGRAM": "ghostty", "TERM_PROGRAM_VERSION": "1.3.1"}
	if name, ver := terminalProgram(client); name != "ghostty" || ver != "1.3.1" {
		t.Errorf("inside a server that carries it: %q %q", name, ver)
	}
	// A server that was not told to carry it, or a terminal that
	// announces nothing, leaves the row the one thing that is true.
	if name, ver := terminalProgram(nil); name != "" || ver != "" {
		t.Errorf("inside a server that does not: %q %q", name, ver)
	}

	// The origin is read the same way, and the environment conn was
	// started with is believed first: it is this process's own.
	t.Setenv("SSH_CONNECTION", "10.0.0.5 51234 10.0.0.9 22")
	if got := sshOrigin(map[string]string{"SSH_CONNECTION": "10.0.0.9 1 2 3"}); got != "10.0.0.5" {
		t.Errorf("own environment: %q", got)
	}
	t.Setenv("SSH_CONNECTION", "")
	if got := sshOrigin(map[string]string{"SSH_CONNECTION": "10.0.0.9 1 2 3"}); got != "10.0.0.9" {
		t.Errorf("the server's answer: %q", got)
	}
	if got := sshOrigin(nil); got != "" {
		t.Errorf("nobody saying: %q", got)
	}
}

// A version is the first thing in a program's answer that reads as one.
func TestFirstVersion(t *testing.T) {
	if firstVersion("zsh 5.9 (arm-apple-darwin23.0.0)") != "5.9" || firstVersion("GNU bash, version 5.2.37(1)-release") != "5.2.37" || firstVersion("") != "" {
		t.Errorf("firstVersion: %q %q", firstVersion("zsh 5.9 (arm-apple-darwin23.0.0)"), firstVersion("GNU bash, version 5.2.37(1)-release"))
	}
}
