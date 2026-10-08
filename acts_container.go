package main

import (
	tea "charm.land/bubbletea/v2"
	"github.com/w0zro/conn/internal/room"
	"github.com/w0zro/conn/internal/shell"
	"github.com/w0zro/conn/internal/work"
	"github.com/w0zro/conn/internal/work/docker"
)

// watchContainer puts a container's output in the workspace: docker's own
// record of what it wrote, followed while it runs.
//
// A container has no shell to enter and no terminal to take over. What it
// has is stdout, which docker keeps whether the container is running or
// long dead — the worker's last words survive it, and those are the ones
// worth having. So the pane conn opens is a reader, and the row it
// belongs to is reached and left like any other from there.
//
// The reading is held open after the log ends. docker logs --follow
// blocks only while there is something to follow: on a container that
// has stopped it prints what there is and returns at once, and the pane
// would be gone before it could be put in the workspace — which is what
// happened the first time this was tried. A container that dies while
// you are watching it ends the same way. So the log is followed and then
// the pane waits, and the last words stay up to be read.
func (m model) watchContainer(e work.Entry) tea.Cmd {
	srv, dir, id := m.srv, e.Cwd, e.Container
	cmd := shell.Quote(docker.Path) + " logs --tail 2000 --follow " + shell.Quote(id) + " 2>&1; " + shell.HoldOpen
	return func() tea.Msg {
		sh, err := srv.OpenWatching(dir, cmd, id)
		if err != nil {
			return nil
		}
		return openedMsg{shell: sh}
	}
}

// shellInContainer opens a shell inside a container, which is what s
// means on a row that is one: a shell here, where here is the container
// rather than the directory it was started for. bash is preferred and sh
// is the fallback, since the smaller images carry only the one.
//
// A container that is not running has no shell to give, and docker says
// so in a line. The pane is held open for that line the way it is held
// for a log that has ended: an error that flashes past is an error
// nobody read.
//
// It is held on the failure only. A log ends and there is still the log
// to read, but a shell you typed exit in has nothing left to show, and
// holding that pane open left you sitting in a cat that echoed what you
// typed and looked for all the world like a shell that had hung.
func (m model) shellInContainer(e work.Entry) tea.Cmd {
	srv, dir, id := m.srv, e.Cwd, e.Container
	cmd := shell.Quote(docker.Path) + " exec -it " + shell.Quote(id) + " sh -c " +
		shell.Quote(pickShell) + " 2>&1 || " + shell.HoldOpen
	return func() tea.Msg {
		sh, err := srv.OpenShellIn(dir, cmd, id)
		if err != nil {
			return nil
		}
		return openedMsg{shell: sh}
	}
}

func (m model) stopContainer(id, service string) tea.Cmd {
	feed := m.dockerFeed
	return func() tea.Msg {
		_, _ = docker.Says(docker.StopWait, "stop", id)
		// The feed hears of it from docker's own events, but a stop
		// asked for here is worth asking about at once rather than
		// waiting to be told.
		feed.Ask()
		return killedMsg{command: service, pid: 0}
	}
}

// containerActs are the keys on a container docker holds up. It has
// no pane until one is opened for it, and what there is to be in front
// of is what it has written, so the first enter opens its output and
// the next goes back into the pane holding it. Nothing to bring up:
// compose is the operator's.
type containerActs struct{}

func (containerActs) enter(m model, e work.Entry) (string, tea.Cmd) {
	return "Its output", m.watchContainer(e)
}

func (containerActs) raise(model, work.Entry) tea.Cmd            { return nil }
func (containerActs) down(work.Entry, map[string]room.Pane) bool { return false }

// end stops the container rather than signalling it: there is no
// process here to send anything to, and docker's stop asks it to go
// before insisting. By its service, which is what it is called here:
// the row's own label carries the ports it publishes, and a question
// that reads STOP CACHE · :6390 is asking about an address. Nothing
// where it has already ended.
func (containerActs) end(m model, e work.Entry) *pendingKill {
	if e.Status == work.StatusEnded || e.Fault {
		return nil
	}
	name := e.Command
	if c := m.containerAt(e.PID); c != nil {
		name = c.Service
	}
	return &pendingKill{prompt: stopPrompt(e.Container, name), end: m.stopContainer(e.Container, name)}
}
