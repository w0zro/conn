package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// e is env, annotated. The navigator's e, and the chord from any shell,
// answer in a popup over the window with the environment of the process
// in the pane — the leaf of the run the pane holds, whose environment is
// exactly the block it was started with — each variable read for what its
// value is and where it came from. A path says whether it is there; a URL
// says whether anything is listening where it points; a secret says only
// that it is set. The variables the project set, and the ones that say
// which environment the process runs in, come first; the rest are grouped
// by who set them — npm, the shell's rc files, the terminal conn was
// started in — and folded. The project's .env is read beside it, so a
// variable the file names and the process lacks is a line of its own.
// The popup shows nothing in the navigator: it is a question, not a
// process, and goes on q.

// envSubject is whose environment the page is about, with the layers it
// is read against.
type envSubject struct {
	title string   // the run's name and the leaf's pid; or the server
	dir   string   // where the process works
	notes []string // under the title: as it was started, 2h ago; what could not be read
	pid   int      // the leaf; 0 for the server
	env   []string

	// chain is the environments of the process's ancestors, nearest
	// first, up to the pane's shell, with the name each answers to; the
	// nearest ancestor without a variable is the one below it that set it.
	chain      []map[string]string
	chainNames []string
	top        string            // the command of the outermost ancestor read: a shell, when the shell could be
	server     map[string]string // tmux's global environment: the launcher's terminal
	dotenv     map[string]string // the place's .env, by name; nil for none
	dotenvKeys []string          // its names, in the file's order
	listeners  map[string]listener
	tools      []string // the commands the place's ecosystem runs, to find on PATH
}

// listener is who is accepting connections on a port.
type listener struct {
	name string
	pid  int
}

// readEnvSubject reads the environment of the process in the run pid heads
// — the leaf, since that is what the pane is running — or, for pid 0, the
// server's global environment, which a shell opened anywhere starts from.
// live, when it is given, is the environment of the shell at pid as it is
// now, brought by conn env typed at its prompt: the page shows that, and
// says the shell is at its prompt.
func readEnvSubject(run runner, pid int, live []string) (envSubject, error) {
	s := envSubject{pid: pid, server: envMap(serverEnv(run))}
	procs, err := runningProcs()
	if err != nil && pid != 0 {
		return s, err
	}
	s.listeners = listenersOf(append(procs, containers()...))
	if pid == 0 {
		s.title = "the server"
		s.notes = []string{"what a shell opened anywhere starts from, before its rc files"}
		s.env = mapEnv(s.server)
		return s, nil
	}
	if err := s.describe(procs, pid, environOf, live); err != nil {
		return s, err
	}
	place := s.dir
	if p, err := placeHolding(s.dir); err == nil {
		place = p.Path
	}
	s.dotenv, s.dotenvKeys = readDotenv(filepath.Join(place, ".env"))
	s.tools = toolsFor(place)
	return s, nil
}

// describe finds the run pid heads in the process table and reads its
// leaf's environment through read, and its ancestors' up to pid, for the
// layers. An ancestor whose environment cannot be read is passed over,
// not taken as empty: macOS keeps the environment of its own binaries —
// the zsh a pane runs — to itself, and a layer unknown is not a layer
// that set everything. A leaf that is such a shell, at its prompt, has
// nothing to read either; the page then shows what the shell started
// from, the server's environment, and says so.
func (s *envSubject) describe(procs []Proc, pid int, read func(int) []string, live []string) error {
	byPID := map[int]*ProcNode{}
	for _, root := range procForest(procs) {
		indexNodes(root, byPID)
	}
	node, ok := byPID[pid]
	if !ok {
		return errors.New("no process " + strconv.Itoa(pid))
	}
	if live != nil {
		s.pid, s.dir = node.PID, node.Dir
		s.title = commandOf(node) + " " + nodeID(node)
		s.env = live
		s.notes = []string{"as it is now, at the prompt: what a command typed there gets"}
		return nil
	}
	steps := runFrom(node, 1024)
	leaf := steps[len(steps)-1]
	s.pid, s.dir = leaf.PID, leaf.Dir
	s.title = commandOf(nameOf(steps)) + " " + nodeID(leaf)
	s.env = read(leaf.PID)
	if len(s.env) == 0 {
		if !isShell(leaf.Command) || runtime.GOOS == "linux" {
			return errors.New("the environment of " + s.title + " cannot be read")
		}
		s.env = mapEnv(s.server)
		s.notes = []string{
			"macOS keeps a system binary's environment to itself, this " + leaf.Command + "'s among them;",
			"this is the server's, which it started from · conn env, typed at its prompt, shows its own",
		}
		return nil
	}
	s.notes = []string{joinWords("as it was started", startedAgo(leaf.PID))}
	for p := byPID[leaf.PPID]; p != nil; p = byPID[p.PPID] {
		if env := read(p.PID); len(env) > 0 {
			s.chain = append(s.chain, envMap(env))
			s.chainNames = append(s.chainNames, commandOf(p))
			s.top = p.Command
		}
		if p.PID == pid {
			break
		}
	}
	return nil
}

