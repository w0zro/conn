package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/w0zro/conn/internal/room"
	"github.com/w0zro/conn/internal/shell"
	"github.com/w0zro/conn/internal/work"
	"github.com/w0zro/conn/internal/work/brew"
	"github.com/w0zro/conn/internal/work/declared"
	"github.com/w0zro/conn/internal/work/docker"
)

// conn u, x and r: start a process in conn's server under a name, end
// one, start one again. What an agent started with a shell of its own
// ran out of sight, its output in a transcript; started through conn it
// is a row on the panel, read by conn l, and the same process the
// operator's run would be. A check is waited on, and answers with its
// output and its status.

// runPoll is how often a wait looks for the end of what it waits on.
const runPoll = 200 * time.Millisecond

// runU starts a command under a name. Without a command, the name is a
// declaration in the project's .conn, brought up as the panel brings
// it up. -w waits for it to end, writes what it said, and exits with
// its status.
func runU(w io.Writer, home, dir string, args []string) int {
	const use = "conn u [-w] name [-- command...]"
	var command []string
	for i, a := range args {
		if a == "--" {
			args, command = args[:i], args[i+1:]
			break
		}
	}
	fs := flag.NewFlagSet("u", flag.ContinueOnError)
	wait := fs.Bool("w", false, "wait for it to end")
	fs.SetOutput(io.Discard)
	words, err := parseAnywhere(fs, args)
	if err != nil || len(words) != 1 || strings.ContainsAny(words[0], ":@") || isPid(words[0]) {
		return usage("u", use)
	}
	name := words[0]
	h, err := readHere("u", home, dir)
	if err != nil {
		return fail("u", err)
	}
	if h.srv == nil {
		return fail("u", errors.New("no server up; conn brings one up"))
	}
	// What stands under the name already: up, it is an error; ended in
	// a pane of conn's, that pane is replaced.
	replace := ""
	var none noSuch
	if f, err := h.find(name); err != nil && !errors.As(err, &none) {
		return fail("u", err)
	} else if err == nil {
		if p, ok := h.paneOf(f.Entry); ok && p.Declared == f.Declared && p.Exit != "" {
			replace = p.ID
		} else {
			return fail("u", fmt.Errorf("%s is up, pid %s; conn r %s starts it again", name, pidWord(f.Entry), name))
		}
	}
	var sh room.Shell
	if len(command) == 0 {
		d, ok := h.declaration(name)
		if !ok {
			return usage("u", use+": "+name+" is not in the project's .conn, so say what runs")
		}
		sh, err = h.srv.RaiseDeclared(d.At(h.root), d.Command, d.Name, declared.Mark(h.root, d.Name), replace, false)
	} else {
		sh, err = h.srv.RunNamed(realDir(dir), shellLine(command), name, declared.Mark(h.root, name), replace)
	}
	if err != nil {
		return fail("u", err)
	}
	if !*wait {
		_, _ = fmt.Fprintf(w, "%s is up in pane %s; conn l %s reads it\n", name, sh.Pane.ID, name)
		return 0
	}
	exit, err := waitExit(h.srv, sh.Pane.ID)
	if err != nil {
		return fail("u", err)
	}
	lines, _, err := h.srv.Scrollback(sh.Pane.ID)
	if err == nil {
		for _, l := range trimBlank(lines) {
			_, _ = fmt.Fprintln(w, l)
		}
	}
	code, _ := strconv.Atoi(exit)
	return code
}

// shellLine is what follows -- as a line for sh: one word is a line
// as written, pipes and all, the way an agent writes one; several are
// a command and its arguments, each quoted, as exec takes them.
func shellLine(words []string) string {
	if len(words) == 1 {
		return words[0]
	}
	quoted := make([]string, len(words))
	for i, w := range words {
		quoted[i] = w
		if !plainWord.MatchString(w) {
			quoted[i] = shell.Quote(w)
		}
	}
	return strings.Join(quoted, " ")
}

// plainWord is a word sh reads as written, which is left unquoted so
// the row reads as it was typed.
var plainWord = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

// waitExit waits for what a pane runs to end, and answers the code it
// recorded. One that ended without recording one was ended from
// outside, and is an error.
func waitExit(srv *room.Server, pane string) (string, error) {
	for {
		if exit, ended := srv.PaneEnded(pane); ended {
			if exit == "" {
				return "", errors.New("it was ended before it said how it ended")
			}
			return exit, nil
		}
		time.Sleep(runPoll)
	}
}

// declaration is the project's .conn line of a name, where it has one.
func (h here) declaration(name string) (declared.Declaration, bool) {
	for _, d := range h.declared[h.root].List {
		if d.Name == name {
			return d, true
		}
	}
	return declared.Declaration{}, false
}

// runX ends the process an address names.
func runX(w io.Writer, home, dir string, args []string) int {
	f, h, code := actOn("x", "conn x name|:port|pid", home, dir, args)
	if code >= 0 {
		return code
	}
	how, err := h.end(f)
	if err != nil {
		return fail("x", err)
	}
	_, _ = fmt.Fprintf(w, "%s ended: %s\n", nameOf(f.Entry), how)
	return 0
}

// runR ends the process an address names and starts it again.
func runR(w io.Writer, home, dir string, args []string) int {
	f, h, code := actOn("r", "conn r name|:port|pid", home, dir, args)
	if code >= 0 {
		return code
	}
	how, err := h.restart(f)
	if err != nil {
		return fail("r", err)
	}
	_, _ = fmt.Fprintf(w, "%s restarted: %s\n", nameOf(f.Entry), how)
	return 0
}

