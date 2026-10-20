package main

import (
	"cmp"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	term "github.com/charmbracelet/x/term"
	"github.com/w0zro/conn/internal/shell"
	"github.com/w0zro/conn/internal/theme"
	"github.com/w0zro/conn/internal/tmux"
)

// conn holds a tmux server of its own, with one session. Its first
// window is conn; every process conn starts is a window beside it, and
// ctrl-space comes back to conn from any of them. tmux is left as it
// comes otherwise, prefix and all.
const (
	sessionName = "conn"
	connWindow  = "conn"
	backKey     = "C-Space"
	// The mark on a pane conn started a conversation in: its id.
	sessionMark = "@conn_session"
	// The number of conversations waiting on you, for the status line.
	waitingMark = "@conn_waiting"
	// The number of conversations that finished their turn, beside it.
	finishedMark = "@conn_finished"
	// The number of runs that failed, beside it.
	failedMark = "@conn_failed"
)

// socketPath is where conn's server listens: CONN_SOCKET, or tmux.sock
// in conn's state directory.
func socketPath(home string) string {
	if p := os.Getenv("CONN_SOCKET"); p != "" {
		return p
	}
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		state = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(state, "conn", "tmux.sock")
}

// configDir is where conn's configuration is kept: conn in
// XDG_CONFIG_HOME, or in ~/.config.
func configDir(home string) string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "conn")
	}
	return filepath.Join(home, ".config", "conn")
}

// themesDir is where theme sheets are kept, a sheet a theme.
func themesDir(home string) string {
	return filepath.Join(configDir(home), "themes")
}

// readConfig is what config.json says, and nothing where there is no
// file.
func readConfig(home string) (config, error) {
	var c config
	path := filepath.Join(configDir(home), "config.json")
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	if c.Ground != "" && c.Ground != "dark" && c.Ground != "light" {
		return c, fmt.Errorf("%s: the ground is dark or light, not %q", path, c.Ground)
	}
	return c, nil
}

// A config is what config.json says about a server coming up: the
// theme it wears, and the ground, dark or light, where the operator
// wants the same one on every terminal rather than the terminal's own.
type config struct {
	Theme  string `json:"theme"`
	Ground string `json:"ground"`
}

// freshMode is the mode a server comes up on: the theme a flag names,
// or config.json, or conn's; and the ground a flag names, or
// config.json, or the terminal's own, asked only then.
func freshMode(home string, want theme.Want, detect func() bool) (theme.Mode, error) {
	c, err := readConfig(home)
	if err != nil {
		return theme.Mode{}, err
	}
	m := theme.Mode{Theme: cmp.Or(want.Theme, c.Theme, theme.Conn.Name)}
	switch {
	case want.Dark != nil:
		m.Dark = *want.Dark
	case c.Ground != "":
		m.Dark = c.Ground == "dark"
	default:
		m.Dark = detect()
	}
	return m, nil
}

// findServer is conn's server on this machine's tmux.
func findServer(home string) (*tmux.Server, error) {
	bin, err := exec.LookPath("tmux")
	if err != nil {
		return nil, fmt.Errorf("conn runs in tmux, and there is no tmux on the PATH")
	}
	return &tmux.Server{Tmux: bin, Socket: socketPath(home)}, nil
}

// insideConn says whether this process runs in a pane of conn's server:
// tmux tells its panes the socket in TMUX, before the first comma.
func insideConn(socket string) bool {
	sock, _, _ := strings.Cut(os.Getenv("TMUX"), ",")
	return sock != "" && filepath.Clean(sock) == filepath.Clean(socket)
}

// generation names this conn's arrangement of the server. A server
// arranged by an earlier conn is not this one's to rearrange.
const generation = "2"

// conf is the server's configuration on a ground, written beside the
// socket on each start and sourced into a server already up. The key
// back to conn is bound once conn's window is known, by its id: a
// window's name is whatever runs in it, unless it was given one.
func conf(g theme.Ground) string {
	return `# conn's tmux server. Written by conn on each start; edits do not keep.
set -g @conn_generation ` + generation + `
` + look(g) + `
set -g prefix C-b
set -g mouse on
set -g mode-keys vi
set -g history-limit 10000
set -g escape-time 10
set -g focus-events on
set -g set-clipboard on
set -g default-terminal tmux-256color
set -as terminal-features ",*:RGB"
set-environment -g COLORTERM truecolor
set-environment -g CLAUDE_CODE_TMUX_TRUECOLOR 1
set-environment -g CONN 1
` + statusLine(g)
}

