package work

import (
	"net"
	"slices"
	"strings"
)

// What a project's work talks to is the project's too. A database a
// dev server connects to runs from no directory of the project's —
// brew starts it under launchd at its own data directory — and has no
// terminal, so the process table files it nowhere, and a .conn line
// was the only way it came to stand beside the work that needs it. The
// machine says it already: the connection. A process listening on a
// port of this machine, with no row of its own, is a row of every
// project whose work holds a connection to that port.
//
// Only a run is asked what it connects to. A contact and an editor
// talk to tools of their own — an MCP server, a language server, a
// model served on a port — which are the instrument's, not the
// project's; a shell talks to nothing. A port a row already listens
// on, or a container publishes, is that row's, and docker's own proxy
// standing behind a published port is not a row beside the container.

// AttachServed puts the listeners the projects' runs connect to among
// them, each a row at the foot of every project that talks to it, with
// how many projects that is where it is more than one. how is how each
// process stands, as the reading worked it out.
func AttachServed(projects []Project, procs []Process, uid int, how map[int]Standing) []Project {
	rowed := map[int]bool{}
	owned := map[string]bool{}
	for _, pl := range projects {
		for _, e := range pl.Entries {
			rowed[e.PID] = true
			for _, port := range e.Ports {
				owned[port] = true
			}
			for _, port := range ListeningPorts(e.Sockets) {
				owned[port] = true
			}
		}
	}
	listener := map[string]Process{}
	for _, p := range procs {
		if p.UID != uid || rowed[p.PID] {
			continue
		}
		switch KindOf(p) {
		case KindConn, kindHold:
			continue
		}
		for _, port := range ListeningPorts(p.Sockets) {
			if !owned[port] {
				listener[port] = p
			}
		}
	}
	if len(listener) == 0 {
		return projects
	}
	byPid := map[int]Process{}
	for _, p := range procs {
		byPid[p.PID] = p
	}
	// Which listeners each project talks to, in the order first met.
	talks := make([][]int, len(projects))
	shared := map[int]int{}
	for i, pl := range projects {
		for _, e := range pl.Entries {
			if e.Kind != KindRun {
				continue
			}
			for _, port := range peerPorts(byPid[e.PID].Sockets) {
				l, ok := listener[port]
				if !ok || slices.Contains(talks[i], l.PID) {
					continue
				}
				talks[i] = append(talks[i], l.PID)
				shared[l.PID]++
			}
		}
	}
	out := make([]Project, len(projects))
	copy(out, projects)
	for i, pids := range talks {
		if len(pids) == 0 {
			continue
		}
		rows := slices.Clone(out[i].Entries)
		for _, pid := range pids {
			p := byPid[pid]
			status := StatusActive
			if how[pid].Working {
				status = StatusWorking
			}
			e := Entry{PID: pid, Kind: KindOf(p), Command: commandLine(p), Typed: typedLine(p),
				Started: p.Started, Status: status, Cwd: p.Cwd, Sockets: p.Sockets, Ports: ListeningPorts(p.Sockets)}
			if shared[pid] > 1 {
				e.Shared = shared[pid]
			}
			rows = append(rows, e)
		}
		out[i].Entries = rows
	}
	return out
}

// peerPorts is the ports of this machine a process holds connections
// to: the peer of each established TCP connection whose peer is a
// loopback address.
func peerPorts(sockets []Socket) []string {
	var out []string
	for _, s := range sockets {
		if s.Proto != "TCP" || s.State != "ESTABLISHED" {
			continue
		}
		_, peer, ok := strings.Cut(s.Addr, "->")
		if !ok {
			continue
		}
		host, port, err := net.SplitHostPort(peer)
		if err != nil {
			continue
		}
		if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
			continue
		}
		if !slices.Contains(out, port) {
			out = append(out, port)
		}
	}
	return out
}
