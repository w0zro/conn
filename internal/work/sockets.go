package work

import (
	"net"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// What a process has open to the world: the ports it listens on, the
// connections it holds, and the unix sockets it has by path. A server
// is told from a shell at its prompt by nothing the process table
// says; it is told by this. The reading carries it on every process of
// the operator's, and the page says it; what the panel makes of it is
// a later step.

// A Socket is one thing a process has open: the protocol, the address
// as the system prints it — *:8438, 127.0.0.1:5173, [::1]:3000, a
// connection as local->peer, or a unix socket's path — and for TCP
// the state, LISTEN or ESTABLISHED or what it is.
type Socket struct {
	Proto string // TCP, UDP, unix
	Addr  string
	State string
}

// Listening says whether a socket is one something could connect to: a
// TCP socket in LISTEN, a UDP socket bound to a port, a unix socket by
// path. A UDP socket bound nowhere, *:*, is what a resolver holds and
// says nothing.
func (s Socket) Listening() bool {
	switch s.Proto {
	case "TCP":
		return s.State == "LISTEN"
	case "UDP":
		return !strings.HasSuffix(s.Addr, ":*") && !strings.Contains(s.Addr, "->")
	case "unix":
		return true
	}
	return false
}

// ListeningPorts is the TCP ports something could connect to, once
// each however many addresses they are bound on, lowest first: what
// the row says of a server. A UDP port and a unix socket are said on
// the page and not here; a port is what you would go to.
func ListeningPorts(sockets []Socket) []string {
	var ports []int
	for _, s := range sockets {
		if s.Proto != "TCP" || !s.Listening() {
			continue
		}
		_, port, err := net.SplitHostPort(s.Addr)
		if err != nil {
			continue
		}
		n, err := strconv.Atoi(port)
		if err != nil || slices.Contains(ports, n) {
			continue
		}
		ports = append(ports, n)
	}
	sort.Ints(ports)
	out := make([]string, 0, len(ports))
	for _, n := range ports {
		out = append(out, strconv.Itoa(n))
	}
	return out
}

// String is the socket as the page says it: the protocol and the
// address, and the state where it is not the one the address implies.
func (s Socket) String() string {
	out := s.Proto + " " + s.Addr
	if s.Proto == "TCP" && s.State != "" && s.State != "LISTEN" && s.State != "ESTABLISHED" {
		out += " · " + strings.ToLower(s.State)
	}
	return out
}

// ClosedAfter is how many readings running a row must have had nothing
// listening before conn says its listener is gone. One is not enough:
// the sockets are a listing of their own, and one that does not come
// back costs the reading every port on the machine at once, which
// would stamp every server there is; a server that closes its listener
// and binds it again between two readings is the same shape. Two
// readings running is a port that is really gone, and says so a couple
// of seconds after it went.
const ClosedAfter = 2

// A ServingSeen is a process conn has seen listening: which process it
// was, by the moment it began, so a pid come round again on another
// process is not taken for it, and how many readings running it has
// had nothing open since.
type ServingSeen struct {
	Started time.Time
	Lost    int
}

// MarkClosed words the rows whose listener has gone while the process
// is still there, and answers what to hold for the next reading. A
// server alive on no port is a server nobody can reach, and without
// this the row simply dropped the port it had been saying and stood
// there looking like any other process at work: a fault told only by
// what was missing from the row. It is a fault, so the row says CLOSED
// where a fault's word goes, its project's block counts it among the
// faults, and the fold keeps the row rather than folding a listener
// that no longer listens into the head it had lifted its port onto.
//
// Only a process of the operator's own is watched this way. A
// container publishes its ports for as long as it runs and stops
// publishing only by stopping; a contact is filed by what it asks of
// you and never by what it has open; a row that is over serves nothing,
// and what ended is said by its own word. A row that is already a
// fault, or waiting on the operator, keeps the word it has: that is
// the thing to look at, and the port is the lesser fact beside it.
func MarkClosed(projects []Project, was map[int]ServingSeen) map[int]ServingSeen {
	next := map[int]ServingSeen{}
	for i := range projects {
		for j := range projects[i].Entries {
			e := &projects[i].Entries[j]
			if e.PID <= 0 || e.Kind == KindContact || Over(e.Status) {
				continue
			}
			if len(e.Ports) > 0 {
				next[e.PID] = ServingSeen{Started: e.Started}
				continue
			}
			prev, ok := was[e.PID]
			if !ok || !prev.Started.Equal(e.Started) {
				continue
			}
			prev.Lost++
			next[e.PID] = prev
			if prev.Lost >= ClosedAfter && !e.Fault && e.Status != StatusWaiting {
				e.Status, e.Fault = StatusClosed, true
			}
		}
	}
	return next
}
