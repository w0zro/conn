package work

import "testing"

// A listener a project's run connects to is a row of that project, and
// of every project that talks to it, counted. A contact's connection
// brings nothing in; a port a container publishes is the container's;
// a listener with a row of its own stays where it is.
func TestWhatTheWorkTalksToIsTheProjects(t *testing.T) {
	conn := func(local, peer string) Socket {
		return Socket{Proto: "TCP", Addr: local + "->" + peer, State: "ESTABLISHED"}
	}
	listen := func(addr string) Socket { return Socket{Proto: "TCP", Addr: addr, State: "LISTEN"} }
	procs := []Process{
		{PID: 10, UID: 501, Command: "node", Args: []string{"node", "server.js"}},
		{PID: 20, UID: 501, Command: "ruby", Args: []string{"ruby", "app.rb"}},
		{PID: 30, UID: 501, Command: "claude"},
		{PID: 40, UID: 501, Command: "postgres", Args: []string{"postgres", "-D", "/opt/homebrew/var/postgresql@16"}, Cwd: "/opt/homebrew/var/postgresql@16",
			Sockets: []Socket{listen("127.0.0.1:5432")}},
		{PID: 50, UID: 501, Command: "ollama", Args: []string{"ollama", "serve"}, Sockets: []Socket{listen("127.0.0.1:11434")}},
		{PID: 60, UID: 501, Command: "com.docker.backend", Sockets: []Socket{listen("*:6379")}},
		{PID: 70, UID: 501, Command: "vite", Sockets: []Socket{listen("[::1]:5173")}},
	}
	procs[0].Sockets = []Socket{conn("127.0.0.1:50001", "127.0.0.1:5432"), conn("127.0.0.1:50002", "127.0.0.1:6379"),
		conn("[::1]:50003", "[::1]:5173"), conn("10.0.0.2:50004", "140.82.112.3:443")}
	procs[1].Sockets = []Socket{conn("127.0.0.1:50005", "127.0.0.1:5432")}
	procs[2].Sockets = []Socket{conn("127.0.0.1:50006", "127.0.0.1:11434")}
	projects := []Project{
		{Path: "/p/shop", Entries: []Entry{
			{PID: 10, Kind: KindRun, Command: "node server.js"},
			{PID: 30, Kind: KindContact, Command: "claude"},
			{PID: 70, Kind: KindRun, Command: "vite", Sockets: procs[6].Sockets},
			{PID: 0, Kind: KindService, Command: "cache", Ports: []string{"6379"}},
		}},
		{Path: "/p/blog", Entries: []Entry{{PID: 20, Kind: KindRun, Command: "ruby app.rb"}}},
	}
	got := AttachServed(projects, procs, 501, map[int]Standing{40: {Working: true}})
	for i, want := range []int{5, 2} {
		if len(got[i].Entries) != want {
			t.Fatalf("%s has %d rows, want %d: %+v", got[i].Path, len(got[i].Entries), want, got[i].Entries)
		}
		pg := got[i].Entries[want-1]
		if pg.PID != 40 || pg.Shared != 2 || pg.Status != StatusWorking || pg.Command != "postgres -D /opt/homebrew/var/postgresql@16" || len(pg.Ports) != 1 || pg.Ports[0] != "5432" {
			t.Errorf("%s's last row is %+v, want postgres shared by 2", got[i].Path, pg)
		}
	}
	if len(projects[0].Entries) != 4 {
		t.Error("the projects given were written to")
	}
}
