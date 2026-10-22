package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Claude Code keeps a transcript of every conversation it has had, at
// <config>/projects/<the directory, encoded>/<id>.jsonl. That record is
// what conn reads to know where work has happened and what can be
// picked back up: no list of conn's own, nothing to fall out of step.

// claudeDir is where Claude Code keeps its state.
func claudeDir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

// A conversation is one transcript: where it was had, when it last
// moved, and what it is called.
type conversation struct {
	ID      string
	Dir     string
	When    time.Time
	Title   string
	Context int    // the tokens its last turn ran in
	Asks    string // what it is stopped on, where a tool wants a yes or a question waits
	path    string
}

// transcripts is every transcript on the machine, newest first, with
// only what the file system says: id, path and when it last moved.
func transcripts() []conversation {
	root := filepath.Join(claudeDir(), "projects")
	dirs, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []conversation
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(root, d.Name()))
		if err != nil {
			continue
		}
		for _, e := range entries {
			id, ok := strings.CutSuffix(e.Name(), ".jsonl")
			if e.IsDir() || !ok || !isSessionID(id) {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			out = append(out, conversation{ID: id, When: info.ModTime(), path: filepath.Join(root, d.Name(), e.Name())})
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

// isSessionID says whether a transcript's name is shaped like the ids
// Claude Code writes, hex and dashes. The id goes onto a shell line.
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

// record is the part of a transcript line conn reads.
type record struct {
	Type        string `json:"type"`
	Cwd         string `json:"cwd"`
	AITitle     string `json:"aiTitle"`
	CustomTitle string `json:"customTitle"`
	LastPrompt  string `json:"lastPrompt"`
	IsSidechain bool   `json:"isSidechain"`
	Message     struct {
		Usage   usage           `json:"usage"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// usage is what an assistant turn says it read: the tokens fresh,
// written to the cache, and read from it. Together they are the
// context the turn ran in.
type usage struct {
	Input         int `json:"input_tokens"`
	CacheCreation int `json:"cache_creation_input_tokens"`
	CacheRead     int `json:"cache_read_input_tokens"`
}

func (u usage) context() int { return u.Input + u.CacheCreation + u.CacheRead }

// headCwd is the directory a transcript says it was had in: the first
// "cwd" in it. The directory's encoded name cannot be read back into a
// path, since Claude Code turns every character that is not a letter or
// a digit into a dash. A line can be megabytes — a pasted image — so the
// file is searched in pieces rather than parsed a line at a time.
func headCwd(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	r := io.LimitReader(f, cwdSearched)
	buf := make([]byte, 64*1024)
	var carry []byte // the end of the last piece, for a match across two
	for {
		n, err := r.Read(buf)
		piece := append(carry, buf[:n]...)
		if m := cwdField.FindSubmatch(piece); m != nil {
			var dir string
			if json.Unmarshal(append(append([]byte{'"'}, m[1]...), '"'), &dir) == nil {
				return dir
			}
		}
		if err != nil {
			return ""
		}
		carry = append(carry[:0], piece[max(len(piece)-4096, 0):]...)
	}
}

// cwdSearched is how far into a transcript its directory is looked for.
const cwdSearched = 16 << 20

var cwdField = regexp.MustCompile(`"cwd":"((?:[^"\\]|\\.)*)"`)

// titleTail is how much of a transcript's end is read for its title:
// Claude Code writes one after each turn, so it is near the end unless
// a turn's tool output was very long.
const titleTail = 256 * 1024

// tailTitle is what a transcript says its conversation is called: the
// last rename or title Claude Code wrote, and otherwise the last thing
// asked of it.
func tailTitle(path string) string {
	title, _ := tailRead(path)
	return title
}

// tailRead reads a transcript's tail once for what the panel says of
// a conversation: its title, and the context its last turn ran in,
// which the last assistant turn's usage says. Zero where no turn has
// been had.
func tailRead(path string) (title string, context int) {
	title, context, _ = tailReadAsk(path)
	return title, context
}

// tailReadAsk is tailRead and, with it, what the transcript's last
// tool call is asking for, where nothing has answered it yet.
func tailReadAsk(path string) (title string, context int, ask string) {
	lines := tailLines(path, titleTail)
	ask = or(pendingAsk(lines, headCwd(path)), lastSaid(lines))
	prompt := ""
	for i := len(lines) - 1; i >= 0; i-- {
		var r record
		if json.Unmarshal(lines[i], &r) != nil {
			continue
		}
		if context == 0 && r.Type == "assistant" {
			context = r.Message.Usage.context()
		}
		switch {
		case title != "":
		case r.Type == "custom-title" && r.CustomTitle != "":
			title = flatten(r.CustomTitle)
		case r.Type == "ai-title" && r.AITitle != "":
			// A rename is written again just before each title after it.
			title = flatten(r.AITitle)
			if i > 0 {
				var p record
				if json.Unmarshal(lines[i-1], &p) == nil && p.Type == "custom-title" && p.CustomTitle != "" {
					title = flatten(p.CustomTitle)
				}
			}
		case r.Type == "last-prompt" && prompt == "":
			prompt = flatten(r.LastPrompt)
		}
		if title != "" && context != 0 {
			break
		}
	}
	return or(title, prompt), context, ask
}

// block is a piece of a turn's content: a tool call, or the result of
// one.
type block struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
}

