package main

import (
	"net"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

// A session is who is at the station and how, as read: the user, the
// shell, the terminal, where conn was started from and what it is
// running as. A blank field leaves its line off the readout.
type login struct {
	user        string
	uid         string
	admin       bool
	host        string // the machine's name, short
	home        string
	shell       string // the shell's path
	shellVer    string
	tty         string // the terminal device, without /dev/
	terminal    string // the terminal program, as it announces itself
	terminalVer string
	tmux        bool
	sshFrom     string // the address conn is reached from over ssh
	lang        string
	zone        string // the time zone's name, where the system links to one
	cwd         string
	pid, ppid   int
	envCount    int
	pathCount   int
	exe         string
	exeSize     int64
	term        string // TERM, and COLORTERM after it
	goVersion   string
	platform    string // GOOS/GOARCH
	threads     int    // the threads the runtime will run at once
}

// readLogin reads the session from the process and its environment.
func readLogin() login {
	s := login{pid: os.Getpid(), ppid: os.Getppid()}
	if u, err := user.Current(); err == nil {
		s.user, s.uid, s.admin = u.Username, u.Uid, isAdmin(u)
	}
	if host, err := os.Hostname(); err == nil {
		s.host, _, _ = strings.Cut(host, ".")
	}
	s.home, _ = os.UserHomeDir()
	if s.shell = os.Getenv("SHELL"); s.shell != "" {
		s.shellVer = firstVersion(run(s.shell, "--version"))
	}
	s.tty = ttyName()
	s.tmux = os.Getenv("TMUX") != ""
	client := tmuxEnvironment()
	s.terminal, s.terminalVer = terminalProgram(client)
	s.sshFrom = sshOrigin(client)
	s.lang = join(" · ", os.Getenv("LANG"), os.Getenv("LC_ALL"))
	s.zone = zoneName()
	s.cwd, _ = os.Getwd()
	s.envCount = len(os.Environ())
	s.pathCount = len(filepath.SplitList(os.Getenv("PATH")))
	if exe, err := os.Executable(); err == nil {
		s.exe = exe
		if info, err := os.Stat(exe); err == nil {
			s.exeSize = info.Size()
		}
	}
	s.term = join(" · ", os.Getenv("TERM"), os.Getenv("COLORTERM"))
	s.goVersion = runtime.Version()
	s.platform = runtime.GOOS + "/" + runtime.GOARCH
	s.threads = runtime.GOMAXPROCS(0)
	return s
}

// isAdmin says whether the user is in the group that administers the
// machine.
func isAdmin(u *user.User) bool {
	ids, err := u.GroupIds()
	if err != nil {
		return false
	}
	for _, name := range []string{"admin", "sudo", "wheel"} {
		g, err := user.LookupGroup(name)
		if err != nil {
			continue
		}
		for _, id := range ids {
			if id == g.Gid {
				return true
			}
		}
	}
	return false
}

// ttyName is the terminal device on stdin, without /dev/.
func ttyName() string {
	info, err := os.Stdin.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return ""
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}
	for _, dir := range []string{"/dev/pts", "/dev"} {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if dir == "/dev" && !strings.HasPrefix(e.Name(), "tty") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			fi, err := os.Stat(path)
			if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
				continue
			}
			if s, ok := fi.Sys().(*syscall.Stat_t); ok && s.Rdev == st.Rdev {
				return strings.TrimPrefix(path, "/dev/")
			}
		}
	}
	return ""
}

// zoneName is the time zone by name: TZ, or the zone /etc/localtime
// links to.
func zoneName() string {
	if tz := os.Getenv("TZ"); tz != "" {
		return tz
	}
	if target, err := os.Readlink("/etc/localtime"); err == nil {
		if _, after, ok := strings.Cut(target, "zoneinfo/"); ok {
			return after
		}
	}
	return ""
}

// firstVersion picks the version out of what a program says of itself:
// the first word that starts with a digit, up to any parenthesis.
func firstVersion(out string) string {
	for _, f := range strings.Fields(out) {
		if f[0] >= '0' && f[0] <= '9' {
			v, _, _ := strings.Cut(f, "(")
			return strings.TrimRight(v, ",;")
		}
	}
	return ""
}

// A network is the interfaces that are up, off loopback, with an address
// that reaches past the link: how many, and the first by name and
// address, an IPv4 one when there is one.
type network struct {
	up    int
	first string
	// The interface the machine reaches everything else through, and
	// whether the route table could be read at all. An interface with
	// an address on it says only that a cable is in: a virtual bridge,
	// a tunnel stub and a link on a stale lease all have one. Where
	// the machine's packets actually leave by is the reading the check
	// was always reaching for.
	route     string
	routeRead bool
}

