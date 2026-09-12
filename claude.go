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

// insideNote is what conn tells an agent it starts about where it is.
// An agent that backgrounds a dev server leaves it with no terminal:
// no pane to attach to, no scrollback to read, and its output wherever
// the agent happened to send it. A window of its own costs the agent
// nothing and makes the server a process like any other here — conn
// shows it, you reach it, and its log is the pane you are looking at.
//
// conn says this to the agents it starts rather than writing it into
// anybody's settings. It travels with conn, so a conn on another
// machine tells its agents the same thing, and a machine conn is gone
// from is as conn found it.
func insideNote(socket string) string {
	return "You are running inside conn, which holds this terminal as a tmux pane " +
		"and watches the processes working this project. Start anything long-lived " +
		"— a dev server, a file watcher, a build that stays up — in a window of its " +
		"own rather than detached in the background:\n\n" +
		"  tmux -S " + socket + " new-window -d -n NAME -c DIR 'COMMAND'\n\n" +
		"It then holds a terminal of its own, so it stands on conn's watch as its " +
		"own row, it can be attached to, and its output is the window's scrollback:\n\n" +
		"  tmux -S " + socket + " capture-pane -p -t NAME\n\n" +
		"Something you background instead holds no terminal and has no pane, and can " +
		"only be read through whatever file its output was sent to."
}

// agentCommand is what conn runs to start an agent: the program, told
// where it is.
func agentCommand(socket string) string {
	return agentProgram + " --append-system-prompt " + shellQuote(insideNote(socket))
}

// resumeCommand is the command that picks a suspended conversation back
// up, told the same. The id travels onto a shell command line, so only
// ids claudeSuspended vetted are ever handed here.
func resumeCommand(socket, id string) string { return agentCommand(socket) + " --resume " + id }

// What Claude Code calls itself, in the file it keeps per instance.
// The vocabulary is closed at four, and these are all of them, read
// off the validator the file is parsed by rather than off whichever
// ones happened to be caught being written.
//
// Busy is mid-turn. Shell is a command of its own running, which is
// work too, and work conn could not otherwise see: an agent waiting on
// its own child spends no processor time, so a test suite running ten
// minutes would read at rest the whole way. Claude Code's own status
// line makes the same pair - busy and shell both come out as its word
// for working - so this is its reading rather than a guess at it.
// Waiting is stopped on something put to its user and unable to go on
// without it. Idle is a turn that is over, asking nothing and holding
// nothing up.
const (
	busyStatus    = "busy"
	shellStatus   = "shell"
	waitingStatus = "waiting"
	idleStatus    = "idle"
)

// sessionFile is the part conn reads of what Claude Code writes for
// each instance it is running, at sessions/<pid>.json: which
// conversation the instance is carrying, and whether it is working on
// it this moment. Small enough to read on every reading of the table.
type sessionFile struct {
	SessionID string `json:"sessionId"`
	Status    string `json:"status"`
	// When the status last became what it is, in milliseconds since the
	// epoch. Claude writes the file on a change rather than on a clock,
	// so this is the moment of the change and holds still between them.
	StatusUpdatedAt int64 `json:"statusUpdatedAt"`
	// What a waiting instance is stopped on, in its own words: a short
	// phrase from a closed set — input needed, dialog open, goal
	// proposal, sandbox request. The watch has no column wide enough
	// for it; the look does.
	WaitingFor string `json:"waitingFor"`
	// What else an instance says of itself, which the look reports and
	// nothing else reads: the name it goes by, what it was built as,
	// and whether it is somebody's own session or something running
	// behind one.
	Name    string `json:"name"`
	Version string `json:"version"`
	Kind    string `json:"kind"`
}

// claudeSessions is what every claude instance says of itself, by the
// pid it says it is. A file here can outlive the process that wrote
// it, so callers pair a pid with the process table before believing
// anything of it.
func claudeSessions() map[int]sessionFile {
	entries, err := os.ReadDir(filepath.Join(claudeConfigDir(), "sessions"))
	if err != nil {
		return nil
	}
	out := map[int]sessionFile{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSuffix(name, ".json"))
		if err != nil {
			continue
		}
		b, err := os.ReadFile(filepath.Join(claudeConfigDir(), "sessions", name))
		if err != nil {
			continue
		}
		var f sessionFile
		if json.Unmarshal(b, &f) == nil {
			out[pid] = f
		}
	}
	return out
}

// agentStandings is what every agent says of itself: working, or
// stopped and waiting on you. An agent is asked rather than measured —
// it knows whether it is mid-turn, where the processor time it happens
// to be using says little, a model answering being barely any and
// waiting on you none at all.
//
// An agent that has stopped has not necessarily stopped on anything:
// a turn that is simply over asks nothing and holds nothing up, while
// a permission or a question is a thing sitting there unanswered. Only
// the second is worth a word that carries, so the two are kept apart
// here rather than both being called waiting.
//
// A file can outlive the process that wrote it, so a pid counts only
// where the table still has it standing as an agent; an agent with no
// file to read - another maker's, or one too old to write one - says
// nothing of itself, and reads as alive like anything else.
func agentStandings(procs []process) map[int]standing {
	kind := map[int]string{}
	for _, p := range procs {
		kind[p.pid] = kindOf(p)
	}
	how := map[int]standing{}
	for pid, s := range claudeSessions() {
		if kind[pid] != kindAgent || s.Status == "" {
			continue
		}
		var since time.Time
		if s.StatusUpdatedAt > 0 {
			since = time.UnixMilli(s.StatusUpdatedAt)
		}
		switch s.Status {
		case busyStatus, shellStatus:
			how[pid] = standing{working: true, since: since}
		case waitingStatus:
			how[pid] = standing{waiting: true, since: since, asking: s.WaitingFor}
		case idleStatus:
			how[pid] = standing{idle: true, since: since}
		}
		// A word outside the four is a Claude newer than this conn, and
		// conn says nothing of an agent it cannot understand — the same
		// as an agent with no file at all, which is the honest answer
		// and already has a word. Idle especially is not the answer to
		// guess: it says at rest, nothing pending, yours when you want
		// it, and none of that is known.
	}
	return how
}

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
	live := map[string]bool{}
	for pid, f := range claudeSessions() {
		if pids[pid] && f.SessionID != "" {
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
