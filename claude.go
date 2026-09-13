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

// Claude Code leaves a suspended session's transcript behind when its
// instance exits: <claude>/projects/<encoded cwd>/<session id>.jsonl.
// It is enough to pick the session back up, which is what A offers:
// every one of them at a project, newest first, filtered the way the
// list is, and not offering one a live instance already has — vetted
// against the process table, so a session file an ended instance left
// behind cannot hide one still going.

// session is a talk claude had in a directory and could pick back
// up: its transcript is on disk, and no live instance is carrying it.
type session struct {
	ID     string
	Dir    string    // where it was had, which is where resuming belongs
	When   time.Time // when it last moved
	Branch string
	Prompt string // the last thing its user asked of it
	// What the instance is waiting on, read only for one that is, and
	// the moment its status became what it is when that was read, so
	// the transcript is read again when the status changes and not
	// on a beat.
	Ask   ask
	AskAt time.Time
}

// claudeConfigDir is where Claude Code keeps its state — the sessions
// and the transcripts the sessions view reads, distinct from claudeDir
// in theme.go, which is that same directory's name relative to home,
// for what conn writes there.
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

// insideNote is what conn tells a contact it starts about where it is.
// A contact that backgrounds a dev server leaves it with no terminal:
// no pane to attach to, no scrollback to read, and its output wherever
// the contact happened to send it. A window of its own costs the
// contact nothing and makes the server a process like any other here —
// conn shows it, you reach it, and its log is the pane you are looking
// at.
//
// conn says this to the contacts it starts rather than writing it into
// anybody's settings. It travels with conn, so a conn on another
// machine tells its contacts the same thing, and a machine conn is gone
// from is as conn found it.
func insideNote(socket string) string {
	return "You are running inside conn, which holds this terminal as a tmux pane " +
		"and watches the processes working this project. Start anything long-lived " +
		"— a dev server, a file watcher, a build that stays up — in a window of its " +
		"own rather than detached in the background:\n\n" +
		"  tmux -S " + socket + " new-window -d -n NAME -c DIR 'COMMAND'\n\n" +
		"It then holds a terminal of its own, so conn lists it as a process " +
		"of its own, it can be attached to, and its output is the window's scrollback:\n\n" +
		"  tmux -S " + socket + " capture-pane -p -t NAME\n\n" +
		"Something you background instead holds no terminal and has no pane, and can " +
		"only be read through whatever file its output was sent to."
}

// contactCommand is what conn runs to start a contact: the program,
// told where it is.
func contactCommand(socket string) string {
	return contactProgram + " --append-system-prompt " + shellQuote(insideNote(socket))
}

// resumeCommand is the command that picks a suspended session back
// up, told the same. The id travels onto a shell command line, so only
// ids claudeSuspended vetted are ever handed here.
func resumeCommand(socket, id string) string { return contactCommand(socket) + " --resume " + id }

// What Claude Code calls itself, in the file it keeps per instance.
// The vocabulary is closed at four, and these are all of them, read
// off the validator the file is parsed by rather than off whichever
// ones happened to be caught being written.
//
// Busy is mid-turn. Shell is a command of its own running, which is
// work too, and work conn could not otherwise see: a contact waiting on
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
// session the instance is carrying, and whether it is working on
// it this moment. Small enough to read on every reading of the table.
type sessionFile struct {
	SessionID string `json:"sessionId"`
	Status    string `json:"status"`
	// When the status last became what it is, in milliseconds since the
	// epoch. Claude writes the file on a change rather than on a clock,
	// so this is the moment of the change and holds still between them.
	StatusUpdatedAt int64 `json:"statusUpdatedAt"`
	// What a waiting instance is stopped on, in its own words: a short
	// phrase from a closed set — input needed, dialog open, goal proposal,
	// sandbox request. The processes view has no column wide enough for
	// it; the readout does.
	WaitingFor string `json:"waitingFor"`
	// What else an instance says of itself, which the readout reports and
	// nothing else reads: the name it goes by, what it was built as,
	// and whether it is somebody's own session or something running
	// behind one.
	Name    string `json:"name"`
	Version string `json:"version"`
	Kind    string `json:"kind"`
	// Where the instance was started, which is where its transcript is
	// filed, and when its process began, which is how a file left
	// behind by a dead pid is told from the live one that now has it.
	Cwd       string `json:"cwd"`
	ProcStart string `json:"procStart"`
}

