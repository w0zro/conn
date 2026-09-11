package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Claude Code leaves a suspended conversation's transcript behind when
// its instance exits: <claude>/projects/<encoded cwd>/<session id>.jsonl.
// It is enough to pick the conversation back up, which is what A offers:
// every one of them at a place, newest first, filtered the way the list
// is, and not offering one a live instance already has — vetted against
// the process table, so a session file an ended instance left behind
// cannot hide one still going.

// conversation is a talk claude had in a directory and could pick back
// up: its transcript is on disk, and no live instance is carrying it.
type conversation struct {
	ID     string
	Dir    string    // where it was had, which is where resuming belongs
	When   time.Time // when it last moved
	Branch string
	Prompt string // the last thing its user asked of it
}

// claudeConfigDir is where Claude Code keeps its state — the sessions
// and the transcripts the picker reads, distinct from claudeDir in
// theme.go, which is that same directory's name relative to home, for
// what conn writes there.
func claudeConfigDir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

// encodePath is how Claude Code names a project's transcript directory:
// every character that is not a letter or digit becomes a dash.
var notAlnum = regexp.MustCompile(`[^A-Za-z0-9]`)

func encodePath(p string) string { return notAlnum.ReplaceAllString(p, "-") }

// isSessionID reports whether a transcript's stem is shaped like the
// ids Claude writes — hex and dashes. The id ends up on a shell command
// line, so anything else found beside the transcripts is not one.
func isSessionID(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F', r == '-':
		default:
			return false
		}
	}
	return true
}

// resumeCommand is the command that picks a suspended conversation back
// up. The id travels onto a shell command line, so only ids
// claudeSuspended vetted are ever handed here.
func resumeCommand(id string) string { return agentCommand + " --resume " + id }

// liveConversations is the id of every conversation a running claude
// instance is carrying. A session file can outlive the process that
// wrote it, so a pid is only believed when the process table still has
// it, standing as an agent.
func liveConversations(places []place) map[string]bool {
	pids := map[int]bool{}
	for _, pl := range places {
		for _, e := range pl.entries {
			if e.kind == kindAgent {
				pids[e.pid] = true
			}
		}
	}

	entries, err := os.ReadDir(filepath.Join(claudeConfigDir(), "sessions"))
	if err != nil {
		return nil
	}
	live := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSuffix(name, ".json"))
		if err != nil || !pids[pid] {
			continue
		}
		b, err := os.ReadFile(filepath.Join(claudeConfigDir(), "sessions", name))
		if err != nil {
			continue
		}
		var f struct {
			SessionID string `json:"sessionId"`
		}
		if json.Unmarshal(b, &f) == nil && f.SessionID != "" {
			live[f.SessionID] = true
		}
	}
	return live
}

// convoTail is how much of a transcript's end is read for the picker:
// enough to reach back past a tool-heavy turn to the last prompt, small
// enough that a directory of them is read on a keystroke.
const convoTail = 256 * 1024

// claudeSuspended lists the conversations at rest under the given
// directories, newest first, excluding the ones a live instance is
// carrying.
func claudeSuspended(dirs []string, places []place) []conversation {
	live := liveConversations(places)
	root := filepath.Join(claudeConfigDir(), "projects")

	// Claude encodes directories lossily, so two of them can share a
	// transcript directory; each conversation is taken once, for the
	// first directory that reached it.
	seen := map[string]bool{}
	var out []conversation
	for _, dir := range dirs {
		entries, err := os.ReadDir(filepath.Join(root, encodePath(dir)))
		if err != nil {
			continue
		}
		for _, e := range entries {
			id := strings.TrimSuffix(e.Name(), ".jsonl")
			if e.IsDir() || id == e.Name() || !isSessionID(id) || live[id] || seen[id] {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			seen[id] = true
			c := conversation{ID: id, Dir: dir, When: info.ModTime()}
			readConvoMeta(filepath.Join(root, encodePath(dir), e.Name()), &c)
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].When.Equal(out[j].When) {
			return out[i].When.After(out[j].When)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// transcriptLine is the part of a transcript record the picker reads.
type transcriptLine struct {
	Type        string `json:"type"`
	IsSidechain bool   `json:"isSidechain"`
	IsMeta      bool   `json:"isMeta"`
	GitBranch   string `json:"gitBranch"`
	LastPrompt  string `json:"lastPrompt"`
	Message     struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// readConvoMeta fills in what a reader recognizes a conversation by:
// the branch it was on and the last thing asked of it. It reads
// backwards from the end and takes the first answer it finds — many
// files are read on one keystroke, so it stops as soon as it has both.
func readConvoMeta(path string, c *conversation) {
	lines, err := tailLines(path, convoTail)
	if err != nil {
		return
	}
	for i := len(lines) - 1; i >= 0; i-- {
		var rec transcriptLine
		if err := json.Unmarshal(lines[i], &rec); err != nil {
			continue
		}
		if rec.IsSidechain {
			continue
		}
		if c.Branch == "" {
			c.Branch = rec.GitBranch
		}
		if c.Prompt == "" && rec.Type == "last-prompt" {
			c.Prompt = flatten(rec.LastPrompt)
		}
		if c.Prompt == "" && rec.Type == "user" && !rec.IsMeta {
			c.Prompt = userPrompt(rec.Message.Content)
		}
		if c.Branch != "" && c.Prompt != "" {
			return
		}
	}
}

// userPrompt is the text of a user record, or "" if it is not something
// its user typed: a tool result arrives as a list rather than a string,
// and the harness delivers reminders and command output as text wrapped
// in tags.
func userPrompt(content json.RawMessage) string {
	var text string
	if err := json.Unmarshal(content, &text); err != nil {
		return ""
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "<") || strings.HasPrefix(text, "Caveat:") {
		return ""
	}
	return flatten(text)
}

// flatten lays prose on one line, the way a row must read.
func flatten(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// tailLines returns the complete lines in the last max bytes of a
// file. The first line read is dropped unless the read started at the
// beginning, because it is whatever the seek landed in the middle of.
func tailLines(path string, max int64) ([][]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	from := info.Size() - max
	if from < 0 {
		from = 0
	}
	if _, err := f.Seek(from, io.SeekStart); err != nil {
		return nil, err
	}

	b, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	lines := bytes.Split(b, []byte("\n"))
	if from > 0 && len(lines) > 0 {
		lines = lines[1:]
	}
	return lines, nil
}