// statusLine is the two rows across the foot of the window. The upper
// is the band, on the border: the wordmark, and at the right how many
// conversations wait on you, in the accent. The lower is the key bar:
// the keys that work where the cursor is while the keys are on the
// panel, lit in the accent's block, so being in conn shows from
// anywhere on the screen; on the surface, the way back while
// they are in a pane, and tmux's own keys while a pane is in copy
// mode, which tmux knows first. The panel writes its keys into an
// option when they change, on a keypress, never on a beat.
func statusLine(g theme.Ground) string {
	var b strings.Builder
	b.WriteString(`set -g status 2
set -g status-interval 0
set -g status-justify left
set -g status-left-length 200
set -g status-right-length 100
# The panel lists what runs; tmux's own list of windows would say it
# again, by window.
set -g window-status-format ""
set -g window-status-current-format ""
set -g window-status-separator ""
`)
	fmt.Fprintf(&b, "set -g status-style \"bg=%s,fg=%s\"\n", g.Border, g.Gray)
	fmt.Fprintf(&b, "set -g status-left \"#[fg=%s bold] conn #[default]\"\n", theme.Hex(g.Ink))
	fmt.Fprintf(&b, "set -g status-right \"#{?#{%s},#[fg=%s bold]#{%s} waiting #[default],}#{?#{%s},#[fg=%s bold]#{%s} finished #[default],}#{?#{%s},#[fg=%s bold]#{%s} failed #[default],}\"\n", waitingMark, g.Accent, waitingMark, finishedMark, theme.Hex(g.Ink), finishedMark, failedMark, g.Scheme[1], failedMark)
	fmt.Fprintf(&b, "set -g set-titles on\nset -g set-titles-string \"conn#{?#{%s}, · #{%s} waiting,}#{?#{%s}, · #{%s} failed,}\"\n", waitingMark, waitingMark, failedMark, failedMark)
	b.WriteString("set -g status-format[0] \"#[align=left]#{T:status-left}#[align=right]#{T:status-right}\"\n")
	onPanel := "#{==:#{pane_id},#{" + panelIDMark + "}}"
	fmt.Fprintf(&b, "set -g status-format[1] \"#[fill=%s bg=%s]#{?pane_in_mode,%s,#{?%s,#[fill=%s bg=%s]#{%s},%s}}\"\n", g.Surface, g.Surface, keyBar(copyHints, g), onPanel, g.Block, g.Block, barMark, keyBar(awayHints, g))
	return b.String()
}

// The marks the key bar is written by: the panel's pane, so the bar
// knows when the keys are on it, and the keys it says there.
const (
	panelIDMark = "@conn_panel_id"
	barMark     = "@conn_bar"
)

// What the bar says while a pane is in copy mode: tmux's own keys in
// vi mode, which is the mode conn sets; and while the keys are in a
// pane. The words have no comma in them, since the bar is written
// inside a conditional of the format's own and a comma there is the
// conditional's.
var (
	copyHints = []keyHint{{"n N", "next and previous"}, {"/ ?", "search down and up"}, {"v y", "select and copy"}, {"q", "leave"}}
	awayHints = []keyHint{{"ctrl-space", "back to conn"}}
)

// keyBar is the keys as the bar writes them while they are in a pane:
// each key in the ink and bold, what it does in the gray after it, on
// the surface. connBar is the keys while they are on the panel, the bar
// lit in the accent's block, so conn's keys are told from a pane's at
// a glance: each key in the block's ink and bold, what it does in the
// same ink.
func keyBar(hints []keyHint, g theme.Ground) string {
	return bar(hints, g.Surface, theme.Hex(g.Ink), g.Gray)
}

func connBar(hints []keyHint, g theme.Ground) string {
	return bar(hints, g.Block, g.OnBlock, g.OnBlock)
}

// bar is the keys on a ground: three cells between one and the next,
// a cell in from the edge.
func bar(hints []keyHint, ground, key, does string) string {
	var b strings.Builder
	b.WriteString(" ")
	for i, h := range hints {
		if i > 0 {
			b.WriteString("   ")
		}
		b.WriteString(tmux.Styled(tmux.Style{FG: key, BG: ground, Bold: true}, h.key+" "))
		b.WriteString(tmux.Styled(tmux.Style{FG: does, BG: ground}, h.does))
	}
	return b.String()
}

