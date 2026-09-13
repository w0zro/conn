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

// readSession reads the session from the process and its environment.
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
	s.terminal = os.Getenv("TERM_PROGRAM")
	s.terminalVer = os.Getenv("TERM_PROGRAM_VERSION")
	s.tmux = os.Getenv("TMUX") != ""
	if f := strings.Fields(os.Getenv("SSH_CONNECTION")); len(f) > 0 {
		s.sshFrom = f[0]
	}
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
}

// readNetwork reads the interfaces.
func readNetwork() (network, bool) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return network{}, false
	}
	var n network
	var firstV6 string
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