// shell is the shell whose startup files set the subject's environment:
// the one its SHELL names, which is what the pane runs, else the
// launcher's.
func (s envSubject) shell() string {
	if sh := envMap(s.env)["SHELL"]; sh != "" {
		return sh
	}
	return os.Getenv("SHELL")
}

// serverEnv is the server's global environment, as show-environment lists
// it: a variable a line, and a line beginning with - for one unset, which
// is not a variable.
func serverEnv(run runner) []string {
	out, err := run("show-environment", "-g")
	if err != nil {
		return nil
	}
	var env []string
	for line := range strings.SplitSeq(out, "\n") {
		if line != "" && !strings.HasPrefix(line, "-") && strings.Contains(line, "=") {
			env = append(env, line)
		}
	}
	return env
}

// envMap is an environment by name.
func envMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, kv := range env {
		if name, value, ok := strings.Cut(kv, "="); ok {
			m[name] = value
		}
	}
	return m
}

// mapEnv is a map back as name=value lines, sorted.
func mapEnv(m map[string]string) []string {
	env := make([]string, 0, len(m))
	for name, value := range m {
		env = append(env, name+"="+value)
	}
	slices.Sort(env)
	return env
}

// startedAgo is how long ago a process started, in the pane's terms, or
// nothing when ps cannot say.
func startedAgo(pid int) string {
	out, err := ps(pid, "lstart=")
	if err != nil {
		return ""
	}
	t, err := time.ParseInLocation("Mon Jan _2 15:04:05 2006", strings.TrimSpace(out), time.Local)
	if err != nil {
		return ""
	}
	return ago(t)
}

// listenersOf is who is listening on each port: a process by the name its
// row shows, a container by its service.
func listenersOf(procs []Proc) map[string]listener {
	ls := map[string]listener{}
	for _, p := range procs {
		for _, port := range p.Ports {
			if _, taken := ls[port]; taken {
				continue
			}
			name := commandOf(&ProcNode{Proc: p})
			if p.Container != nil {
				name = "docker " + p.Container.Service
			}
			ls[port] = listener{name: name, pid: p.PID}
		}
	}
	return ls
}

// readDotenv reads a .env file: a name and a value a line, an export
// allowed in front, quotes around a value taken off, blank lines and
// comments skipped. The names come back in the file's order too, for the
// ones the process lacks to be listed as the file lists them.
func readDotenv(path string) (map[string]string, []string) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil
	}
	vars := map[string]string{}
	var keys []string
	for line := range strings.SplitSeq(string(b), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export "))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		name = strings.TrimSpace(name)
		if !ok || !envName.MatchString(name+"=") {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		} else if i := strings.Index(value, " #"); i >= 0 {
			value = strings.TrimSpace(value[:i])
		}
		if _, seen := vars[name]; !seen {
			keys = append(keys, name)
		}
		vars[name] = value
	}
	return vars, keys
}

// toolsFor is the commands a place's ecosystem runs, by the files that
// mark it — the ones worth finding on PATH, since which node and which
// python is the usual question.
func toolsFor(dir string) []string {
	marks := []struct {
		file  string
		tools []string
	}{
		{"go.mod", []string{"go"}},
		{"package.json", []string{"node", "npm"}},
		{"Cargo.toml", []string{"cargo"}},
		{"pyproject.toml", []string{"python3"}},
		{"manage.py", []string{"python3"}},
		{"requirements.txt", []string{"python3"}},
		{"Gemfile", []string{"ruby", "bundle"}},
		{"mix.exs", []string{"mix"}},
		{"deno.json", []string{"deno"}},
		{"Package.swift", []string{"swift"}},
	}
	var tools []string
	for _, m := range marks {
		if exists(filepath.Join(dir, m.file)) {
			for _, t := range m.tools {
				if !slices.Contains(tools, t) {
					tools = append(tools, t)
				}
			}
		}
	}
	return tools
}

// The page's rows.

type envRowKind int

const (
	envGroupRow envRowKind = iota // a group's title, with its count
	envVarRow                     // a variable
	envEntryRow                   // one entry of a variable that is a list, under it
)

// envRow is one line of the page: a group's title, a variable, or one
// entry of a list-valued variable. under names the variable an entry is
// listed beneath; always marks an entry shown while its variable is folded.
type envRow struct {
	kind      envRowKind
	group     int
	name      string
	value     string
	valueTone tone
	note      string
	noteTone  tone
	source    string
	under     string
	always    bool
	folds     bool // a variable with entries under it
}

// envGroup is a heading on the page: its title, and whether it starts
// folded.
type envGroup struct {
	title  string
	folded bool
}