// restart ends a row and starts it again, and says how: a container or
// a brew service by its own restart, and a process conn started by
// name in a pane of its own anew, as it was started. A process conn
// did not start, conn cannot start again, and says so. The panel's r
// and the CLI's are this.
func (h here) restart(f found) (string, error) {
	name := nameOf(f.Entry)
	switch {
	case f.Container != "":
		if _, err := docker.Says(docker.StopWait, "restart", f.Container); err != nil {
			return "", fmt.Errorf("docker would not restart %s: %v", name, err)
		}
		return "docker restart " + f.Container, nil
	case f.Brew != "":
		if _, err := brew.Says(brew.Wait, "services", "restart", f.Brew); err != nil {
			return "", fmt.Errorf("brew would not restart %s: %v", name, err)
		}
		return "brew services restart " + f.Brew, nil
	}
	p, ok := h.paneOf(f.Entry)
	if !ok || f.Declared == "" || p.Declared != f.Declared {
		return "", fmt.Errorf("conn did not start %s, so it cannot start it again; conn x ends it, and conn u %s -- command starts it in conn", name, name)
	}
	path, _, _ := declared.Unmark(f.Declared)
	command, at := p.Run, f.Cwd
	if command == "" {
		d, ok := declaredIn(h, path, name)
		if !ok {
			return "", fmt.Errorf("%s is no longer in the project's .conn", name)
		}
		command, at = d.Command, d.At(path)
	}
	if p.Exit == "" {
		if _, err := h.end(f); err != nil {
			return "", err
		}
	}
	var sh room.Shell
	var err error
	if p.Run != "" {
		sh, err = h.srv.RunNamed(at, command, name, f.Declared, "")
	} else {
		sh, err = h.srv.RaiseDeclared(at, command, name, f.Declared, "", false)
	}
	if err != nil {
		return "", err
	}
	if p.Exit != "" {
		_ = h.srv.ClosePane(p.ID)
	}
	return "in pane " + sh.Pane.ID, nil
}

// declaredIn is a project's .conn line of a name.
func declaredIn(h here, path, name string) (declared.Declaration, bool) {
	for _, d := range h.declared[path].List {
		if d.Name == name {
			return d, true
		}
	}
	return declared.Declaration{}, false
}

// actOn reads the machine and finds the row an acting verb's address
// names, in the project or the folder of repositories it is in: a
// verb that changes a process reaches the caller's own work and no
// further. code is -1 when the row is found, and the exit status
// otherwise.
func actOn(cmd, use, home, dir string, args []string) (found, here, int) {
	if len(args) != 1 {
		return found{}, here{}, usage(cmd, use)
	}
	h, err := readHere(cmd, home, dir)
	if err != nil {
		return found{}, here{}, fail(cmd, err)
	}
	f, err := h.find(args[0])
	if err != nil {
		return found{}, here{}, fail(cmd, err)
	}
	if !h.reaches(f) {
		return found{}, here{}, fail(cmd, fmt.Errorf("%s is %s's, not this project's; conn %s reaches only the work it is run in", args[0], f.project, cmd))
	}
	return f, h, -1
}

// reaches says whether a row is the caller's own work: in the project
// the directory is in, or the folder of repositories that holds it.
func (h here) reaches(f found) bool {
	scope := h.root
	if h.group != "" {
		scope = h.group
	}
	return work.Within(f.project, scope)
}

// end ends a row as x does on the panel, and says how: a container
// stopped and a brew service by their own word, a pane conn opened by
// name sent ctrl-c and closed once it has ended, or closed where it
// has ended already, and a process signalled.
func (h here) end(f found) (string, error) {
	name := nameOf(f.Entry)
	switch {
	case f.Container != "":
		if _, err := docker.Says(docker.StopWait, "stop", f.Container); err != nil {
			return "", fmt.Errorf("docker would not stop %s: %v", name, err)
		}
		return "docker stop " + f.Container, nil
	case f.Brew != "":
		if _, err := brew.Says(brew.Wait, "services", "stop", f.Brew); err != nil {
			return "", fmt.Errorf("brew would not stop %s: %v", name, err)
		}
		return "brew services stop " + f.Brew, nil
	}
	if p, ok := h.paneOf(f.Entry); ok && f.Declared != "" && p.Declared == f.Declared {
		if p.Exit != "" {
			return "kill-pane " + p.ID, h.srv.ClosePane(p.ID)
		}
		if err := h.srv.Interrupt(p.ID); err != nil {
			return "", err
		}
		for deadline := time.Now().Add(closeWait); time.Now().Before(deadline); time.Sleep(closePoll) {
			if _, ended := h.srv.PaneEnded(p.ID); ended {
				_ = h.srv.ClosePane(p.ID)
				return "send-keys C-c", nil
			}
		}
		return "", fmt.Errorf("%s did not end within %s of ctrl-c; its pane %s is left standing", name, closeWait, p.ID)
	}
	if f.PID <= 0 || f.Status == work.StatusDown {
		return "", fmt.Errorf("%s is not running", name)
	}
	sig := killSignal(f.Kind)
	if f.Kind == work.KindShell {
		return "", fmt.Errorf("%s is a shell; conn x ends what a shell runs, not the shell", name)
	}
	if err := signal(f.PID, sig); err != nil {
		return "", fmt.Errorf("%s: %v", name, err)
	}
	return "kill -" + signalName(sig) + " " + strconv.Itoa(f.PID), nil
}

// trimBlank is lines less the blank rows below the last line written.
func trimBlank(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