// look is what the server wears on a ground: the panes on the ground in
// the ink, the sixteen a program asks for by name, the cursor in the
// accent, a selection on the chosen ground, a search's matches in a
// quiet mark with the one the cursor is on in the accent's block, and
// the border between the panel and the workspace painted in the
// ground, so the seam is a column of air and not a line.
func look(g theme.Ground) string {
	var b strings.Builder
	ground, ink := theme.Hex(g.Ground), theme.Hex(g.Ink)
	fmt.Fprintf(&b, "set -g window-style \"bg=%s,fg=%s\"\n", ground, ink)
	fmt.Fprintf(&b, "set -g cursor-colour \"%s\"\n", g.Cursor)
	// The cursor keeps its color in copy mode. tmux draws a pane in a
	// mode from a screen of the mode's own, which carries no cursor
	// colour, and sends the terminal a reset the moment the mode comes
	// on. The option is applied to whichever screen a pane is showing
	// at the time, so it is applied again, a beat after the mode has
	// changed, when the mode's screen is the one showing, and unset
	// again so no pane carries a colour of its own past the theme's.
	fmt.Fprintf(&b, "set-hook -g pane-mode-changed \"run-shell -b -d 0 -C \\\"set -p -t '#{hook_pane}' cursor-colour '%s' ; set -pu -t '#{hook_pane}' cursor-colour\\\"\"\n", g.Cursor)
	fmt.Fprintf(&b, "set -g mode-style \"bg=%s,fg=%s\"\n", g.Chosen, g.ChosenInk)
	fmt.Fprintf(&b, "set -g copy-mode-match-style \"bg=%s,fg=%s\"\n", g.Gray, ground)
	fmt.Fprintf(&b, "set -g copy-mode-current-match-style \"bg=%s,fg=%s,bold\"\n", g.Block, g.OnBlock)
	for i, c := range g.Scheme {
		fmt.Fprintf(&b, "set -g pane-colours[%d] \"%s\"\n", i, c)
	}
	b.WriteString("set -g pane-border-lines single\n")
	fmt.Fprintf(&b, "set -g pane-border-style \"fg=%s,bg=%s\"\n", ground, ground)
	fmt.Fprintf(&b, "set -g pane-active-border-style \"fg=%s,bg=%s\"\n", ground, ground)
	b.WriteString("set -g pane-border-indicators off\n")
	return b.String()
}

// ground is the ground the server is on: the theme it chose when it
// came up, on the ground it chose, and conn's dark where it has not
// chosen. A sheet gone since is conn's own.
func ground(home, socket string) theme.Ground {
	m, ok := theme.ReadMode(socket)
	if !ok {
		return theme.Dark
	}
	th, err := theme.Load(themesDir(home), m.Theme)
	if err != nil {
		th = theme.Conn
	}
	return th.On(m.Dark)
}

// oscColors asks the terminal to take the ink of a ground and the
// padding for its own, and oscOwnColors gives it its own back. tmux
// keeps what a pane asks for to the pane, so the padding around the
// client is the terminal's own unless the conn that attaches asks,
// which is also the conn there to take it back. The padding is black
// on either ground: the terminal's own edge, with the frame standing on
// it. The cursor is the other way about: tmux puts the server's on the
// terminal and leaves it there when the client goes, so conn asks for
// nothing and takes it back all the same.
func oscColors(g theme.Ground) string {
	return fmt.Sprintf("\x1b]10;%s\x1b\\\x1b]11;%s\x1b\\", theme.Hex(g.Ink), paddingHex)
}

const (
	paddingHex   = "#000000"
	oscOwnColors = "\x1b]110\x1b\\\x1b]111\x1b\\\x1b]112\x1b\\"
)

// errEarlier is a server up under an earlier conn's arrangement.
var errEarlier = fmt.Errorf("an earlier conn's server is up")

