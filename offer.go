package main

import (
	"os"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/w0zro/conn/internal/shell"
	"github.com/w0zro/conn/internal/tmux"
)

// What the keys start for a role — ss the stack, st the checks — is
// settled in the workspace, not on the panel. The panel starts conn's
// own program for it, conn offer, in a pane of its own, and shows it
// in the workspace with the keys in it, as it shows a row's pane. The
// program lists what is written down, or asks Claude Code where
// nothing is; enter starts each line under its name, as conn u starts
// one, writes it down as what the role means here, and watches the
// starts for a moment (see again.go). Done, it puts the keys back on
// the panel and ends, and the panel, reading its pane ended, puts the
// cursor on the first pane it started. Its pane is marked as a hold:
// no row, and the workspace moving on takes it with it.

// firstMark is set on an offer's pane: the first pane it started, for
// the panel to land on.
const firstMark = "@conn_first"

// offerCommand is how a build runs an offer for a role in a project.
func offerCommand(self string, as role, p project) string {
	return "exec " + shell.Quote(self) + " offer " + string(as) + " " + shell.Quote(p.Root) + " " + shell.Quote(p.Name)
}

// openOffer starts an offer in a window of its own, out of sight,
// where the cursor stands, marked as conn's own, and answers its pane.
func openOffer(srv *tmux.Server, self string, as role, p project, dir string) (string, error) {
	id, err := openWindow(srv, windowName(p.Name), dir, offerCommand(self, as, p), "")
	if err != nil {
		return "", err
	}
	_, err = srv.Do(tmux.SetPane(id, holdMark, "1"))
	return id, err
}

// runOffer is conn offer ROLE ROOT NAME: the question for a role,
// standing in a pane of conn's server.
func runOffer(args []string) error {
	if len(args) != 3 || args[0] != string(roleStack) && args[0] != string(roleCheck) {
		return usageError{"conn offer takes stack or check, a project's root and its name"}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	srv, err := findServer(home)
	if err != nil {
		return err
	}
	wear(ground(home, srv.Socket))
	dir, _ := os.Getwd()
	p := project{Root: args[1], Name: args[2], Group: isFolder(args[1], home)}
	o := newOffer(srv, role(args[0]), p, dir, os.Getenv("TMUX_PANE"))
	_, err = tea.NewProgram(o, tea.WithColorProfile(colorprofile.TrueColor)).Run()
	return err
}

// An offerModel is the question as it stands: what is listed, whether
// it is written down or Claude Code's answer, the cursor on it, and
// the line x last took off; what is being asked; the starts made on
// enter, while they are watched; and what is said once it is over.
type offerModel struct {
	srv *tmux.Server
	as  role
	p   project
	// Where the cursor stood on the panel: a checkout of the project,
	// where each line of its repository runs, or the root.
	dir string
	// Its own pane, which carries the first pane started.
	own           string
	width, height int

	list    []stacked
	written bool
	at      int
	pruned  *pruned
	// Asking Claude Code what the role means, or, with again set,
	// about the runs that failed at once.
	asking bool
	again  []failure
	err    string
	// The starts enter made, and what each is doing by the last read;
	// and the first pane started, blank for none.
	runs  []watched
	state map[string]string
	first string
	// The last word, before the keys go back: Claude's that a failure
	// is the project's own.
	over string
	note string
}

// newOffer is the question for a role in a project: what is written
// down, listed, or nothing, to ask about.
func newOffer(srv *tmux.Server, as role, p project, dir, own string) offerModel {
	o := offerModel{srv: srv, as: as, p: p, dir: dir, own: own, width: 80, height: 24}
	if list := established(readStarted(startedPath(srv.Socket)), p.belongs, as); len(list) > 0 {
		o.list, o.written = list, true
	} else {
		o.asking = true
	}
	return o
}

func (o offerModel) Init() tea.Cmd {
	if o.asking {
		return askRole(o.p, o.as)
	}
	return nil
}

// offerTick is the watch's read, due.
type offerTick struct{}

func (o offerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		o.width, o.height = msg.Width, msg.Height
	case roleAnswer:
		if !o.asking || msg.as != o.as {
			return o, nil
		}
		o.asking = false
		switch {
		case msg.kept:
			o.over = names(o.again) + " failed at once; Claude says the command is right and the failure is the project's"
			o.again = nil
		case msg.err != "":
			o.err = msg.err
		default:
			o.list, o.written, o.at, o.pruned = msg.list, false, 0, nil
		}
	case startedRoles:
		o.note = msg.said
		if msg.first != "" {
			o.first = msg.first
		}
		return o, o.watch()
	case offerTick:
		if len(o.runs) == 0 {
			return o, nil
		}
		ps, _ := panes(o.srv)
		return o.watched(ps, time.Now())
	case tea.KeyPressMsg:
		o.note = ""
		return o.key(msg)
	}
	return o, nil
}

