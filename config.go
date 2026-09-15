package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// conn's own configuration: a file the operator keeps, holding what
// conn cannot work out for itself and would otherwise have to be told
// again on every start. conn reads it and never writes it — what is in
// it was put there on purpose, so a file that cannot be read is said
// out loud rather than passed over for the defaults.

// A config is what the file says. A field left out is not set, and what
// conn would have done without a file at all still stands.
type config struct {
	// Roots are the directories conn looks for projects under. A path
	// may be written with a leading ~, which is the home of whoever is
	// running conn: the file is read by conn, not by a shell, so there
	// is nothing else to expand it.
	Roots []string `json:"roots"`
}

// configHome is where a program's configuration goes: XDG_CONFIG_HOME,
// or ~/.config. conn keeps its own under here, and finds other
// programs' directories the same way when it has something to write
// them — a colorscheme for nvim.
func configHome(home string) string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return dir
	}
	return filepath.Join(home, ".config")
}

// configPath is the file conn reads its configuration from.
func configPath(home string) string {
	return filepath.Join(configHome(home), "conn", "config.json")
}

// readConfig reads the file. No file is not an error: a machine without
// one is the ordinary case and every default holds. A file that is
// there and will not parse is an error, and the error names the file,
// since the reader's next move is to open it.
func readConfig(home string) (config, error) {
	path := configPath(home)
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return config{}, nil
	}
	if err != nil {
		return config{}, err
	}
	var c config
	if err := json.Unmarshal(b, &c); err != nil {
		return config{}, fmt.Errorf("%s: %w", tilde(path, home), err)
	}
	return c, nil
}

// expandHome is a path as conn will use it: a leading ~ is the home,
// and everything else is left as it was written.
func expandHome(path, home string) string {
	if home == "" || path == "" || path[0] != '~' {
		return path
	}
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path // ~someone else: not conn's to guess at
}

// Which of the three answers the roots were taken from, for the console
// to say where what it is showing came from.
type rootSource int

const (
	rootsDefault rootSource = iota
	rootsEnv
	rootsFile
)

// resolveRoots is projectRoots with the source it took them from.
func resolveRoots(home string) ([]string, rootSource, error) {
	if out := splitRoots(os.Getenv("CONN_ROOTS"), home); len(out) > 0 {
		return out, rootsEnv, nil
	}
	c, err := readConfig(home)
	if err != nil {
		return defaultRoots(home), rootsDefault, err
	}
	if out := cleanRoots(c.Roots, home); len(out) > 0 {
		return out, rootsFile, nil
	}
	return defaultRoots(home), rootsDefault, nil
}

// A configState is conn's configuration as the console found it: the
// file and how it read, and the roots in force with what each one turned
// out to be on this machine.
type configState struct {
	path    string
	present bool
	err     error
	// names is whether the file named roots conn understands. A file
	// that is there, parses, and names none is the quiet mistake this
	// has: conn goes on with the defaults and nothing says the file was
	// wasted. It is read whether or not the file is what is in force,
	// since the environment standing in front of it does not make an
	// unread file any less of a mistake.
	names  bool
	source rootSource
	roots  []rootState
}

// A rootState is one configured directory and what is actually there.
type rootState struct {
	path    string
	problem string // "", rootMissing, rootNotDir
}

const (
	rootMissing = "missing"
	rootNotDir  = "not a dir"
)

// readConfigState reads the configuration the way the console reports
// it: the file, the roots it settled on, and a look at each root, since
// a root that is not there is the mistake this is most often made with.
func readConfigState(home string) configState {
	s := configState{path: configPath(home)}
	if _, err := os.Stat(s.path); err == nil {
		s.present = true
	}
	c, err := readConfig(home)
	s.err = err
	s.names = len(cleanRoots(c.Roots, home)) > 0
	var roots []string
	roots, s.source, _ = resolveRoots(home)
	for _, r := range roots {
		s.roots = append(s.roots, rootStateOf(r))
	}
	return s
}

// rootStateOf is what a configured directory turned out to be.
func rootStateOf(path string) rootState {
	info, err := os.Stat(path)
	switch {
	case err != nil:
		return rootState{path: path, problem: rootMissing}
	case !info.IsDir():
		return rootState{path: path, problem: rootNotDir}
	}
	return rootState{path: path}
}

// editor is the program the operator edits with: VISUAL, then EDITOR,
// then vi, which is the one a machine can be relied on to have.
func editor() string {
	for _, name := range []string{"VISUAL", "EDITOR"} {
		if e := strings.TrimSpace(os.Getenv(name)); e != "" {
			return e
		}
	}
	return "vi"
}

// seedConfig is the file conn writes for an operator who has none:
// the roots conn is walking as it stands, so that opening it shows what
// conn is actually doing rather than an empty buffer to guess at.
func seedConfig(roots []string, home string) []byte {
	c := config{Roots: make([]string, 0, len(roots))}
	for _, r := range roots {
		c.Roots = append(c.Roots, tilde(r, home))
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return append(b, '\n')
}

// openableConfig is the config file with something in it to edit: the
// path, made if it was not there. conn reads the file and does not
// write it, and this is not conn writing it — it is conn making the
// file the operator asked to edit, once, before handing it over.
func openableConfig(home string) (string, error) {
	path := configPath(home)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	roots, _, _ := resolveRoots(home)
	if err := os.WriteFile(path, seedConfig(roots, home), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// editConfigCommand is what conn runs to put the operator in the file.
func editConfigCommand(path string) string {
	return editor() + " " + shellQuote(path)
}