// attach brings the server up if it is down, with conn in its first
// window, and puts this terminal on it until the client leaves. A
// server already up gets the configuration again, and conn's window
// again where it is gone or runs another build; the keys stay where
// they were left in it, on the panel or in the process beside it.
// want is what the flags asked for.
func attach(srv *tmux.Server, self, home string, want theme.Want) (int, error) {
	if err := os.MkdirAll(filepath.Dir(srv.Socket), 0o700); err != nil {
		return 0, err
	}
	g, again, err := chooseGround(home, srv.Socket, srv.HasSession(sessionName), want, theme.DetectDark)
	if err != nil {
		return 0, err
	}
	theme.RefreshClaudeTheme(home, g)
	path := filepath.Join(filepath.Dir(srv.Socket), "tmux.conf")
	if err := os.WriteFile(path, []byte(conf(g)), 0o600); err != nil {
		return 0, err
	}
	fmt.Print(oscColors(g))
	defer fmt.Print(oscOwnColors)
	panel := "exec " + shell.Quote(self) + " panel"
	if srv.Up() {
		if g, _ := srv.Global("@conn_generation"); g != generation {
			return 0, errEarlier
		}
		if _, err := srv.Do(tmux.SourceFile(path)); err != nil {
			return 0, err
		}
	}
	if !srv.HasSession(sessionName) {
		// Made at the size it will have, this terminal less the status
		// line: made at any other, the panel can start before the
		// client's size reaches it and miss the change.
		args := []string{"-f", path, "new-session", "-d", "-s", sessionName, "-n", connWindow, "-c", home}
		if w, h, err := term.GetSize(os.Stdout.Fd()); err == nil && h > 1 {
			args = append(args, "-x", strconv.Itoa(w), "-y", strconv.Itoa(h-1))
		}
		if _, err := srv.Run(append(args, panel)...); err != nil {
			return 0, err
		}
		// The pane just made runs this build; said now, it is not
		// started again over a build it already runs.
		if _, err := srv.Do(tmux.SetGlobal(buildMark, fingerprint(self))); err != nil {
			return 0, err
		}
	}
	win, err := ensurePanel(srv, home, self, panel, again)
	if err != nil {
		return 0, err
	}
	own, err := findPanel(srv, "")
	if err != nil {
		return 0, err
	}
	if _, err := srv.Do(bindBack(win, own.pane), tmux.Cmd{"select-window", "-t", win}); err != nil {
		return 0, err
	}
	return srv.AttachClient(path, sessionName, connWindow, home, panel)
}

// bindBack binds the one key conn takes from every window: the keys
// back to the panel, in conn's window. From a pane, the pane the keys
// leave is written down, as tmux fills it in, the keys go to the
// panel, and the key is sent on to the panel, which notes where they
// came from. On the panel already, the key is sent on alone, and the
// panel answers it with the other process: so from inside a process,
// twice over is the process worked in before it.
func bindBack(window, panel string) tmux.Cmd {
	onPanel := "#{==:#{pane_id}," + panel + "}"
	arrive := "send-keys -t " + panel + " " + backKey
	fromPane := "set-option -gF " + fromMark + " '#{pane_id}' ; select-window -t " + window + " ; select-pane -t " + panel + " ; " + arrive
	return tmux.Cmd{"bind", "-n", backKey, "if-shell", "-F", onPanel, arrive, fromPane}
}

// fromMark is the pane the keys were in when ctrl-space brought them
// to the panel, written as the key fires, since by the time the panel
// hears the key it is the pane with the keys and nothing on its side
// can tell where they came from. Read once and cleared.
const fromMark = "@conn_from"

// reground puts the server on the ground a flag names, from a pane of
// its own: the configuration is written and sourced again, and conn's
// pane started again to read it. A pane of the server has no terminal
// of its own to ask, only tmux, which would answer with the ground
// conn set.
func reground(srv *tmux.Server, self, home string, want theme.Want) error {
	g, again, err := chooseGround(home, srv.Socket, true, want, func() bool { return true })
	if err != nil {
		return err
	}
	if err := dressServer(srv, home, g); err != nil {
		return err
	}
	_, err = ensurePanel(srv, "", self, "exec "+shell.Quote(self)+" panel", again)
	return err
}

// dressServer puts a server already up on a ground: the configuration
// written and sourced again, which dresses every pane where it stands
// but conn's own, and Claude Code's theme rewritten.
func dressServer(srv *tmux.Server, home string, g theme.Ground) error {
	theme.RefreshClaudeTheme(home, g)
	path := filepath.Join(filepath.Dir(srv.Socket), "tmux.conf")
	if err := os.WriteFile(path, []byte(conf(g)), 0o600); err != nil {
		return err
	}
	_, err := srv.Do(tmux.SourceFile(path))
	return err
}