// key is a key on the question: enter starts what is listed, or
// closes the last word; esc leaves, and leaves what was started
// running; on a list of what is written down, j and k move, x takes a
// line off for good and u puts it back.
func (o offerModel) key(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc", "q", "ctrl+c":
		return o.leave()
	case "enter":
		if o.over != "" || o.err != "" {
			return o.leave()
		}
		if o.asking || len(o.runs) > 0 || len(o.list) == 0 {
			return o, nil
		}
		return o.start()
	case "j", "down":
		if o.written && len(o.list) > 0 {
			o.at = min(o.at+1, len(o.list)-1)
		}
	case "k", "up":
		o.at = max(o.at-1, 0)
	case "x":
		if o.written && o.at < len(o.list) {
			s := o.list[o.at]
			if err := recordStart(startedPath(o.srv.Socket), started{Root: or(s.root, o.p.Root), Name: s.name, Command: s.command, Role: roleOff}); err != nil {
				o.note = err.Error()
				return o, nil
			}
			o.pruned = &pruned{s, o.at}
			o.list = append(append([]stacked{}, o.list[:o.at]...), o.list[o.at+1:]...)
			o.at = min(o.at, max(len(o.list)-1, 0))
			o.note = s.name + " is off for good: " + o.keys() + " won't run it and new agents aren't told it; u puts it back"
		}
	case "u":
		// Undo: the line x last took off is written down again as what
		// it was, and stands where it stood.
		if p := o.pruned; o.written && p != nil {
			if err := recordStart(startedPath(o.srv.Socket), started{Root: or(p.s.root, o.p.Root), Name: p.s.name, Command: p.s.command, Role: o.as}); err != nil {
				o.note = err.Error()
				return o, nil
			}
			at := min(p.at, len(o.list))
			o.list = append(append(append([]stacked{}, o.list[:at]...), p.s), o.list[at:]...)
			o.at, o.pruned = at, nil
			o.note = p.s.name + " is back"
		}
	}
	return o, nil
}

// A pruned line is what x took off the list of what is written down,
// and where it stood, for u to put back.
type pruned struct {
	s  stacked
	at int
}

// keys is the chord that opens this question from the panel.
func (o offerModel) keys() string {
	if o.as == roleStack {
		return "ss"
	}
	return "st"
}

// leave puts the keys back on the panel and ends.
func (o offerModel) leave() (tea.Model, tea.Cmd) {
	srv := o.srv
	return o, tea.Sequence(func() tea.Msg {
		if p, err := findPanel(srv, ""); err == nil && p.pane != "" {
			_, _ = srv.Do(tmux.Cmd{"select-window", "-t", p.window}, tmux.SelectPane(p.pane))
		}
		return nil
	}, tea.Quit)
}