// The groups, in the page's order. The first two are by kind — what you
// came to look for; the rest are by who set the variable, nearest first.
const (
	groupProject = "the project's"
	groupRuntime = "the runtime"
	groupConn    = "conn's"
	groupShell   = "the shell's"
	groupTerm    = "the terminal conn started in"
)

// envRows is the page: the subject's variables annotated and grouped.
func envRows(s envSubject) ([]envGroup, []envRow) {
	byGroup := map[string][]envRow{}
	var setters []string
	env := envMap(s.env)
	for _, kv := range s.env {
		name, value, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		source := sourceOf(name, value, s)
		rows := describeVar(name, value, source, s)
		group := groupOf(name, source, s)
		if !fixedGroup(group) && !slices.Contains(setters, group) {
			setters = append(setters, group)
		}
		byGroup[group] = append(byGroup[group], rows...)
	}
	// What the file names and the process was not given.
	for _, name := range s.dotenvKeys {
		if _, has := env[name]; has {
			continue
		}
		says := ".env sets it"
		if !secretName(name) {
			says = ".env says " + truncateTail(maskURL(s.dotenv[name]), 24)
		}
		byGroup[groupProject] = append(byGroup[groupProject], envRow{
			kind: envVarRow, name: name, value: "not set", valueTone: toneAttn, note: says, noteTone: toneAttn})
	}

	order := []string{groupProject, groupRuntime}
	order = append(order, setters...)
	order = append(order, groupConn, groupShell, groupTerm)
	var groups []envGroup
	var rows []envRow
	for _, title := range order {
		vars := byGroup[title]
		if len(vars) == 0 {
			continue
		}
		g := len(groups)
		count := 0
		for _, r := range vars {
			if r.kind == envVarRow {
				count++
			}
		}
		folded := title != groupProject && title != groupRuntime && title != groupConn
		groups = append(groups, envGroup{title: title, folded: folded})
		rows = append(rows, envRow{kind: envGroupRow, group: g, name: title, value: plural(count, "variable", "variables")})
		for _, r := range vars {
			r.group = g
			rows = append(rows, r)
		}
	}
	return groups, rows
}

// fixedGroup reports a group the page always has a place for, as against
// one named for whichever process set its variables.
func fixedGroup(title string) bool {
	switch title {
	case groupProject, groupRuntime, groupConn, groupShell, groupTerm:
		return true
	}
	return false
}

// sourceOf is where a variable came from: the ancestor below the nearest
// one that did not have it with this value — npm, say — else, when every
// ancestor had it, the terminal conn was started in when the server's
// environment agrees, and the shell's own rc files when it does not. A
// variable conn or tmux set says so by its name. A process that had it
// alone set it for itself. When the shell could not be read and the
// outermost ancestor known is an npm or the like, a variable of its own
// family — npm_config_prefix — is its, and the rest are left to the shell.
func sourceOf(name, value string, s envSubject) string {
	if s.dotenv != nil && s.dotenv[name] == value {
		return ".env"
	}
	if connName(name) {
		return "conn"
	}
	for i, anc := range s.chain {
		if anc[name] == value {
			continue
		}
		if i == 0 {
			return "its own"
		}
		return s.chainNames[i-1]
	}
	if s.server[name] == value || s.pid == 0 {
		return "the terminal"
	}
	if len(s.chain) > 0 && !isShell(s.top) && inFamily(name, s.top) {
		return s.chainNames[len(s.chainNames)-1]
	}
	return "the shell"
}

