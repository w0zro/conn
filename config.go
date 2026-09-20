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
// again on every start. What is in it was put there on purpose, so a
// file that cannot be read is said out loud rather than passed over
// for the defaults, and a file conn writes keeps every key it did not
// come to change; see saveSetting, and settings.go for the view the
// operator changes one in.

// A config is what the file says. A field left out is not set, and what
// conn would have done without a file at all still stands.
type config struct {
	// Roots are the directories conn looks for projects under. A path
	// may be written with a leading ~, which is the home of whoever is
	// running conn: the file is read by conn, not by a shell, so there
	// is nothing else to expand it.
	Roots []string `json:"roots"`
	// Theme is the theme conn comes up in: conn unless set. A name conn
	// has no theme by is conn's, and the console says so.
	Theme string `json:"theme"`
	// Ground is the ground conn comes up on: dark or light. Left out,
	// conn asks the terminal its own with OSC 11, which is what it has
	// always done and what most stations want; a file naming one is an
	// operator who wants the same ground whatever terminal they are at.
	// A word that is neither is asked for the same way, and the console
	// says so.
	Ground string `json:"ground"`
}

// The grounds, as the file names them.
const (
	darkGround  = "dark"
	lightGround = "light"
)

// groundNamed reads a ground the file names: whether it is dark, and
// whether conn has a ground by that name at all. Nothing named is not a
// mistake — it is the terminal's to answer.
func groundNamed(name string) (dark, ok bool) {
	switch name {
	case darkGround:
		return true, true
	case lightGround:
		return false, true
	}
	return false, false
}

// configTheme is the theme the file names, when conn has one by that
// name, and conn's own otherwise: a file that cannot be read is the
// console's to report, not a reason to come up in nothing.
func configTheme(home string) string {
	c, _ := readConfig(home)
	if _, ok := themeNamed(c.Theme); ok {
		return c.Theme
	}
	return defaultTheme
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
	rootsNone rootSource = iota
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
		return nil, rootsNone, err
	}
	if out := cleanRoots(c.Roots, home); len(out) > 0 {
		return out, rootsFile, nil
	}
	return nil, rootsNone, nil
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
	names bool
	// theme is the theme the file names, as written, and noSuchTheme
	// whether conn has none by that name. A file naming none means
	// conn's own, which is not a mistake.
	theme       string
	noSuchTheme bool
	// ground is the ground the file names, as written, and
	// noSuchGround whether it is a word conn knows. A file naming none
	// means the terminal's own, which is not a mistake.
	ground       string
	noSuchGround bool
	source       rootSource
	roots        []rootState
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
	s.theme = c.Theme
	if _, ok := themeNamed(c.Theme); c.Theme != "" && !ok {
		s.noSuchTheme = true
	}
	s.ground = c.Ground
	if _, ok := groundNamed(c.Ground); c.Ground != "" && !ok {
		s.noSuchGround = true
	}
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

// saveRoots writes the roots into the config file.
func saveRoots(home string, roots []string) error {
	return saveSetting(home, "roots", roots)
}

// saveTheme writes the theme conn is to come up in.
func saveTheme(home, name string) error {
	return saveSetting(home, "theme", name)
}

// saveGround writes the ground conn is to come up on, and takes the key
// out again for a ground of nothing — which is not the same as a ground
// written empty. The file is what conn reads; a key that is there says
// somebody decided, and giving the choice back to the terminal is
// taking the decision out rather than writing down a blank one.
func saveGround(home, name string) error {
	if name == "" {
		return dropSetting(home, "ground")
	}
	return saveSetting(home, "ground", name)
}

// saveSetting writes one setting into the config file, keeping
// whatever else is in it. conn reads this file and otherwise does not
// write it, and these are the one place that does, because conn asked
// for the answer and the operator gave it: the alternative is telling
// somebody the path to a file and the spelling of a key and sending
// them away to type it themselves.
func saveSetting(home, key string, value any) error {
	set, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return writeSetting(home, key, set)
}

// dropSetting takes a key out of the file, which is how a setting is
// given back to whatever conn would have done without one.
func dropSetting(home, key string) error {
	return writeSetting(home, key, nil)
}

// writeSetting is the read, the merge and the write both of those are;
// a value of nothing takes the key out.
//
// What conn does not understand is carried through untouched. A file
// may hold settings from a conn older or newer than this one, and a
// save that dropped them would be conn deciding they did not matter.
// The keys are written in the order Go writes a map, which is sorted;
// the file is small and the order is not what it is for.
func writeSetting(home, key string, set json.RawMessage) error {
	path := configPath(home)
	fields := map[string]json.RawMessage{}
	if b, err := os.ReadFile(path); err == nil {
		// A file that will not parse is not merged into. conn cannot
		// tell what is in it, so it cannot keep it, and overwriting is
		// how somebody's file is lost.
		if err := json.Unmarshal(b, &fields); err != nil {
			return fmt.Errorf("%s will not parse, and conn will not write over it", tilde(path, home))
		}
	}
	if set == nil {
		delete(fields, key)
	} else {
		fields[key] = set
	}
	out, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o644)
}