// start starts each line listed, off the loop, and watches the starts.
// Each runs in its repository's checkout where the cursor stood in
// one, else at its repository, and is written down under the
// repository as what the role means there; the first pane started is
// put on the offer's own pane for the panel.
func (o offerModel) start() (tea.Model, tea.Cmd) {
	list, as, p, dir, srv, own := o.list, o.as, o.p, o.dir, o.srv, o.own
	now := time.Now()
	o.runs, o.state = nil, map[string]string{}
	for _, s := range list {
		o.runs = append(o.runs, watched{project: p.Root, stacked: s, as: as, at: now})
		o.state[s.name] = "starting"
	}
	o.again = nil
	return o, func() tea.Msg {
		ps, _ := panes(srv)
		var said []string
		first := ""
		for _, s := range list {
			repo := or(s.root, p.Root)
			at := repo
			if rootOf(dir) == repo {
				at = dir
			}
			r, id, err := startIn(srv, ps, p, s.name, s.command, at, as)
			if err != nil {
				return startedRoles{said: s.name + ": " + err.Error(), first: first}
			}
			_ = recordStart(startedPath(srv.Socket), started{Root: repo, Name: s.name, Command: s.command, Role: as})
			if first == "" && id != "" {
				first = id
				if own != "" {
					_, _ = srv.Do(tmux.SetPane(own, firstMark, id))
				}
			}
			said = append(said, r)
		}
		return startedRoles{said: strings.Join(said, "; "), first: first}
	}
}

// startedRoles is the starts of a role made: what was said of each,
// and the first pane started, blank where none was.
type startedRoles struct {
	said  string
	first string
}

// startIn starts a process under a name for a role in a project, in a
// window of its own, unless one by the name runs in the project
// already, which is left and said. A pane by the name whose run has
// ended is the row's last run: the new run takes its place, and the
// old pane goes once the new one is up.
func startIn(srv *tmux.Server, ps []pane, p project, name, command, dir string, as role) (said, id string, err error) {
	replace := ""
	for _, r := range ps {
		if r.Name != name || !p.holds(r) {
			continue
		}
		if r.Exit == "" && !r.Dead {
			return name + " runs already", "", nil
		}
		replace = r.ID
	}
	id, err = startNamed(srv, name, command, dir, dir, as)
	if err != nil {
		return "", "", err
	}
	if replace != "" {
		_, _ = srv.Do(tmux.Kill(replace))
	}
	return name + " is up", id, nil
}

// watch reads the starts again after a moment.
func (o offerModel) watch() tea.Cmd {
	return tea.Tick(runPoll, func(time.Time) tea.Msg { return offerTick{} })
}

// watched reads what the starts have done by a read of the panes: each
// that failed at once is a failure, each that ended well or is still
// going past quickWait is let go of, the rest are still watched. Once
// nothing is watched, the failures are asked about, and with none the
// question is over.
func (o offerModel) watched(ps []pane, at time.Time) (tea.Model, tea.Cmd) {
	var still []watched
	var failed []failure
	for _, w := range o.runs {
		p, ok := runOf(ps, o.p, w)
		switch {
		case ok && p.Exit != "" && p.Exit != "0" && p.Ended.Sub(w.at) <= quickWait:
			failed = append(failed, failure{stacked: w.stacked, pane: p.ID, exit: p.Exit, as: w.as})
			o.state[w.name] = "failed at once, exited " + p.Exit
		case ok && p.Exit != "":
			o.state[w.name] = "done"
		case at.Sub(w.at) > quickWait:
			if ok {
				o.state[w.name] = "up"
			}
		default:
			if ok {
				o.state[w.name] = "up"
			}
			still = append(still, w)
		}
	}
	o.runs = still
	o.again = append(o.again, failed...)
	if len(still) > 0 {
		return o, o.watch()
	}
	if len(o.again) == 0 {
		return o.leave()
	}
	o.asking = true
	return o, askAgain(o.srv, o.p, o.as, o.again)
}

// holds says whether a pane of a process stands in the project: by
// the directory it was started for, or the repository that is in.
func (p project) holds(r pane) bool {
	return !r.Hold && !r.Panel && (p.belongs(r.Dir) || p.belongs(rootOf(r.Dir)))
}

// runOf is the pane a watched run stands in: the one under its name in
// the project that ended no sooner than it started, where it has
// ended, or the one still running. A pane by the name that ended
// before is the run it replaced, which goes once the new one is up.
func runOf(ps []pane, p project, w watched) (pane, bool) {
	for _, r := range ps {
		if r.Name != w.name || !p.holds(r) {
			continue
		}
		if r.Exit == "" || !r.Ended.Before(w.at.Truncate(time.Second)) {
			return r, true
		}
	}
	return pane{}, false
}