// inFamily reports a variable named the way a tool names the ones it
// sets for what it runs: npm_ under npm, and each of the rest under its
// own prefix.
func inFamily(name, command string) bool {
	base := strings.ToLower(filepath.Base(command))
	prefixes := []string{strings.ToUpper(base) + "_", base + "_"}
	switch base {
	case "npm", "npx", "yarn", "pnpm", "bun":
		prefixes = append(prefixes, "npm_", "NPM_", "NODE_")
	case "node":
		prefixes = append(prefixes, "NODE_")
	case "bundle", "bundler", "ruby":
		prefixes = append(prefixes, "BUNDLE", "RUBY", "GEM_")
	case "cargo":
		prefixes = append(prefixes, "CARGO_", "RUST")
	case "go":
		prefixes = append(prefixes, "GO")
	case "make", "gmake":
		prefixes = append(prefixes, "MAKE", "MFLAGS")
	case "python", "python3", "uv", "poetry", "pip":
		prefixes = append(prefixes, "PYTHON", "VIRTUAL_ENV", "PIP_", "UV_", "POETRY_")
	}
	for _, p := range prefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// connName reports a variable conn or tmux gives every pane.
func connName(name string) bool {
	switch name {
	case "TMUX", "TMUX_PANE", "TERM", "TERM_PROGRAM", "TERM_PROGRAM_VERSION", "CLAUDE_CODE_TMUX_TRUECOLOR":
		return true
	}
	return false
}

// groupOf is the group a variable is listed under.
func groupOf(name, source string, s envSubject) string {
	switch {
	case source == ".env" || tellingName(name) || (s.dotenv != nil && s.dotenv[name] != ""):
		return groupProject
	case runtimeName(name):
		return groupRuntime
	case source == "conn":
		return groupConn
	case source == "the terminal":
		return groupTerm
	case source == "the shell" || source == "its own":
		return groupShell
	}
	return source + "'s"
}

// runtimeName reports a variable that says which tools run and how: the
// paths, and the families each ecosystem's tools read.
func runtimeName(name string) bool {
	switch name {
	case "PATH", "MANPATH", "INFOPATH", "LD_LIBRARY_PATH", "DYLD_LIBRARY_PATH", "PKG_CONFIG_PATH",
		"CPATH", "LIBRARY_PATH", "JAVA_HOME", "VIRTUAL_ENV", "CONDA_PREFIX", "CONDA_DEFAULT_ENV":
		return true
	}
	for _, prefix := range []string{"GO", "NODE_", "NVM_", "NPM_", "PNPM_", "YARN_", "PYTHON", "PIP_", "UV_",
		"CARGO_", "RUST", "RUBY", "GEM_", "BUNDLE_", "DENO_", "MIX_", "ERL_", "JAVA_", "GRADLE_", "MAVEN_",
		"HOMEBREW_", "SDKMAN_", "DOCKER_", "KUBECONFIG"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// describeVar is a variable's rows: the one for the variable, its value
// read for what it is, and under a list of paths one per entry. A secret
// says only that it is set, and how long it is.
func describeVar(name, value, source string, s envSubject) []envRow {
	row := envRow{kind: envVarRow, name: name, value: value, source: source}
	var entries []envRow
	switch {
	case secretName(name):
		row.value, row.valueTone = "set, "+plural(len(value), "char", "chars"), toneQuiet
	case value == "":
		row.value, row.valueTone = "empty", toneAttn
	case isPathList(name, value):
		row.value, row.valueTone, entries = describePathList(name, value, s)
		row.folds = len(entries) > 0
	case isURL(value):
		row.value = maskURL(value)
		row.note, row.noteTone = urlNote(value, s)
	case isPort(name, value):
		row.note, row.noteTone = listenNote(value, s)
	case looksLikePath(value) && name != "TMUX":
		row.value = homely(value)
		if !exists(expandPath(value)) {
			row.note, row.noteTone = "missing", toneBad
		}
	case envishName(name):
		row.valueTone = toneName
		if strings.EqualFold(value, "production") || strings.EqualFold(value, "prod") {
			row.valueTone = toneAttn
		}
	}
	if name == "VIRTUAL_ENV" {
		if v := venvVersion(value); v != "" {
			row.note = "python " + v
		}
	}
	if row.note == "" {
		if s.dotenv != nil && s.dotenv[name] != "" && s.dotenv[name] != value {
			row.note, row.noteTone = ".env says otherwise", toneAttn
			if !secretName(name) {
				row.note = ".env says " + truncateTail(maskURL(s.dotenv[name]), 24)
			}
		} else if m, ok := meanings[name]; ok {
			row.note, row.noteTone = m, toneQuiet
		}
	}
	return append([]envRow{row}, entries...)
}

// envishName reports a variable that names the environment a process runs
// in: NODE_ENV, RAILS_ENV, ENV.
func envishName(name string) bool {
	return name == "ENV" || name == "ENVIRONMENT" || strings.HasSuffix(name, "_ENV")
}

// isPathList reports a value that is directories separated by colons, or
// a name that is known to hold one.
func isPathList(name, value string) bool {
	if !strings.Contains(value, ":") || strings.Contains(value, "://") {
		return false
	}
	if name == "PATH" || strings.HasSuffix(name, "PATH") {
		return true
	}
	if !strings.Contains(value, ":") || strings.Contains(value, "://") {
		return false
	}
	for _, part := range strings.Split(value, ":") {
		if part != "" && !strings.HasPrefix(part, "/") && !strings.HasPrefix(part, "~") && !strings.HasPrefix(part, ".") {
			return false
		}
	}
	return true
}

// describePathList is a list's summary — its count, how many entries are
// missing, how many are there twice — and its entries as rows under it,
// each said to be missing or a repeat. For PATH, the place's tools are
// found along it, each a row that shows while the list is folded.
func describePathList(name, value string, s envSubject) (string, tone, []envRow) {
	parts := strings.Split(value, ":")
	seen := map[string]bool{}
	missing, twice := 0, 0
	var rows []envRow
	for _, p := range parts {
		r := envRow{kind: envEntryRow, under: name, value: homely(p), valueTone: toneQuiet}
		switch {
		case p == "":
			r.value, r.note, r.noteTone = "(empty: the working directory)", "", toneAttn
		case seen[p]:
			twice++
			r.note, r.noteTone = "again", toneQuiet
		case !exists(expandPath(p)):
			missing++
			r.note, r.noteTone = "missing", toneBad
		}
		seen[p] = true
		rows = append(rows, r)
	}
	if name == "PATH" {
		var found []envRow
		for _, tool := range s.tools {
			r := envRow{kind: envEntryRow, under: name, name: tool, always: true, note: "not on the path", noteTone: toneAttn}
			for i, p := range parts {
				if fi, err := os.Stat(filepath.Join(expandPath(p), tool)); err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
					r.value, r.valueTone = homely(p), tonePlain
					r.note, r.noteTone = "entry "+strconv.Itoa(i+1), toneQuiet
					break
				}
			}
			found = append(found, r)
		}
		rows = append(found, rows...)
	}
	summary := plural(len(parts), "entry", "entries")
	t := tonePlain
	if missing > 0 {
		summary += " · " + strconv.Itoa(missing) + " missing"
		t = toneAttn
	}
	if twice > 0 {
		summary += " · " + strconv.Itoa(twice) + " twice"
	}
	return summary, t, rows
}

// urlScheme is the start of a URL.
var urlScheme = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*://`)

func isURL(value string) bool { return urlScheme.MatchString(value) }

// isPort reports a variable holding a port number.
func isPort(name, value string) bool {
	if name != "PORT" && !strings.HasSuffix(name, "_PORT") {
		return false
	}
	n, err := strconv.Atoi(value)
	return err == nil && n > 0 && n < 65536
}

// looksLikePath reports a value that is one path: absolute, or from ~,
// with nothing else on it.
func looksLikePath(value string) bool {
	return (strings.HasPrefix(value, "/") || strings.HasPrefix(value, "~/")) && !strings.ContainsAny(value, " :")
}

// defaultPorts is the port a URL's scheme means when it names none.
var defaultPorts = map[string]string{
	"http": "80", "https": "443", "ws": "80", "wss": "443",
	"postgres": "5432", "postgresql": "5432", "mysql": "3306", "mariadb": "3306",
	"redis": "6379", "rediss": "6379", "mongodb": "27017", "amqp": "5672", "nats": "4222",
	"memcached": "11211", "smtp": "25", "ldap": "389",
}

// urlNote says whether anything is listening where a URL points, for a
// URL that points at this machine; a URL elsewhere is not probed.
func urlNote(value string, s envSubject) (string, tone) {
	u, err := url.Parse(value)
	if err != nil || !localHost(u.Hostname()) {
		return "", tonePlain
	}
	port := u.Port()
	if port == "" {
		port = defaultPorts[strings.ToLower(u.Scheme)]
	}
	if port == "" {
		return "", tonePlain
	}
	return listenNote(port, s)
}

// localHost reports a host that is this machine.
func localHost(host string) bool {
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1", "0.0.0.0", "[::1]", "host.docker.internal":
		return true
	}
	return false
}

// listenNote says who is listening on a port: this process, another by
// its name, or nobody.
func listenNote(port string, s envSubject) (string, tone) {
	l, ok := s.listeners[port]
	switch {
	case !ok:
		return "nothing is listening", toneAttn
	case l.pid == s.pid && s.pid != 0:
		return "this process listens", toneGood
	}
	return l.name + " listens", toneGood
}

// venvVersion is the python a virtual environment was made with, from its
// pyvenv.cfg.
func venvVersion(dir string) string {
	b, err := os.ReadFile(filepath.Join(expandPath(dir), "pyvenv.cfg"))
	if err != nil {
		return ""
	}
	for line := range strings.SplitSeq(string(b), "\n") {
		if name, value, ok := strings.Cut(line, "="); ok && strings.TrimSpace(name) == "version" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// meanings is a word on the variables that always mean the same thing,
// where the page has nothing more pressing to say of them.
var meanings = map[string]string{
	"SHELL":                      "the shell s opens",
	"HOME":                       "the home directory",
	"USER":                       "who you are",
	"LOGNAME":                    "who you logged in as",
	"PWD":                        "where the shell was when this started",
	"OLDPWD":                     "where it was before that",
	"SHLVL":                      "how many shells deep",
	"TMPDIR":                     "where temporary files go",
	"LANG":                       "the locale",
	"LC_ALL":                     "the locale, over the LC_ variables",
	"LC_CTYPE":                   "the character set",
	"EDITOR":                     "what git and the rest open to edit",
	"VISUAL":                     "the editor, over EDITOR, on a terminal",
	"PAGER":                      "what long output is read through",
	"TERM":                       "the terminal's kind, per tmux",
	"COLORTERM":                  "what colors the terminal claims",
	"TERM_PROGRAM":               "the terminal around tmux",
	"TERM_PROGRAM_VERSION":       "its version",
	"TMUX":                       "conn's server: socket, pid, session",
	"TMUX_PANE":                  "the pane",
	"CLAUDE_CODE_TMUX_TRUECOLOR": "conn's: Claude Code in true color",
	"SSH_AUTH_SOCK":              "the ssh agent's socket",
	"SSH_CONNECTION":             "the ssh session this came in on",
	"DISPLAY":                    "the X display",
	"DEBUG":                      "what prints its debugging",
	"LOG_LEVEL":                  "how much is logged",
	"RUST_LOG":                   "how much rust logs",
	"NODE_OPTIONS":               "node's flags",
	"GOFLAGS":                    "go's flags",
	"GOPATH":                     "go's modules and installs",
	"GOROOT":                     "the go toolchain",
	"VIRTUAL_ENV":                "the python virtual environment",
	"CONDA_DEFAULT_ENV":          "the conda environment",
	"NVM_DIR":                    "where nvm keeps its nodes",
	"JAVA_HOME":                  "the jdk",
	"CONN_SOCKET":                "conn reads this: the shells' socket",
	"XDG_CONFIG_HOME":            "conn reads this: its configuration",
	"XDG_STATE_HOME":             "conn reads this: its state directory",
	"NO_COLOR":                   "conn reads this: no color",
	"CLICOLOR_FORCE":             "conn reads this: color regardless",
	"CLAUDE_CONFIG_DIR":          "conn reads this: Claude Code's sessions",
	"OLLAMA_MODELS":              "conn reads this: ollama's models",
}

// The page as a program.

// envModel is the page: the rows under a heading, a cursor, the folds,
// and a filter typed with /.
type envModel struct {
	pid     int
	live    []string // the shell's environment as it is now, for conn env typed there
	subj    envSubject
	groups  []envGroup
	rows    []envRow
	err     error
	loaded  bool
	cursor  int
	top     int
	width   int
	height  int
	filter  string
	typing  bool
	folded  map[int]bool    // by group
	foldedV map[string]bool // by list variable
}

// envReadMsg is the subject, read.
type envReadMsg struct {
	subj envSubject
	err  error
}

func newEnvModel(pid int, live []string) envModel {
	return envModel{pid: pid, live: live, width: 80, height: 24, folded: map[int]bool{}, foldedV: map[string]bool{}}
}

func (m envModel) Init() tea.Cmd {
	pid, live := m.pid, m.live
	return func() tea.Msg {
		s, err := readEnvSubject(tmuxCommand, pid, live)
		return envReadMsg{subj: s, err: err}
	}
}

func (m envModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case envReadMsg:
		m.loaded, m.err, m.subj = true, msg.err, msg.subj
		m.groups, m.rows = envRows(msg.subj)
		for i, g := range m.groups {
			if g.folded {
				m.folded[i] = true
			}
		}
		for _, r := range m.rows {
			if r.folds {
				m.foldedV[r.name] = true
			}
		}
		if msg.err == nil {
			return m, traceStartup(m.subj.shell())
		}
	case envTraceMsg:
		m.rows = withOrigins(m.rows, msg)
	case tea.KeyPressMsg:
		return m.key(msg)
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			m.move(-3)
		case tea.MouseWheelDown:
			m.move(3)
		}
	}
	return m, nil
}

// key is a keystroke: the filter's while one is being typed, else the
// navigator's vocabulary — j k move, space folds, - unfolds all, / finds,
// esc clears the filter, q leaves.
func (m envModel) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.typing {
		switch msg.String() {
		case "enter":
			m.typing = false
		case "esc":
			m.typing, m.filter = false, ""
		case "backspace":
			if m.filter != "" {
				m.filter = m.filter[:len(m.filter)-1]
			}
		case "ctrl+c":
			return m, tea.Quit
		default:
			if t := msg.Text; t != "" {
				m.filter += t
			}
		}
		m.cursor, m.top = 0, 0
		return m, nil
	}
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.filter == "" && !m.typing {
			return m, tea.Quit
		}
		m.filter = ""
		m.cursor, m.top = 0, 0
	case "/":
		m.typing = true
	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = max(0, len(m.visible())-1)
	case "ctrl+d", "pgdown":
		m.move(m.pageSize())
	case "ctrl+u", "pgup":
		m.move(-m.pageSize())
	case "space":
		m.toggle()
	case "-":
		m.folded, m.foldedV = map[int]bool{}, map[string]bool{}
	}
	return m, nil
}

