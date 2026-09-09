package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// agedTranscript files a transcript through writeTranscript, then ages it
// to when — the recency the picker orders by.
func agedTranscript(t *testing.T, claude, dir, id string, when time.Time, records ...string) {
	t.Helper()
	path := writeTranscript(t, claude, dir, id, records...)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
}

func TestSuspendedConversationsAreNewestFirst(t *testing.T) {
	claude := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claude)
	now := time.Now()

	agedTranscript(t, claude, "/p/conn", "aaaa-1111", now.Add(-2*time.Hour),
		`{"type":"last-prompt","lastPrompt":"fix the resize race","gitBranch":"main"}`)
	agedTranscript(t, claude, "/p/conn", "bbbb-2222", now.Add(-time.Minute),
		`{"type":"last-prompt","lastPrompt":"polish the picker","gitBranch":"picker"}`,
		`{"type":"system","subtype":"away_summary","content":"laying out the pane"}`)

	got := claudeSuspended([]string{"/p/conn"}, nil)
	if len(got) != 2 {
		t.Fatalf("listed %d conversations, want 2", len(got))
	}
	if got[0].ID != "bbbb-2222" || got[1].ID != "aaaa-1111" {
		t.Fatalf("order = %s, %s; want the newest first", got[0].ID, got[1].ID)
	}
	if got[0].Prompt != "polish the picker" || got[0].Branch != "picker" {
		t.Errorf("meta = %q on %q, want the prompt and the branch read back", got[0].Prompt, got[0].Branch)
	}
	if got[0].Summary != "laying out the pane" {
		t.Errorf("summary = %q, want the away summary read back", got[0].Summary)
	}
	if got[0].Dir != "/p/conn" {
		t.Errorf("dir = %q, want the directory it was filed under", got[0].Dir)
	}
}

func TestARunningConversationIsNotSuspended(t *testing.T) {
	claude := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claude)

	agedTranscript(t, claude, "/p/conn", "aaaa-1111", time.Now(),
		`{"type":"last-prompt","lastPrompt":"still going"}`)

	got := claudeSuspended([]string{"/p/conn"}, map[string]bool{"aaaa-1111": true})
	if len(got) != 0 {
		t.Fatalf("listed %d conversations, want the live one excluded", len(got))
	}
}

func TestOnlyIdShapedFilesAreConversations(t *testing.T) {
	claude := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claude)

	agedTranscript(t, claude, "/p/conn", "aaaa-1111", time.Now(),
		`{"type":"last-prompt","lastPrompt":"the real one"}`)
	pdir := filepath.Join(claude, "projects", encodePath("/p/conn"))
	// A name that is not an id would end up on a shell command line; it is
	// not a session, whatever it holds.
	for _, name := range []string{"notes.txt", "evil;rm.jsonl", "aaaa-1111"} {
		if err := os.WriteFile(filepath.Join(pdir, name), []byte("{}"), 0o600); err != nil {
			// aaaa-1111 also exists as a directory name in real layouts; a
			// file is close enough for the shape check.
			t.Fatal(err)
		}
	}

	got := claudeSuspended([]string{"/p/conn"}, nil)
	if len(got) != 1 || got[0].ID != "aaaa-1111" {
		t.Fatalf("listed %+v, want only the id-shaped transcript", got)
	}
}

func TestAConversationIsTakenOnceAcrossDirs(t *testing.T) {
	claude := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claude)

	// Claude's encoding is lossy: both of these file under -p-a-b.
	agedTranscript(t, claude, "/p/a/b", "aaaa-1111", time.Now(),
		`{"type":"last-prompt","lastPrompt":"once"}`)

	got := claudeSuspended([]string{"/p/a/b", "/p/a-b"}, nil)
	if len(got) != 1 {
		t.Fatalf("listed %d conversations, want the collision taken once", len(got))
	}
}