// View is the question in the pane: the project's head, what stands
// under it, and the keys on the last row.
func (o offerModel) View() tea.View {
	lines, keys := o.draw()
	feet := []string{""}
	if o.note != "" {
		feet = model{width: o.width + 2}.feet(foot{say: o.note})
	}
	feet = append(feet, at(keyLine(keys)))
	h := max(o.height, len(lines)+len(feet))
	for len(lines) < h-len(feet) {
		lines = append(lines, "")
	}
	lines = append(lines[:h-len(feet)], feet...)
	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	return v
}

// keyLine is keys on a row: each in the ink and bold, what it does in
// the gray after it, three cells between one and the next.
func keyLine(keys []keyHint) string {
	var parts []string
	for _, k := range keys {
		parts = append(parts, keyInk.Render(k.key)+gray.Render(" "+k.does))
	}
	return strings.Join(parts, "   ")
}

// draw is the question's lines and its keys.
func (o offerModel) draw() ([]string, []keyHint) {
	what, verb, ask, again := "runs", "start them", "Start these in "+o.p.Name+"?", "what to start"
	if o.as == roleCheck {
		what, verb, ask, again = "checks", "run them", "Run these checks in "+o.p.Name+"?", "what they are"
	}
	with := "the stack"
	if o.as == roleCheck {
		with = "the checks"
	}
	lines := head(o.p.Name, with)
	switch {
	case o.over != "":
		lines = append(lines, at(ink.Render(o.over)))
		return lines, []keyHint{{"esc", "close"}}
	case o.err != "":
		lines = append(lines, at(ink.Render("Claude would not say what "+what+" "+o.p.Name+":")), at(gray.Render(o.err)))
		return lines, []keyHint{{"esc", "close"}}
	case o.asking && len(o.again) > 0:
		lines = append(lines, at(gray.Render(names(o.again)+" failed at once; asking Claude again…")))
		return lines, []keyHint{{"esc", "leave it"}}
	case o.asking:
		lines = append(lines, at(gray.Render("Nobody has done it here yet; asking Claude what "+what+" "+o.p.Name+"…")))
		return lines, []keyHint{{"esc", "cancel"}}
	case len(o.runs) > 0 || o.state != nil && len(o.again) == 0:
		// The starts, each with what it is doing, while they are
		// watched.
		lines = append(lines, at(title.Render("Starting in "+o.p.Name)), "")
		return append(lines, o.rows(false, func(s stacked) string { return o.state[s.name] })...), []keyHint{{"esc", "leave them"}}
	case o.written && len(o.list) == 0:
		lines = append(lines, at(ink.Render("No "+what+" written down in "+o.p.Name+". "+o.keys()+" asks Claude "+again+".")))
		return lines, []keyHint{{"esc", "close"}}
	}
	if len(o.again) > 0 {
		ask = "Start these instead?"
		if o.as == roleCheck {
			ask = "Run these instead?"
		}
	}
	m := model{width: o.width + 2}
	count := ""
	if o.written {
		count = strconv.Itoa(len(o.list))
	}
	lines = append(lines, m.row([]piece{{ask, title}}, piece{count, gray}, false), "")
	lines = append(lines, o.rows(o.written, func(stacked) string { return "" })...)
	keys := []keyHint{{"enter", verb}, {"esc", "run nothing"}}
	if o.written {
		keys = append([]keyHint{{"j k", "move"}, {"x", "take off for good"}}, keys...)
		if o.pruned != nil {
			keys = append(keys, keyHint{"u", "put it back"})
		}
	}
	return lines, keys
}

// rows is the list, a name and its command a row, the cursor on one
// where the list has a cursor, and a word at the right for each.
func (o offerModel) rows(cursor bool, right func(stacked) string) []string {
	m := model{width: o.width + 2}
	w := 0
	for _, s := range o.list {
		w = max(w, len(s.name))
	}
	var out []string
	for i, s := range o.list {
		st := gray
		if r := right(s); strings.HasPrefix(r, "failed") {
			st = failing
		}
		out = append(out, m.row([]piece{{pad(s.name, w), keyInk}, {"  " + s.command, gray}}, piece{right(s), st}, cursor && i == o.at))
	}
	return out
}
