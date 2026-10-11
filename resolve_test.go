package main

import (
	"net"
	"testing"
)

// The command is the answer's first line that is not a fence; NONE is
// no command, and nothing is no answer.
func TestTheAnswerIsItsCommand(t *testing.T) {
	for out, want := range map[string]string{
		"redis-cli -p 6390\n":                  "redis-cli -p 6390",
		"```\nredis-cli -p 6390\n```\n":        "redis-cli -p 6390",
		"```bash\n`mongosh --port 27017`\n```": "mongosh --port 27017",
	} {
		if got, err := answerLine(out); err != nil || got != want {
			t.Errorf("%q answered %q (%v), want %q", out, got, err, want)
		}
	}
	for _, out := range []string{"NONE\n", "", "```\n```"} {
		if got, err := answerLine(out); err == nil {
			t.Errorf("%q answered %q, want an error", out, got)
		}
	}
}

// A port that answers a request with something other than an HTTP
// status line is not the web's, and is asked about; see
// TestThePortIsWhereTheBrowserGoes for the ones that are.
func TestAPortThatSpeaksAnotherProtocolIsNotTheWebs(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			_, _ = c.Write([]byte("-ERR unknown command\r\n"))
			_ = c.Close()
		}
	}()
	_, port, _ := net.SplitHostPort(l.Addr().String())
	if _, ok := speaksWeb(port); ok {
		t.Error("a redis-like port was taken for the web")
	}
}

// An answer is offered as the question x arms: the command and what it
// is about, run on y.
func TestAnAnswerIsOfferedBeforeItRuns(t *testing.T) {
	m := plainModel()
	m = m.offer(resolvedMsg{command: "redis-cli -p 6390", about: "redis-server", dir: "/w"})
	if m.kill == nil || m.kill.prompt != "redis-cli -p 6390 · redis-server?" || m.kill.end == nil {
		t.Fatalf("offered: %+v", m.kill)
	}
	if m, cmd := press(m, "n"); m.kill != nil || cmd != nil {
		t.Errorf("n let it run: kill %+v, cmd %v", m.kill, cmd != nil)
	}
}

// A stack is every NAME: COMMAND line of the answer, each name once,
// past fences and talk; NONE anywhere is a stack of nothing.
func TestTheStackIsItsNamedLines(t *testing.T) {
	list, err := parseStack("```\nweb: bin/rails server -p 3000\nworker: bundle exec sidekiq\nweb: again\n```\nThese keep running.\n")
	if err != nil || len(list) != 2 || list[0] != (stacked{"web", "bin/rails server -p 3000"}) || list[1] != (stacked{"worker", "bundle exec sidekiq"}) {
		t.Errorf("parsed %+v (%v)", list, err)
	}
	for _, out := range []string{"NONE\n\nIt is a library.", "nothing here\n"} {
		if list, err := parseStack(out); err == nil {
			t.Errorf("%q parsed as %+v", out, list)
		}
	}
}

// A stack is offered on one question, naming each process, and started
// on y.
func TestAStackIsOfferedOnOneQuestion(t *testing.T) {
	m := plainModel()
	m = m.offerStack(stackMsg{project: "/p/shop", list: []stacked{{"web", "npm run dev"}, {"worker", "npm run worker"}}})
	if m.kill == nil || m.kill.prompt != "start web: npm run dev; worker: npm run worker · shop?" || m.kill.end == nil {
		t.Fatalf("offered: %+v", m.kill)
	}
}
