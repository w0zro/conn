package main

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
)

// fakeProc lays out one process under a /proc of the test's own: its
// stat line, its command line, its directory, and the sockets its fds
// hold, by inode.
func fakeProc(t *testing.T, root string, pid, ppid, pgid int, comm, state, started, cwd, argv string, sockets ...string) {
	t.Helper()
	dir := filepath.Join(root, strconv.Itoa(pid))
	if err := os.MkdirAll(filepath.Join(dir, "fd"), 0o755); err != nil {
		t.Fatal(err)
	}
	stat := strconv.Itoa(pid) + " (" + comm + ") " + state + " " + strconv.Itoa(ppid) + " " + strconv.Itoa(pgid) +
		" 1 0 -1 4194304 100 0 0 0 5 3 0 0 20 0 1 0 " + started + " 1000 200 18446744073709551615"
	if err := os.WriteFile(filepath.Join(dir, "stat"), []byte(stat+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if argv != "" {
		b := []byte(argv)
		for i := range b {
			if b[i] == ' ' {
				b[i] = 0
			}
		}
		if err := os.WriteFile(filepath.Join(dir, "cmdline"), append(b, 0), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if cwd != "" {
		if err := os.Symlink(cwd, filepath.Join(dir, "cwd")); err != nil {
			t.Fatal(err)
		}
	}
	for i, inode := range sockets {
		if err := os.Symlink("socket:["+inode+"]", filepath.Join(dir, "fd", strconv.Itoa(3+i))); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTheProcfsScanReadsWhatLsofWould(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "net"), 0o755); err != nil {
		t.Fatal(err)
	}
	// net/tcp: a listener on 8080 (inode 111), one on 5173 (inode 222)
	// over v6, and an established connection that is not a listener.
	tcp := "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" +
		"   0: 00000000:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 111 1 0000000000000000 100 0 0 10 0\n" +
		"   1: 0100007F:C350 0100007F:1F90 01 00000000:00000000 00:00000000 00000000  1000        0 333 1 0000000000000000 20 4 30 10 -1\n"
	tcp6 := "  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" +
		"   0: 00000000000000000000000000000000:1435 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 222 1 0000000000000000 100 0 0 10 0\n"
	if err := os.WriteFile(filepath.Join(root, "net", "tcp"), []byte(tcp), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "net", "tcp6"), []byte(tcp6), 0o644); err != nil {
		t.Fatal(err)
	}
	fakeProc(t, root, 100, 1, 100, "zsh", "S", "5000", "/p/conn", "zsh")
	fakeProc(t, root, 101, 100, 100, "node", "S", "5100", "/p/conn", "node server.js", "111", "333")
	fakeProc(t, root, 102, 1, 102, "vite dev (web)", "R", "5200", "/p/web", "/usr/bin/node vite", "222")
	fakeProc(t, root, 200, 1, 200, "postgres", "S", "10", "", "")              // another user's: no cwd to read
	fakeProc(t, root, 300, 1, 300, "conn", "S", "6000", "/p/conn", "conn nav") // self
	fakeProc(t, root, 301, 300, 300, "lsof", "R", "6001", "/p/conn", "lsof")   // self's child
	if err := os.WriteFile(filepath.Join(root, "uptime"), []byte("1 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	procs, err := procfsScan(root, 300)
	if err != nil {
		t.Fatal(err)
	}
	byPID := map[int]Proc{}
	for _, p := range procs {
		byPID[p.PID] = p
	}
	if len(procs) != 3 {
		t.Fatalf("procs = %+v, want the three the user can read that are not conn's own", procs)
	}
	node := byPID[101]
	if node.PPID != 100 || node.PGID != 100 || node.Dir != "/p/conn" || node.Argv != "node server.js" || node.Command != "node" || node.State != "S" || node.Started != "5100" {
		t.Errorf("node = %+v", node)
	}
	if !slices.Equal(node.Ports, []string{"8080"}) {
		t.Errorf("node ports = %v, want its listener and not its connection", node.Ports)
	}
	vite := byPID[102]
	if vite.Command != "vite dev (web)" || !slices.Equal(vite.Ports, []string{"5173"}) {
		t.Errorf("vite = %+v, want a command with spaces and parentheses read whole, and a v6 listener", vite)
	}

	// The start times, asked again, are what the scan said.
	if got := procfsStarted(root, []int{101, 102, 999}); got[101] != "5100" || got[102] != "5200" || len(got) != 2 {
		t.Errorf("started = %v", got)
	}
}

func TestParseProcStatFindsTheFieldsAfterTheCommand(t *testing.T) {
	p, ok := parseProcStat("42 (a (b) c) Z 7 8 1 0 -1 0 0 0 0 0 0 0 0 0 20 0 1 0 12345 0 0 0")
	if !ok || p.PID != 42 || p.Command != "a (b) c" || p.State != "Z" || p.PPID != 7 || p.PGID != 8 || p.Started != "12345" {
		t.Errorf("parsed %+v, %v", p, ok)
	}
	if _, ok := parseProcStat("garbage"); ok {
		t.Error("garbage should not parse")
	}
}
