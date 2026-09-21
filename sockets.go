package main

import (
	enchex "encoding/hex"
	"net"
	"os"
	"path/filepath"
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

// A socket is one thing a process has open: the protocol, the address
// as the system prints it — *:8438, 127.0.0.1:5173, [::1]:3000, a
// connection as local->peer, or a unix socket's path — and for TCP
// the state, LISTEN or ESTABLISHED or what it is.
type socket struct {
	proto string // TCP, UDP, unix
	addr  string
	state string
}

// listening says whether a socket is one something could connect to: a
// TCP socket in LISTEN, a UDP socket bound to a port, a unix socket by
// path. A UDP socket bound nowhere, *:*, is what a resolver holds and
// says nothing.
func (s socket) listening() bool {
	switch s.proto {
	case "TCP":
		return s.state == "LISTEN"
	case "UDP":
		return !strings.HasSuffix(s.addr, ":*") && !strings.Contains(s.addr, "->")
	case "unix":
		return true
	}
	return false
}

// listeningPorts is the TCP ports something could connect to, once
// each however many addresses they are bound on, lowest first: what
// the row says of a server. A UDP port and a unix socket are said on
// the page and not here; a port is what you would go to.
func listeningPorts(sockets []socket) []string {
	var ports []int
	for _, s := range sockets {
		if s.proto != "TCP" || !s.listening() {
			continue
		}
		_, port, err := net.SplitHostPort(s.addr)
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
func (s socket) String() string {
	out := s.proto + " " + s.addr
	if s.proto == "TCP" && s.state != "" && s.state != "LISTEN" && s.state != "ESTABLISHED" {
		out += " · " + strings.ToLower(s.state)
	}
	return out
}

// parseSockets reads lsof -nP -a -i -F pcnPT: for each process, its
// internet sockets, one per descriptor, the protocol and name and the
// TCP state on their own lines. A socket held on two descriptors is
// one socket.
func parseSockets(out string) map[int][]socket {
	held := map[int][]socket{}
	pid := 0
	var cur socket
	seen := map[int]map[socket]bool{}
	flush := func() {
		if pid > 0 && cur.addr != "" {
			if seen[pid] == nil {
				seen[pid] = map[socket]bool{}
			}
			if !seen[pid][cur] {
				seen[pid][cur] = true
				held[pid] = append(held[pid], cur)
			}
		}
		cur = socket{}
	}
	for _, l := range strings.Split(out, "\n") {
		if l == "" {
			continue
		}
		switch l[0] {
		case 'p':
			flush()
			pid, _ = strconv.Atoi(l[1:])
		case 'f':
			flush()
		case 'P':
			cur.proto = l[1:]
		case 'n':
			cur.addr = l[1:]
		case 'T':
			if v, ok := strings.CutPrefix(l[1:], "ST="); ok {
				cur.state = v
			}
		}
	}
	flush()
	return held
}

// parseUnixSockets reads lsof -nP -a -U -F pcn: for each process, the
// unix sockets it holds that have a path. One without is a pair of
// ends nobody else can reach, and is not said.
func parseUnixSockets(out string) map[int][]socket {
	held := map[int][]socket{}
	pid := 0
	seen := map[int]map[string]bool{}
	for _, l := range strings.Split(out, "\n") {
		if l == "" {
			continue
		}
		switch l[0] {
		case 'p':
			pid, _ = strconv.Atoi(l[1:])
		case 'n':
			path := l[1:]
			if pid <= 0 || !strings.HasPrefix(path, "/") {
				continue
			}
			if seen[pid] == nil {
				seen[pid] = map[string]bool{}
			}
			if !seen[pid][path] {
				seen[pid][path] = true
				held[pid] = append(held[pid], socket{proto: "unix", addr: path})
			}
		}
	}
	return held
}

// The same off /proc, on Linux: the kernel's tables of sockets by
// inode, and each process's descriptors by the inode they point at.

// tcpStates is the kernel's numbering of a TCP socket's state.
var tcpStates = map[string]string{
	"01": "ESTABLISHED", "02": "SYN_SENT", "03": "SYN_RECV", "04": "FIN_WAIT1", "05": "FIN_WAIT2",
	"06": "TIME_WAIT", "07": "CLOSE", "08": "CLOSE_WAIT", "09": "LAST_ACK", "0A": "LISTEN", "0B": "CLOSING",
}

// procNetTable reads one of /proc/net/tcp, tcp6, udp and udp6: each
// row a socket, its addresses in hex and its inode, keyed by inode.
func procNetTable(text, proto string) map[string]socket {
	out := map[string]socket{}
	for i, l := range strings.Split(text, "\n") {
		f := strings.Fields(l)
		if i == 0 || len(f) < 10 {
			continue
		}
		local, remote, state, inode := hexAddr(f[1]), hexAddr(f[2]), f[3], f[9]
		s := socket{proto: proto, addr: local}
		if proto == "TCP" {
			s.state = tcpStates[state]
			if s.state != "LISTEN" {
				s.addr += "->" + remote
			}
		}
		out[inode] = s
	}
	return out
}

// hexAddr is a /proc/net address, ADDR:PORT in hex with the address in
// the host's byte order, as lsof would print it.
func hexAddr(s string) string {
	addr, port, ok := strings.Cut(s, ":")
	if !ok {
		return s
	}
	n, _ := strconv.ParseUint(port, 16, 32)
	p := strconv.FormatUint(n, 10)
	if n == 0 {
		p = "*"
	}
	b, err := enchex.DecodeString(addr)
	if err != nil {
		return s
	}
	switch len(b) {
	case 4:
		ip := net.IPv4(b[3], b[2], b[1], b[0])
		if ip.IsUnspecified() {
			return "*:" + p
		}
		return ip.String() + ":" + p
	case 16:
		// Four words, each little-endian.
		ip := make(net.IP, 16)
		for w := 0; w < 4; w++ {
			for i := 0; i < 4; i++ {
				ip[w*4+i] = b[w*4+3-i]
			}
		}
		if ip.IsUnspecified() {
			return "*:" + p
		}
		return "[" + ip.String() + "]:" + p
	}
	return s
}

// procNetUnix reads /proc/net/unix: each row a unix socket, its path
// where it has one, keyed by inode.
func procNetUnix(text string) map[string]socket {
	out := map[string]socket{}
	for i, l := range strings.Split(text, "\n") {
		f := strings.Fields(l)
		if i == 0 || len(f) < 8 || !strings.HasPrefix(f[7], "/") {
			continue
		}
		out[f[6]] = socket{proto: "unix", addr: f[7]}
	}
	return out
}

// procSocketTables is every socket the kernel has, by inode, read off
// the tables under root/net.
func procSocketTables(root string) map[string]socket {
	all := map[string]socket{}
	for _, t := range []struct{ file, proto string }{{"tcp", "TCP"}, {"tcp6", "TCP"}, {"udp", "UDP"}, {"udp6", "UDP"}} {
		if text, err := os.ReadFile(filepath.Join(root, "net", t.file)); err == nil {
			for inode, s := range procNetTable(string(text), t.proto) {
				all[inode] = s
			}
		}
	}
	if text, err := os.ReadFile(filepath.Join(root, "net", "unix")); err == nil {
		for inode, s := range procNetUnix(string(text)) {
			all[inode] = s
		}
	}
	return all
}

// fdSockets is the sockets a process holds, read off its descriptors:
// each that is a socket names its inode, which the tables know.
func fdSockets(dir string, tables map[string]socket) []socket {
	fds, err := os.ReadDir(filepath.Join(dir, "fd"))
	if err != nil {
		return nil
	}
	var out []socket
	seen := map[socket]bool{}
	for _, fd := range fds {
		link, err := os.Readlink(filepath.Join(dir, "fd", fd.Name()))
		if err != nil {
			continue
		}
		inode, ok := strings.CutPrefix(link, "socket:[")
		if !ok {
			continue
		}
		if s, ok := tables[strings.TrimSuffix(inode, "]")]; ok && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

// closedAfter is how many readings running a row must have had nothing
// listening before conn says its listener is gone. One is not enough:
// the sockets are a listing of their own, and one that does not come
// back costs the reading every port on the machine at once, which
// would stamp every server there is; a server that closes its listener
// and binds it again between two readings is the same shape. Two
// readings running is a port that is really gone, and says so a couple
// of seconds after it went.
const closedAfter = 2

// A servingSeen is a process conn has seen listening: which process it
// was, by the moment it began, so a pid come round again on another
// process is not taken for it, and how many readings running it has
// had nothing open since.
type servingSeen struct {
	started time.Time
	lost    int
}

// markClosed words the rows whose listener has gone while the process
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
func markClosed(projects []project, was map[int]servingSeen) map[int]servingSeen {
	next := map[int]servingSeen{}
	for i := range projects {
		for j := range projects[i].entries {
			e := &projects[i].entries[j]
			if e.pid <= 0 || e.kind == kindContact || over(e.status) {
				continue
			}
			if len(e.ports) > 0 {
				next[e.pid] = servingSeen{started: e.started}
				continue
			}
			prev, ok := was[e.pid]
			if !ok || !prev.started.Equal(e.started) {
				continue
			}
			prev.lost++
			next[e.pid] = prev
			if prev.lost >= closedAfter && !e.fault && e.status != statusWaiting {
				e.status, e.fault = statusClosed, true
			}
		}
	}
	return next
}