func (m *envModel) move(by int) {
	m.cursor = min(max(m.cursor+by, 0), max(0, len(m.visible())-1))
}

// pageSize is how many rows fit under the heading and over the foot.
func (m envModel) pageSize() int { return max(1, m.height-m.headLines()-1) }

// headLines is the heading: the title, its notes, and a blank line.
func (m envModel) headLines() int { return 2 + len(m.subj.notes) }

// toggle folds or unfolds what the cursor is on: a list variable's
// entries, else the group.
func (m *envModel) toggle() {
	vis := m.visible()
	if m.cursor >= len(vis) {
		return
	}
	r := vis[m.cursor]
	switch {
	case r.folds:
		m.foldedV[r.name] = !m.foldedV[r.name]
	case r.kind == envEntryRow:
		m.foldedV[r.under] = !m.foldedV[r.under]
	default:
		m.folded[r.group] = !m.folded[r.group]
	}
}

// visible is the rows the page shows: under a filter, every variable and
// entry that matches, folds ignored; otherwise what the folds leave.
func (m envModel) visible() []envRow {
	var out []envRow
	needle := strings.ToLower(m.filter)
	for _, r := range m.rows {
		if needle != "" {
			if r.kind == envGroupRow {
				continue
			}
			hay := strings.ToLower(r.name + " " + r.under + " " + r.value + " " + r.note + " " + r.source)
			if strings.Contains(hay, needle) {
				out = append(out, r)
			}
			continue
		}
		if r.kind != envGroupRow && m.folded[r.group] {
			continue
		}
		if r.kind == envEntryRow && m.foldedV[r.under] && !r.always {
			continue
		}
		out = append(out, r)
	}
	return out
}