// askGround is the ground for a conn that attaches to nothing: what
// the flags say, over what the server is on while it is up, or the
// mode a server would come up on.
func askGround(srv *tmux.Server, home string, want theme.Want) (theme.Ground, error) {
	m, ok := theme.ReadMode(srv.Socket)
	if ok && srv.HasSession(sessionName) {
		if want.Dark != nil {
			m.Dark = *want.Dark
		}
		m.Theme = cmp.Or(want.Theme, m.Theme)
	} else {
		var err error
		if m, err = freshMode(home, want, theme.DetectDark); err != nil {
			return theme.Ground{}, err
		}
	}
	th, err := theme.Load(themesDir(home), m.Theme)
	return th.On(m.Dark), err
}

// chooseGround is the ground the server is on, and whether a server
// already up was just put on another. A server's mode is what it came
// up on, held until it goes down: freshMode's, the first time. A flag
// says it whenever it is given, so a server already up is put on what
// it names where it stands; --theme reads its sheet again even where
// the server wears it already, so a sheet being worked on is seen.
func chooseGround(home, socket string, up bool, want theme.Want, detect func() bool) (g theme.Ground, again bool, err error) {
	m, ok := theme.ReadMode(socket)
	fresh := !ok || !up
	if !fresh && !want.Any() {
		return ground(home, socket), false, nil
	}
	if fresh {
		if m, err = freshMode(home, want, detect); err != nil {
			return theme.Ground{}, false, err
		}
	} else {
		if want.Dark != nil && *want.Dark != m.Dark {
			m.Dark, again = *want.Dark, true
		}
		if want.Theme != "" {
			m.Theme, again = want.Theme, true
		}
	}
	th, err := theme.Load(themesDir(home), m.Theme)
	if err != nil {
		return theme.Ground{}, false, err
	}
	_ = theme.WriteMode(socket, m)
	return th.On(m.Dark), again, nil
}

// ensurePanel answers conn's window: conn's pane started again where
// it has ended, runs another build, or is asked to read the ground
// again; and made where there is none. The configuration sourced
// again dresses every other pane where it stands, but conn's own pane
// read the ground once, when it started.
func ensurePanel(srv *tmux.Server, home, self, panel string, again bool) (string, error) {
	own, err := findPanel(srv, self)
	if err != nil {
		return "", err
	}
	// A rebuild keeps its path, so the build is told by its contents.
	build := fingerprint(self)
	if was, _ := srv.Global(buildMark); was != build {
		own.ours = false
	}
	if own.pane == "" {
		out, err := srv.Run("new-window", "-d", "-t", sessionName+":", "-n", connWindow, "-c", home, "-P", "-F", "#{window_id} #{pane_id}", panel)
		if err != nil {
			return "", err
		}
		own.window, own.pane, _ = strings.Cut(strings.TrimSpace(out), " ")
	} else if own.dead || !own.ours || again {
		if _, err := srv.Do(tmux.Respawn(own.pane, panel)); err != nil {
			return "", err
		}
	}
	_, err = srv.Do(tmux.SetPane(own.pane, panelMark, "1"), tmux.SetGlobal(buildMark, build))
	return own.window, err
}

// buildMark is the build conn's pane runs, by fingerprint.
const buildMark = "@conn_build"