// pendingAsk is what the last tool call in a transcript's lines asks,
// if no result has come back for it: "wants to run: rm -rf dist", "wants
// to edit: app/router.ts", or a question's first line. Paths are said
// from dir. Blank where nothing is pending.
func pendingAsk(lines [][]byte, dir string) string {
	var open *block
	for _, l := range lines {
		var r record
		if json.Unmarshal(l, &r) != nil || r.IsSidechain {
			continue
		}
		var blocks []block
		if json.Unmarshal(r.Message.Content, &blocks) != nil {
			continue
		}
		for i, b := range blocks {
			switch {
			case b.Type == "tool_use" && r.Type == "assistant":
				open = &blocks[i]
			case b.Type == "tool_result" && open != nil && b.ToolUseID == open.ID:
				open = nil
			}
		}
	}
	if open == nil {
		return ""
	}
	var in struct {
		Command   string `json:"command"`
		FilePath  string `json:"file_path"`
		Path      string `json:"notebook_path"`
		Questions []struct {
			Question string `json:"question"`
		} `json:"questions"`
	}
	_ = json.Unmarshal(open.Input, &in)
	switch open.Name {
	case "Bash":
		return "wants to run: " + firstLine(in.Command)
	case "Edit", "MultiEdit", "Write", "NotebookEdit":
		return "wants to edit: " + from(dir, or(in.FilePath, in.Path))
	case "AskUserQuestion":
		if len(in.Questions) > 0 {
			return firstLine(in.Questions[0].Question)
		}
	}
	return "wants to use: " + open.Name
}

// textBlock is a piece of an assistant turn's content that says
// something in words.
type textBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// lastSaid is the last line of the last message in a transcript's
// lines, where nothing has been written after it that is not Claude
// Code's own bookkeeping: how a turn handed back ended. Blank where a
// prompt, a tool call or a result came after.
func lastSaid(lines [][]byte) string {
	for i := len(lines) - 1; i >= 0; i-- {
		var r record
		if json.Unmarshal(lines[i], &r) != nil || r.IsSidechain {
			continue
		}
		if r.Type != "assistant" && r.Type != "user" {
			continue
		}
		if r.Type == "user" {
			return ""
		}
		var blocks []textBlock
		if json.Unmarshal(r.Message.Content, &blocks) != nil {
			return ""
		}
		said := ""
		for _, b := range blocks {
			switch b.Type {
			case "tool_use":
				return ""
			case "text":
				said = b.Text
			}
		}
		ls := strings.Split(said, "\n")
		for j := len(ls) - 1; j >= 0; j-- {
			if l := unmarked(flatten(ls[j])); l != "" {
				return l
			}
		}
		return ""
	}
	return ""
}

// The markdown a message is written in, which a line of the panel
// shows as it is: what heads a line (a heading, a quote, a list's
// mark), a link's target, and the marks around emphasis and code.
var (
	lineHead = regexp.MustCompile(`^(#{1,6}\s+|>\s*|[-*+]\s+|\d+[.)]\s+)+`)
	linkTo   = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	emphasis = regexp.MustCompile("\\*\\*|__|`")
)

// unmarked is a line of markdown as words: "- **done** see [the
// docs](x)" is "done see the docs".
func unmarked(l string) string {
	l = lineHead.ReplaceAllString(l, "")
	l = linkTo.ReplaceAllString(l, "$1")
	return strings.TrimSpace(emphasis.ReplaceAllString(l, ""))
}

// firstLine is the first line of s that says anything, its spaces
// squeezed.
func firstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = flatten(l); l != "" {
			return l
		}
	}
	return ""
}

