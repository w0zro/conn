package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
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

// A port that answers a request with an HTTP status line is the web's,
// and goes to the browser; one that answers something else, or
// nothing, is asked about.
func TestAPortThatSpeaksTheWebIsOpened(t *testing.T) {
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer web.Close()
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(web.URL, "http://"))
	if scheme, ok := speaksWeb(port); !ok || scheme != "http" {
		t.Errorf("an http server's port: %q %v", scheme, ok)
	}
	tlsWeb := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer tlsWeb.Close()
	_, port, _ = net.SplitHostPort(strings.TrimPrefix(tlsWeb.URL, "https://"))
	if scheme, ok := speaksWeb(port); !ok || scheme != "https" {
		t.Errorf("an https server's port: %q %v", scheme, ok)
	}
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
	_, port, _ = net.SplitHostPort(l.Addr().String())
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