// procStartLayout is how Claude writes the moment its process began:
// the C library's ctime, in UTC. Read off a live file against ps on a
// Pacific machine: the file said 16:43:50 and ps said 09:43:50.
const procStartLayout = "Mon Jan _2 15:04:05 2006"

// wroteBy says whether this file was written by a process that began
// at the given moment. A session file is named by pid and outlives the
// process that wrote it, and a pid comes round again; the file says
// when its process began, and a process that began at another time is
// another process. A file too old to say is believed only if its
// status changed after the process began, since a file written before
// a process existed cannot be about it.
func (f sessionFile) wroteBy(started time.Time) bool {
	if started.IsZero() {
		return true
	}
	if f.ProcStart != "" {
		at, err := time.ParseInLocation(procStartLayout, f.ProcStart, time.UTC)
		if err != nil {
			return true
		}
		d := at.Sub(started)
		return d > -2*time.Second && d < 2*time.Second
	}
	if f.StatusUpdatedAt > 0 {
		return !time.UnixMilli(f.StatusUpdatedAt).Before(started.Truncate(time.Second))
	}
	return true
}

// An ask is what a waiting contact wants, in its own words, read off
// the end of its transcript. The session file says only that it is
// stopped and on what sort of thing, one phrase from a closed set; the
// transcript has the thing itself: the tool it asked to use and has no
// answer for yet, or, with nothing pending, the last thing it said,
// which is the question when a turn ended on one.
type ask struct {
	Tool   string // the tool waiting on an answer, as Claude names it
	Detail string // what it asked to do with it, in its own words
	Said   string // the last thing it said, when nothing is pending
}

// String is the ask on one line: the tool and what it asked, or what
// was said, or nothing.
func (a ask) String() string {
	switch {
	case a.Tool != "" && a.Detail != "":
		return a.Tool + " · " + a.Detail
	case a.Tool != "":
		return a.Tool
	default:
		return a.Said
	}
}

// askDetail is what a tool call is about, in one line, from the
// arguments Claude gave it: the question a question tool asks, or the
// first of the arguments that says what the call is for.
func askDetail(input map[string]json.RawMessage) string {
	if raw, ok := input["questions"]; ok {
		var qs []struct {
			Question string `json:"question"`
		}
		if json.Unmarshal(raw, &qs) == nil && len(qs) > 0 && qs[0].Question != "" {
			return flatten(qs[0].Question)
		}
	}
	for _, k := range []string{"description", "prompt", "command", "file_path", "path", "url", "pattern", "query"} {
		var v string
		if raw, ok := input[k]; ok && json.Unmarshal(raw, &v) == nil && v != "" {
			return flatten(v)
		}
	}
	return ""
}