// from is a path said from dir, where it is under it.
func from(dir, path string) string {
	if rel, err := filepath.Rel(dir, path); err == nil && filepath.IsAbs(path) && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return path
}

// tokens says a count of tokens the short way: the thousands, as 262k,
// or the count itself under one.
func tokens(n int) string {
	if n >= 1000 {
		return strconv.Itoa((n+500)/1000) + "k"
	}
	return strconv.Itoa(n)
}

// tailLines is the whole lines in the last n bytes of a file.
func tailLines(path string, n int64) [][]byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil
	}
	from := max(info.Size()-n, 0)
	if _, err := f.Seek(from, io.SeekStart); err != nil {
		return nil
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return nil
	}
	lines := bytes.Split(b, []byte("\n"))
	if from > 0 && len(lines) > 0 {
		lines = lines[1:] // whatever the seek landed in the middle of
	}
	return lines
}

func flatten(s string) string { return strings.Join(strings.Fields(s), " ") }

// A claudeRun is a Claude Code instance running now: the terminal it
// runs on, where it was started, and what it says it is doing.
type claudeRun struct {
	PID    int
	TTY    string
	Cwd    string
	Status string
}

// What a running instance says it is doing, in the four words Claude
// Code writes. Busy is mid-turn; shell is a command of its own running,
// which is work too; waiting is stopped on something put to its user;
// idle is a turn that is over, asking nothing. A word outside the four
// is a newer Claude Code, and conn says nothing of it.
const (
	statusBusy    = "busy"
	statusShell   = "shell"
	statusWaiting = "waiting"
	statusIdle    = "idle"
)

// state is a run's status in the words conn shows: working, waiting,
// idle, or blank where it says nothing conn knows.
func (r claudeRun) state() string {
	switch r.Status {
	case statusBusy, statusShell:
		return "working"
	case statusWaiting:
		return "waiting"
	case statusIdle:
		return "idle"
	}
	return ""
}

// liveClaude is the conversations Claude Code instances are carrying
// right now, by id. Each instance writes <config>/sessions/<pid>.json;
// the file outlives its process, and a pid comes round again, so a file
// counts only while its pid is alive and began when the file says it
// did.
func liveClaude() map[string]claudeRun {
	dir := filepath.Join(claudeDir(), "sessions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	type instance struct {
		SessionID string `json:"sessionId"`
		ProcStart string `json:"procStart"`
		Status    string `json:"status"`
		Cwd       string `json:"cwd"`
	}
	claimed := map[int]instance{}
	for _, e := range entries {
		pid, err := strconv.Atoi(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		if syscall.Kill(pid, 0) != nil {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var in instance
		if json.Unmarshal(b, &in) == nil && in.SessionID != "" {
			claimed[pid] = in
		}
	}
	if len(claimed) == 0 {
		return nil
	}
	pids := make([]string, 0, len(claimed))
	for pid := range claimed {
		pids = append(pids, strconv.Itoa(pid))
	}
	out, _ := exec.Command("ps", "-o", "pid=,tty=,lstart=", "-p", strings.Join(pids, ",")).Output()
	live := map[string]claudeRun{}
	for pid, ps := range parsePS(string(out)) {
		in, ok := claimed[pid]
		if !ok || !beganAt(in.ProcStart, ps.started) {
			continue
		}
		live[in.SessionID] = claudeRun{PID: pid, TTY: ps.tty, Cwd: in.Cwd, Status: in.Status}
	}
	return live
}

type psLine struct {
	tty     string
	started time.Time
}

// parsePS reads ps -o pid=,tty=,lstart=: a pid, a terminal ("??" for
// none), and when the process began, in local time.
func parsePS(out string) map[int]psLine {
	m := map[int]psLine{}
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) < 7 {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		at, _ := time.ParseInLocation("Mon Jan _2 15:04:05 2006", strings.Join(f[2:7], " "), time.Local)
		tty := f[1]
		if tty == "??" || tty == "-" {
			tty = ""
		}
		m[pid] = psLine{tty: tty, started: at}
	}
	return m
}

// beganAt says whether a process that began at started is the one an
// instance file names: Claude Code writes when its process began as
// the C library's ctime, in UTC.
func beganAt(procStart string, started time.Time) bool {
	if procStart == "" || started.IsZero() {
		return true
	}
	at, err := time.ParseInLocation("Mon Jan _2 15:04:05 2006", procStart, time.UTC)
	if err != nil {
		return true
	}
	d := at.Sub(started)
	return d > -2*time.Second && d < 2*time.Second
}