func (m envModel) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// render draws the page: the subject and its moment, the rows that fit
// around the cursor, and a foot with the keys — or the filter, while one
// is typed.
func (m envModel) render() string {
	if !m.loaded {
		return "\n " + hintStyle.Render("reading the environment…")
	}
	if m.err != nil {
		return "\n " + errStyle.Render(m.err.Error()) + "\n\n " + hintStyle.Render("q leaves")
	}
	vis := m.visible()
	size := m.pageSize()
	cursor := min(m.cursor, max(0, len(vis)-1))
	top := m.top
	if cursor < top {
		top = cursor
	}
	if cursor >= top+size {
		top = cursor - size + 1
	}
	top = max(0, min(top, max(0, len(vis)-size)))

	head := " " + headingStyle.Render(m.subj.title)
	if m.subj.dir != "" {
		head += "  " + noteStyle.Render(homely(m.subj.dir))
	}
	lines := []string{truncateTail(head, m.width)}
	for i, n := range m.subj.notes {
		if i == len(m.subj.notes)-1 {
			n += " · " + plural(len(m.subj.env), "variable", "variables")
		}
		lines = append(lines, " "+hintStyle.Render(truncateTail(n, m.width-1)))
	}
	lines = append(lines, "")

	widths := m.columns(vis[top:min(top+size, len(vis))])
	for i := top; i < len(vis) && i < top+size; i++ {
		lines = append(lines, m.line(vis[i], i == cursor, widths))
	}
	for len(lines) < m.height-1 {
		lines = append(lines, "")
	}
	foot := " " + hintStyle.Render("space fold · - unfold all · / find · q leave")
	if m.typing || m.filter != "" {
		foot = " " + itemStyle.Render("/"+m.filter)
		if m.typing {
			foot += itemStyle.Render("▏")
		}
	}
	if len(vis) == 0 && m.filter != "" && len(lines) > m.headLines() {
		lines[m.headLines()] = " " + hintStyle.Render("nothing matches "+m.filter)
	}
	lines = append(lines[:max(0, min(m.height-1, len(lines)))], truncateTail(foot, m.width))
	return strings.Join(lines, "\n")
}