// readAsk reads the end of a transcript for what the contact is waiting
// on: the tool uses of its last turn, less the ones that have been
// answered, or what it last said. A turn is written as several records,
// the text and each tool use on a line of its own, so the reading
// walks back through all of them, to the prompt that began the turn.
func readAsk(path string) ask {
	lines, err := tailLines(path, sessionTail)
	if err != nil {
		return ask{}
	}
	answered := map[string]bool{}
	var a ask
	for i := len(lines) - 1; i >= 0; i-- {
		var rec struct {
			Type        string `json:"type"`
			IsSidechain bool   `json:"isSidechain"`
			Message     struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(lines[i], &rec) != nil || rec.IsSidechain {
			continue
		}
		var items []struct {
			Type      string                     `json:"type"`
			ID        string                     `json:"id"`
			ToolUseID string                     `json:"tool_use_id"`
			Name      string                     `json:"name"`
			Text      string                     `json:"text"`
			Input     map[string]json.RawMessage `json:"input"`
		}
		switch rec.Type {
		case "user":
			// A prompt typed by a person is a string and begins the
			// turn; anything earlier is another turn. A list is tool
			// results, which answer tool uses.
			if json.Unmarshal(rec.Message.Content, &items) != nil {
				return a
			}
			for _, it := range items {
				if it.Type == "tool_result" {
					answered[it.ToolUseID] = true
				}
			}
		case "assistant":
			if json.Unmarshal(rec.Message.Content, &items) != nil {
				continue
			}
			for _, it := range items {
				switch it.Type {
				case "tool_use":
					if !answered[it.ID] && a.Tool == "" {
						a.Tool, a.Detail = it.Name, askDetail(it.Input)
					}
				case "text":
					if t := flatten(it.Text); t != "" && a.Said == "" {
						a.Said = t
					}
				}
			}
			// A tool use with no answer is the ask; what was said
			// around it is not.
			if a.Tool != "" {
				a.Said = ""
				return a
			}
		}
	}
	return a
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

// contactStatuses is what every contact says of itself: working, or
// stopped and waiting on you. A contact is asked rather than measured —
// it knows whether it is mid-turn, where the processor time it happens
// to be using says little, a model answering being barely any and
// waiting on you none at all.
//
// A contact that has stopped has not necessarily stopped on anything:
// a turn that is simply over asks nothing and holds nothing up, while
// a permission or a question is a thing sitting there unanswered. Only
// the second is worth a word that carries, so the two are kept apart
// here rather than both being called waiting.
//
// A file can outlive the process that wrote it, so a pid counts only
// where the table still has it status as a contact; a contact with no
// file to read - another maker's, or one too old to write one - says
// nothing of itself, and reads as alive like anything else.
func contactStatuses(procs []process) map[int]status {
	byPid := map[int]process{}
	for _, p := range procs {
		byPid[p.pid] = p
	}
	how := map[int]status{}
	for pid, s := range claudeSessions() {
		p, ok := byPid[pid]
		if !ok || kindOf(p) != kindContact || s.Status == "" || !s.wroteBy(p.started) {
			continue
		}
		var since time.Time
		if s.StatusUpdatedAt > 0 {
			since = time.UnixMilli(s.StatusUpdatedAt)
		}
		switch s.Status {
		case busyStatus, shellStatus:
			how[pid] = status{working: true, since: since}
		case waitingStatus:
			how[pid] = status{waiting: true, since: since, asking: s.WaitingFor}
		case idleStatus:
			how[pid] = status{idle: true, since: since}
		}
		// A word outside the four is a Claude newer than this conn, and
		// conn says nothing of a contact it cannot understand — the same
		// as a contact with no file at all, which is the honest answer
		// and already has a word. Idle especially is not the answer to
		// guess: it says at rest, nothing pending, yours when you want
		// it, and none of that is known.
	}
	return how
}

// liveSessions is the id of every session a running claude
// instance is carrying. A session file can outlive the process that
// wrote it, so a pid is only believed when the process table still has
// it, status as a contact.
func liveSessions(projects []project) map[string]bool {
	began := map[int]time.Time{}
	for _, pl := range projects {
		for _, e := range pl.entries {
			if e.kind == kindContact {
				began[e.pid] = e.started
			}
		}
	}
	live := map[string]bool{}
	for pid, f := range claudeSessions() {
		if at, ok := began[pid]; ok && f.SessionID != "" && f.wroteBy(at) {
			live[f.SessionID] = true
		}
	}
	return live
}

// sessionTail is how much of a transcript's end is read for the
// sessions view: enough to reach back past a tool-heavy turn to the
// last prompt, small enough that a directory of them is read on a
// keystroke.
const sessionTail = 256 * 1024

// claudeSuspended lists the sessions at rest under the given
// directories, newest first, excluding the ones a live instance is
// carrying.
func claudeSuspended(dirs []string, projects []project) []session {
	live := liveSessions(projects)
	root := filepath.Join(claudeConfigDir(), "projects")

	// Claude encodes directories lossily, so two of them can share a
	// transcript directory; each session is taken once, for the
	// first directory that reached it.
	seen := map[string]bool{}
	var out []session
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
			c := session{ID: id, Dir: dir, When: info.ModTime()}
			readSessionMeta(filepath.Join(root, encodePath(dir), e.Name()), &c)
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

// transcriptLine is the part of a transcript record the sessions view
// reads.
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

// readSessionMeta fills in what a reader recognizes a session by:
// the branch it was on and the last thing asked of it. It reads
// backwards from the end and takes the first answer it finds — many
// files are read on one keystroke, so it stops as soon as it has both.
func readSessionMeta(path string, c *session) {
	lines, err := tailLines(path, sessionTail)
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