// fingerprint names a binary by its contents.
func fingerprint(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

// panelMark is on conn's own pane. A window's name will not do to find
// it by: a window conn starts is named for its project, and a project
// can be called conn.
const panelMark = "@conn_panel"

// A panelPane is where conn's pane is, whether it has ended, and
// whether it runs this build.
type panelPane struct {
	window, pane string
	dead, ours   bool
}

// findPanel is conn's pane: the one marked as conn's, or, before it is
// marked, the one this build was started in. self is this build, or
// blank where any build's pane will do.
func findPanel(srv *tmux.Server, self string) (panelPane, error) {
	out, err := srv.Run("list-panes", "-s", "-t", sessionName, "-F", "#{window_id} #{pane_id} #{pane_dead} #{"+panelMark+"} #{pane_start_command}")
	if err != nil {
		return panelPane{}, err
	}
	for _, l := range strings.Split(out, "\n") {
		f := strings.SplitN(l, " ", 5)
		if len(f) < 5 {
			continue
		}
		ours := self != "" && strings.Contains(f[4], shell.Quote(self)+" panel")
		if f[3] == "1" || ours {
			return panelPane{window: f[0], pane: f[1], dead: f[2] == "1", ours: ours}, nil
		}
	}
	return panelPane{}, nil
}

// endServer ends the server and everything running in it.
func endServer(srv *tmux.Server) error {
	theme.ForgetMode(srv.Socket)
	return srv.Down()
}

// A pane in conn's server, as the panel reads it.
type pane struct {
	ID      string
	Window  string // the window's id
	Dead    bool
	PID     int       // what the pane runs, as started
	TTY     string    // without /dev/
	Panel   bool      // conn's own
	Hold    bool      // conn's blank, standing in the workspace
	Session string    // the conversation conn started in it
	Name    string    // the name conn u started it under, if any
	Run     string    // what conn u started in it, as it was given
	Role    role      // what it was started as: a check, or the stack
	Exit    string    // how what conn u started ended, once it has
	Ended   time.Time // when it ended, once it has
	Command string    // what tmux says runs in it now
	Dir     string    // where it stands: the project it was started for, else Path
	Path    string    // where what runs in it is now
}

// started says whether conn u started what a pane runs: it has a name,
// or a command of its own under none.
func (p pane) started() bool {
	return p.Name != "" || p.Run != ""
}

// The marks conn sets on the panes conn u starts: the name, if it was
// given one, the command and the directory it was started with, the
// role it was started as, the project it was started for, and how and
// when it ended, which the pane sets itself.
const (
	nameMark    = "@conn_name"
	runMark     = "@conn_run"
	roleMark    = "@conn_role"
	dirMark     = "@conn_dir"
	projectMark = "@conn_project"
	exitMark    = "@conn_exit"
	endedMark   = "@conn_ended"
)

// The fields are separated by a space: tmux turns a tab into an
// underscore when it prints to a pipe in the C locale. Every field but
// the last is one token — a name conn u takes has no space in it — and
// the directory, which can hold a space, is last and taken whole.
const paneFormat = "#{pane_id} #{window_id} #{pane_dead} #{pane_pid} #{pane_tty} #{" + panelMark + "} #{" + holdMark + "} #{" + sessionMark + "} #{" + nameMark + "} #{" + roleMark + "} #{" + exitMark + "} #{" + endedMark + "} #{pane_current_command} #{pane_current_path}"

// The project a pane was started for is a path with spaces of its
// own, as the directory is, and the command it was started with has
// spaces of its own: neither can share a line read by its spaces with
// the directory, and each is read in a line of its own.
const (
	projectFormat = "#{pane_id} #{" + projectMark + "}"
	runFormat     = "#{pane_id} #{" + runMark + "}"
)

// panes is every pane in the server, each where it stands.
func panes(srv *tmux.Server) ([]pane, error) {
	out, err := srv.Run("list-panes", "-s", "-t", sessionName, "-F", paneFormat)
	if err != nil {
		return nil, err
	}
	projects, err := srv.Run("list-panes", "-s", "-t", sessionName, "-F", projectFormat)
	if err != nil {
		return nil, err
	}
	runs, err := srv.Run("list-panes", "-s", "-t", sessionName, "-F", runFormat)
	if err != nil {
		return nil, err
	}
	return withRuns(inProjects(parsePanes(out), byPane(projects)), byPane(runs)), nil
}

// byPane is a mark read a line a pane, by the pane's id, for the
// panes that have it.
func byPane(out string) map[string]string {
	m := map[string]string{}
	for _, l := range strings.Split(out, "\n") {
		if id, project, ok := strings.Cut(l, " "); ok && project != "" {
			m[id] = project
		}
	}
	return m
}

// inProjects puts each pane where it stands: in the project it was
// started for, else where what runs in it is.
func inProjects(ps []pane, projects map[string]string) []pane {
	for i := range ps {
		ps[i].Dir = or(projects[ps[i].ID], ps[i].Path)
	}
	return ps
}

// withRuns gives each pane the command conn u started in it.
func withRuns(ps []pane, runs map[string]string) []pane {
	for i := range ps {
		ps[i].Run = runs[ps[i].ID]
	}
	return ps
}

func parsePanes(out string) []pane {
	var ps []pane
	for _, l := range strings.Split(out, "\n") {
		f := strings.SplitN(l, " ", 14)
		if len(f) < 14 {
			continue
		}
		pid, _ := strconv.Atoi(f[3])
		var ended time.Time
		if at, err := strconv.ParseInt(f[11], 10, 64); err == nil && at > 0 {
			ended = time.Unix(at, 0)
		}
		ps = append(ps, pane{
			ID: f[0], Window: f[1], Dead: f[2] == "1", PID: pid,
			TTY: strings.TrimPrefix(f[4], "/dev/"), Panel: f[5] == "1", Hold: f[6] == "1", Session: f[7],
			Name: f[8], Role: role(f[9]), Exit: f[10], Ended: ended, Command: f[12], Dir: f[13], Path: f[13],
		})
	}
	return ps
}

// openWindow starts a command, or a shell where there is none, in a
// window of its own at a directory, out of sight, marks it with the
// conversation it holds where it holds one, and answers its pane.
func openWindow(srv *tmux.Server, name, dir, cmd, session string) (string, error) {
	args := []string{"new-window", "-d", "-t", sessionName + ":", "-n", name, "-c", dir, "-P", "-F", "#{pane_id}"}
	if cmd != "" {
		args = append(args, cmd) // with none, tmux runs the shell
	}
	id, err := srv.Run(args...)
	if err != nil {
		return "", err
	}
	id = strings.TrimSpace(id)
	if session != "" {
		if _, err := srv.Do(tmux.SetPane(id, sessionMark, session)); err != nil {
			return "", err
		}
	}
	return id, nil
}

// resumeCommand picks a conversation back up in Claude Code, told what
// conn tells the agents it starts, from its file.
func resumeCommand(noteFile, id string) string {
	return readInto("conn_note", noteFile) + "exec claude --append-system-prompt \"$conn_note\" --resume " + shell.Quote(id)
}

// newCommand starts a conversation in Claude Code under an id conn
// chose, so its pane is marked with it before its transcript exists,
// told what conn tells the agents it starts, and with a first prompt
// where there is one. Both reach it from files, read by the pane's
// shell and let go once read: tmux carries a command to its server in
// one message of 16KB, and a project with many names written down is
// told more than that, as a failure handed with its last lines is.
// Claude keeps them from there on.
func newCommand(noteFile, id, promptFile string) string {
	cmd := readInto("conn_note", noteFile) + "exec claude --append-system-prompt \"$conn_note\" --session-id " + shell.Quote(id)
	if promptFile != "" {
		cmd = readInto("conn_prompt", promptFile) + cmd + " \"$conn_prompt\""
	}
	return cmd
}

// readInto is the shell lines that read a file into a variable and let
// the file go.
func readInto(variable, file string) string {
	return variable + "=$(cat " + shell.Quote(file) + ")\nrm -f " + shell.Quote(file) + "\n"
}

// promptPath is where a conversation's first prompt waits for it:
// beside the socket, under the conversation's id.
func promptPath(socket, id string) string {
	return filepath.Join(filepath.Dir(socket), "prompt."+id)
}

// notePath is where what a conversation is told waits for it: beside
// the socket, under the conversation's id.
func notePath(socket, id string) string {
	return filepath.Join(filepath.Dir(socket), "note."+id)
}

// writeNote leaves what a conversation is told where its pane's shell
// reads it, and answers the file.
func writeNote(socket, id, note string) (string, error) {
	file := notePath(socket, id)
	return file, os.WriteFile(file, []byte(note), 0o600)
}

// told is what a conversation started in a project is told: the CLI,
// and what the project's stack and checks are here, as last run
// through conn by whoever ran them. in says which roots are the
// project's.
func told(conn, socket string, in func(string) bool) string {
	starts := readStarted(startedPath(socket))
	return agentNote(conn, established(starts, in, roleStack), established(starts, in, roleCheck))
}

// agentNote is what conn tells the agents it starts: to work the
// project's processes through conn, and what the project runs here. It
// travels with conn rather than being written into anybody's settings.
// conn is how to run this build.
func agentNote(conn string, stack, checks []stacked) string {
	called := ""
	if conn != "conn" {
		called = "conn here is " + conn + "; run it by that path.\n\n"
	}
	here := ""
	if len(stack) > 0 || len(checks) > 0 {
		here = "\n\nIn this project, as last run through conn:\n"
		for _, s := range checks {
			here += "  conn u " + s.name + " -w -- " + s.command + "\n"
		}
		for _, s := range stack {
			here += "  conn u " + s.name + " -- " + s.command + "\n"
		}
		here = strings.TrimRight(here, "\n")
	}
	return "You are running inside conn, which holds this terminal as a tmux pane " +
		"and watches the processes working this project. Work the project's processes " +
		"through conn rather than starting them in the background or finding them with " +
		"ps, lsof and tmux:\n\n" +
		"  conn ls                          this project's processes, and the names they answer to\n" +
		"  conn u -w -- COMMAND             run something and wait: tests, a build, a one-off; its output, and its exit status as conn's\n" +
		"  conn u NAME -- COMMAND           start a dev server, a watcher, anything long-lived, in a pane of its own\n" +
		"  conn u NAME -w -- COMMAND        declare a check the project keeps, and run it: the suite, the build, the lint\n" +
		"  conn l NAME|:PORT|PID [-n N]     what a process said\n" +
		"  conn r NAME|:PORT|PID            restart one conn started\n" +
		"  conn x NAME|:PORT|PID            end one\n\n" + called +
		"A process started this way holds a terminal of its own: conn lists it, the operator " +
		"can go into it, and its output is the pane's. Something you background instead holds " +
		"no terminal and has no pane, and can only be read through whatever file its output was sent to. " +
		"A name is a declaration: what you start under one, conn writes down as what this project runs, " +
		"what waited (-w) as its checks and the rest as its stack, and the operator's keys run them again as you ran them. " +
		"A project has a few checks, the suite, the build, the lint, each named once and run again under " +
		"the name it has, up or not, in the pane it had. Everything else runs under no name: a few test files, " +
		"a one-off, a command you are trying. It is a row while its pane lives, and the project does not keep it. " +
		"Never make up a name for a run, or number one." + here
}

// connCommand is how an agent runs this build: conn, where the PATH
// finds this build by that name, and its whole path otherwise. Setting
// the PATH in tmux's environment would not reach a shell, whose startup
// files build the PATH again.
func connCommand(self string) string {
	if found, err := exec.LookPath("conn"); err == nil && realDir(found) == realDir(self) {
		return "conn"
	}
	return self
}

// newID is a random UUID, the shape Claude Code takes for a session id.
func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// leave lets the terminal go that conn was last used from, and keeps
// everything in conn running. It is named: tmux's current client can
// be conn's own control-mode client, which watches the server.
func leave(srv *tmux.Server) error {
	cs := terminals(srv)
	if len(cs) == 0 {
		return srv.Detach()
	}
	_, err := srv.Do(tmux.Cmd{"detach-client", "-t", cs[0]})
	return err
}

// terminals is the clients on a terminal, not in control mode, the
// last used first.
func terminals(srv *tmux.Server) []string {
	out, err := srv.Run("list-clients", "-F", "#{client_activity} #{client_control_mode} #{client_name}")
	if err != nil {
		return nil
	}
	return parseTerminals(out)
}

// leftWatching detaches the control-mode clients on conn's session.
// Only conn's own pane makes one, to watch the server, and tmux keeps a
// control-mode client on after the process that made it has gone: a
// pane started again, on a new build, leaves its old one behind.
func leftWatching(srv *tmux.Server) {
	out, err := srv.Run("list-clients", "-t", sessionName, "-F", "#{client_control_mode} #{client_name}")
	if err != nil {
		return
	}
	for _, l := range strings.Split(out, "\n") {
		if name, ok := strings.CutPrefix(l, "1 "); ok {
			_, _ = srv.Do(tmux.Cmd{"detach-client", "-t", name})
		}
	}
}

func parseTerminals(out string) []string {
	type client struct {
		at   int
		name string
	}
	var cs []client
	for _, l := range strings.Split(out, "\n") {
		f := strings.SplitN(l, " ", 3)
		if len(f) < 3 || f[1] == "1" {
			continue
		}
		at, _ := strconv.Atoi(f[0])
		cs = append(cs, client{at, f[2]})
	}
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].at > cs[j].at })
	names := make([]string, len(cs))
	for i, c := range cs {
		names[i] = c.name
	}
	return names
}

// windowName is a short name for a window, from a project's name.
func windowName(name string) string {
	if len(name) > 20 {
		name = name[:20]
	}
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == ':' || r == '.' {
			return '-'
		}
		return r
	}, name)
}