// envColumns is the width of each column: the name, the value, the note,
// the source.
type envColumns struct{ name, value, note, source int }

// columns sizes the columns to the rows in view: the name and the source
// as wide as their widest — the source goes when every row of the page
// has the same, as the server's do; a filter down to one row keeps it,
// since the row's source may be what was asked — the note as wide as its
// widest up to a cap, and the value the rest, no narrower than a few
// words; when even that is short the note gives way, since the value is
// what the page is for.
func (m envModel) columns(rows []envRow) envColumns {
	var c envColumns
	for _, r := range rows {
		if r.kind == envGroupRow {
			continue
		}
		c.name = max(c.name, lipgloss.Width(r.name)+2*btoi(r.kind == envEntryRow))
		c.note = max(c.note, lipgloss.Width(r.note))
		c.source = max(c.source, lipgloss.Width(r.source))
	}
	sources := map[string]bool{}
	for _, r := range m.rows {
		if r.kind == envVarRow {
			sources[r.source] = true
		}
	}
	if len(sources) < 2 {
		c.source = 0
	}
	c.name = min(c.name, 28)
	c.note = min(c.note, 36)
	c.source = min(c.source, 30)
	rest := func() int { return m.width - 1 - c.name - 2 - c.note - 2 - c.source - 2*btoi(c.source > 0) }
	if rest() < 40 {
		c.note = max(12, c.note+rest()-40)
	}
	c.value = max(16, rest())
	return c
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// line draws one row: a group's title bold with its count and a mark
// while it is folded; a variable's name, value, note and source in
// their columns; an entry indented under its variable.
func (m envModel) line(r envRow, selected bool, c envColumns) string {
	mark := " "
	if selected {
		mark = selStyle.Render("▌")
	}
	if r.kind == envGroupRow {
		title := headingStyle.Render(r.name)
		if selected {
			title = selStyle.Render(r.name)
		}
		fold := ""
		if m.folded[r.group] {
			fold = " ▸"
		}
		return truncateTail(mark+title+"  "+hintStyle.Render(r.value)+hintStyle.Render(fold), m.width)
	}
	name := r.name
	nameStyle := itemStyle
	if r.kind == envEntryRow {
		name = "  " + name
		nameStyle = faintStyle
	}
	if selected {
		nameStyle = selStyle
	}
	width := c.value - 2*btoi(r.folds)
	value := truncateTail(r.value, width)
	if strings.HasPrefix(r.value, "/") || strings.HasPrefix(r.value, "~") {
		value = truncate(r.value, width)
	}
	if r.folds && m.foldedV[r.name] && m.filter == "" {
		value += " ▸"
	}
	out := mark + pad(nameStyle.Render(truncateTail(name, c.name)), c.name) + "  " +
		pad(toneStyles[r.valueTone].Render(value), c.value) + "  " +
		pad(toneStyles[r.noteTone].Render(truncateTail(r.note, c.note)), c.note)
	if c.source > 0 {
		out += "  " + faintStyle.Render(truncate(r.source, c.source))
	}
	return truncateTail(out, m.width)
}

// showEnvironment is the navigator's e: the environment of the run the
// row's process heads, or, on a place, the server's — what a shell opened
// there starts from. A container's is docker's to show.
func (m *model) showEnvironment() tea.Cmd {
	pid := 0
	if r, ok := m.selected(); ok && r.kind == rowProc {
		if r.node.Container != nil {
			m.status, m.statusErr = "a container's environment is docker's to show", true
			return nil
		}
		pid = r.chain().PID
	}
	m.server.environment(pid)
	return nil
}

// runEnvPage is `conn page env [pid]`, the page in the popup — and `conn
// page env live pid`, the page for the environment the popup itself was
// given, which is the shell's at pid as it is now.
func runEnvPage(args []string) {
	var live []string
	if len(args) > 0 && args[0] == "live" {
		live, args = os.Environ(), args[1:]
	}
	pid := 0
	if len(args) > 0 {
		pid, _ = strconv.Atoi(args[0])
	}
	if _, err := tea.NewProgram(newEnvModel(pid, live)).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "conn: %v\n", err)
		os.Exit(1)
	}
}