func TestConvoMetaFallsBackToAUserRecord(t *testing.T) {
	claude := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claude)

	// An old transcript predates the last-prompt records; what the user
	// typed is still in it.
	agedTranscript(t, claude, "/p/conn", "aaaa-1111", time.Now(),
		`{"type":"user","message":{"content":"make the tests pass"}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"done"}]}}`)

	got := claudeSuspended([]string{"/p/conn"}, nil)
	if len(got) != 1 || got[0].Prompt != "make the tests pass" {
		t.Fatalf("prompt = %+v, want the user record read back", got)
	}
}

func TestClaudeResumeNamesTheSession(t *testing.T) {
	if got := claudeResume("aaaa-1111"); got != "claude --resume aaaa-1111" {
		t.Fatalf("resume command = %q", got)
	}
}

func TestShortAge(t *testing.T) {
	now := time.Now()
	cases := []struct {
		when time.Time
		want string
	}{
		{now.Add(-10 * time.Second), "now"},
		{now.Add(-5 * time.Minute), "5m"},
		{now.Add(-3 * time.Hour), "3h"},
		{now.Add(-50 * time.Hour), "2d"},
		{now.Add(-40 * 24 * time.Hour), "5w"},
	}
	for _, c := range cases {
		if got := shortAge(c.when); got != c.want {
			t.Errorf("shortAge(%v ago) = %q, want %q", time.Since(c.when), got, c.want)
		}
	}
}
func TestConvoDirsCoverThePlace(t *testing.T) {
	m := withProcs(96, 14, []Project{
		{Name: "a", Path: "/g/one/a", Group: "/g/one"},
		{Name: "b", Path: "/g/one/b", Group: "/g/one"},
	}, nil)
	m.groups = []Project{{Name: "one", Path: "/g/one"}}
	m.subs = map[string][]Project{"/g/one/a": {{Name: "docs", Path: "/g/one/a/docs"}}}
	m.rebuild()

	got := m.convoDirs(Project{Path: "/g/one"})
	want := []string{"/g/one", "/g/one/a", "/g/one/a/docs", "/g/one/b"}
	if len(got) != len(want) {
		t.Fatalf("dirs = %v, want %v", got, want)
	}
	seen := map[string]bool{}
	for _, d := range got {
		seen[d] = true
	}
	for _, d := range want {
		if !seen[d] {
			t.Errorf("dirs = %v, missing %s", got, d)
		}
	}
}

func TestLiveConversationsAreVettedAgainstTheTable(t *testing.T) {
	m := withClaude("claude", map[int]claudeSession{
		700: {PID: 700, SessionID: "live-1111"},
	})
	if live := m.liveConversations(); !live["live-1111"] {
		t.Fatal("a running instance's conversation was not counted live")
	}

	// The same advertisement over a pid whose process is something else is a
	// leftover file, and its conversation is suspended.
	m = withClaude("vim", map[int]claudeSession{
		700: {PID: 700, SessionID: "left-2222"},
	})
	if live := m.liveConversations(); live["left-2222"] {
		t.Fatal("a stale session file hid the conversation it left behind")
	}
}

func TestTheNewestAtRestIsReadAloneAndNotALiveOne(t *testing.T) {
	// The row under a place reads one transcript, the newest at rest by
	// its file's time — never one a running instance carries.
	dir := claudeHome(t)
	old := writeTranscript(t, dir, "/p/conn", "aaaa-1111", userRec, asstRec)
	newer := writeTranscript(t, dir, "/p/conn", "bbbb-2222", `{"type":"user","message":{"content":"polish the site"}}`, asstRec)
	live := writeTranscript(t, dir, "/p/conn", "cccc-3333", userRec, asstRec)
	now := time.Now()
	for path, at := range map[string]time.Time{old: now.Add(-2 * time.Hour), newer: now.Add(-time.Hour), live: now} {
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
	}
	c, ok := newestSuspended([]string{"/p/conn"}, map[string]bool{"cccc-3333": true})
	if !ok || c.ID != "bbbb-2222" || c.Kind != "claude" || c.Prompt != "polish the site" {
		t.Errorf("newest = %+v, want the newer conversation at rest, read for its prompt", c)
	}
	if _, ok := newestSuspended([]string{"/p/none"}, nil); ok {
		t.Error("a place with no transcripts has nothing at rest")
	}
}