// readNetwork reads the interfaces.
func readNetwork() (network, bool) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return network{}, false
	}
	var n network
	n.route, n.routeRead = defaultRoute()
	var firstV6, onRoute string
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		var v4, v6 string
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || !ipn.IP.IsGlobalUnicast() {
				continue
			}
			if ipn.IP.To4() != nil && v4 == "" {
				v4 = ifc.Name + " " + ipn.IP.String()
			} else if v6 == "" {
				v6 = ifc.Name + " " + ipn.IP.String()
			}
		}
		if v4 == "" && v6 == "" {
			continue
		}
		n.up++
		if ifc.Name == n.route && onRoute == "" {
			onRoute = join(" ", v4, v6)
			if v4 != "" {
				onRoute = v4
			}
		}
		if n.first == "" && v4 != "" {
			n.first = v4
		}
		if firstV6 == "" {
			firstV6 = v6
		}
	}
	if n.first == "" {
		n.first = firstV6
	}
	// The interface the route leaves by, where there is one, rather
	// than whichever the machine happened to enumerate first.
	if onRoute != "" {
		n.first = onRoute
	}
	return n, true
}

// The state directory: where conn keeps what it keeps, and what stands
// in the way of writing there.
type stateDir struct {
	path    string
	problem string // blank, notDir, readOnly or noPath
}

const (
	stateNotDir   = "not a directory"
	stateReadOnly = "read only"
	stateNoPath   = "no path"
)

// readStateDir finds the state directory — XDG_STATE_HOME, or
// ~/.local/state — and probes the nearest existing ancestor for whether
// conn could write under it.
func readStateDir(home string) stateDir {
	dir := filepath.Join(stateHome(home), "conn")
	s := stateDir{path: dir}
	for probe := dir; ; probe = filepath.Dir(probe) {
		if info, err := os.Stat(probe); err == nil {
			if !info.IsDir() {
				s.problem = stateNotDir
			} else if syscall.Access(probe, 2) != nil {
				s.problem = stateReadOnly
			}
			return s
		}
		if filepath.Dir(probe) == probe {
			s.problem = stateNoPath
			return s
		}
	}
}

// terminalProgram is the terminal conn is being looked at through, and
// the version it gives for itself.
//
// Inside tmux, TERM_PROGRAM is tmux: the multiplexer announces itself
// in the variable the terminal would have used. The row read TMUX 3.5A
// · IN TMUX, which says tmux twice and never says what is drawing the
// screen. What the attached client brought with it is asked for
// instead, which conn's own server is told to keep current as clients
// come and go. A terminal that announces nothing, or a tmux that was
// not told to carry the answer, leaves the row with the one thing that
// is true of it: that this is inside tmux.
func terminalProgram(client map[string]string) (string, string) {
	name, version := os.Getenv("TERM_PROGRAM"), os.Getenv("TERM_PROGRAM_VERSION")
	if name != "tmux" {
		return name, version
	}
	return client["TERM_PROGRAM"], client["TERM_PROGRAM_VERSION"]
}

// sshOrigin is the address this session is reached from over ssh, and
// is blank where nothing says it is reached from anywhere.
//
// The row said LOCAL whenever SSH_CONNECTION was unset, which is not a
// reading: a scrubbed environment, a shell under sudo and a pane that
// outlived the client that made it all say nothing at all, and the row
// called every one of them local. Nothing knowing where a session came
// from is not the same as knowing it came from here.
//
// Inside tmux the environment conn was started with is the one the
// server held at the time, which is frozen: a conn that came up on the
// machine and is now being worked over ssh would still be reading the
// morning's answer. The server is asked instead, since it updates that
// variable as each client attaches, and it is the client that is
// either here or somewhere else. A server that answers -SSH_CONNECTION
// is saying the variable is unset, which is the same silence.
func sshOrigin(client map[string]string) string {
	origin := os.Getenv("SSH_CONNECTION")
	if origin == "" {
		origin = client["SSH_CONNECTION"]
	}
	if f := strings.Fields(origin); len(f) > 0 {
		return f[0]
	}
	return ""
}

// tmuxEnvironment is what the tmux server conn is inside holds, which
// is what the client that last attached brought with it. It is asked
// once and read for whatever the station wants of it, since it is a
// process to ask and the answer is the whole environment either way.
//
// A variable the server has no value for is written with a leading
// minus, which is tmux saying it is unset, and is left out here: that
// is the same silence as never having asked. Outside tmux there is
// nobody to ask and the answer is nothing.
func tmuxEnvironment() map[string]string {
	socket, _, found := strings.Cut(os.Getenv("TMUX"), ",")
	if !found || socket == "" {
		return nil
	}
	held := map[string]string{}
	for _, line := range strings.Split(run("tmux", "-S", socket, "show-environment"), "\n") {
		name, value, ok := strings.Cut(line, "=")
		if !ok || strings.HasPrefix(name, "-") {
			continue
		}
		held[name] = value
	}
	return held
}