// envPopupWidth and envPopupHeight are the room the page asks for; a
// smaller client cuts it to the client.
const envPopupWidth, envPopupHeight = 110, 40

// showEnv shows the environment of the run pid heads — or the server's,
// for 0 — in a popup over the client; "" is the client that spoke last.
func showEnv(run runner, exe, client string, pid int) error {
	return popup(run, client, " environment ", envPopupWidth, envPopupHeight,
		shellQuote(exe)+" page env "+strconv.Itoa(pid))
}

// showLiveEnv shows an environment as it is now — the shell's at pid,
// which conn env typed there inherited — in the popup: handed to the
// popup's command variable by variable, so the page reads its own.
func showLiveEnv(run runner, exe, client string, pid int, env []string) error {
	return popup(run, client, " environment ", envPopupWidth, envPopupHeight,
		shellQuote(exe)+" page env live "+strconv.Itoa(pid), env...)
}

// runEnvChord is `conn env [pid client]`: the chord's, handed the pane's
// shell pid and the client that pressed it in one word. Typed at a shell
// inside conn, with neither, it is that shell's own environment as it is
// now — this process inherited it, exports and all — over the client
// that spoke last.
func runEnvChord(arg string) error {
	f := strings.Fields(arg)
	pid, client := 0, ""
	if len(f) > 0 {
		pid, _ = strconv.Atoi(f[0])
	}
	if len(f) > 1 {
		client = f[1]
	}
	if pid != 0 {
		return showEnv(tmuxCommand, connExe(), client, pid)
	}
	pane := os.Getenv("TMUX_PANE")
	if pane == "" {
		return errors.New("no pane to read the environment of")
	}
	out, err := tmuxCommand("display-message", "-p", "-t", pane, "#{pane_pid}")
	if err != nil {
		return err
	}
	if pid, _ = strconv.Atoi(strings.TrimSpace(out)); pid == 0 {
		return errors.New("no pane to read the environment of")
	}
	return showLiveEnv(tmuxCommand, connExe(), client, pid, os.Environ())
}
