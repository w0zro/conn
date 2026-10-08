package procs

import (
	enchex "encoding/hex"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/w0zro/conn/internal/work"
)

// parseSockets reads lsof -nP -a -i -F pcnPT: for each process, its
// internet sockets, one per descriptor, the protocol and name and the
// TCP state on their own lines. A socket held on two descriptors is
// one socket.
func parseSockets(out string) map[int][]work.Socket {
	held := map[int][]work.Socket{}
	pid := 0
	var cur work.Socket
	seen := map[int]map[work.Socket]bool{}
	flush := func() {
		if pid > 0 && cur.Addr != "" {
			if seen[pid] == nil {
				seen[pid] = map[work.Socket]bool{}
			}
			if !seen[pid][cur] {
				seen[pid][cur] = true
				held[pid] = append(held[pid], cur)
			}
		}
		cur = work.Socket{}
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
			cur.Proto = l[1:]
		case 'n':
			cur.Addr = l[1:]
		case 'T':
			if v, ok := strings.CutPrefix(l[1:], "ST="); ok {
				cur.State = v
			}
		}
	}
	flush()
	return held
}

// parseUnixSockets reads lsof -nP -a -U -F pcn: for each process, the
// unix sockets it holds that have a path. One without is a pair of
// ends nobody else can reach, and is not said.
func parseUnixSockets(out string) map[int][]work.Socket {
	held := map[int][]work.Socket{}
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
				held[pid] = append(held[pid], work.Socket{Proto: "unix", Addr: path})
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
func procNetTable(text, proto string) map[string]work.Socket {
	out := map[string]work.Socket{}
	for i, l := range strings.Split(text, "\n") {
		f := strings.Fields(l)
		if i == 0 || len(f) < 10 {
			continue
		}
		local, remote, state, inode := hexAddr(f[1]), hexAddr(f[2]), f[3], f[9]
		s := work.Socket{Proto: proto, Addr: local}
		if proto == "TCP" {
			s.State = tcpStates[state]
			if s.State != "LISTEN" {
				s.Addr += "->" + remote
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
func procNetUnix(text string) map[string]work.Socket {
	out := map[string]work.Socket{}
	for i, l := range strings.Split(text, "\n") {
		f := strings.Fields(l)
		if i == 0 || len(f) < 8 || !strings.HasPrefix(f[7], "/") {
			continue
		}
		out[f[6]] = work.Socket{Proto: "unix", Addr: f[7]}
	}
	return out
}

// procSocketTables is every socket the kernel has, by inode, read off
// the tables under root/net.
func procSocketTables(root string) map[string]work.Socket {
	all := map[string]work.Socket{}
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
func fdSockets(dir string, tables map[string]work.Socket) []work.Socket {
	fds, err := os.ReadDir(filepath.Join(dir, "fd"))
	if err != nil {
		return nil
	}
	var out []work.Socket
	seen := map[work.Socket]bool{}
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
