// Package config is conn's own configuration: a file the operator
// keeps, holding what conn cannot work out for itself and would
// otherwise have to be told again on every start. What is in it was put
// there on purpose, so a file that cannot be read is said out loud
// rather than passed over for the defaults, and a file conn writes
// keeps every key it did not come to change; see saveSetting.
//
// It is also where conn keeps things: the home, written from ~ and
// back, and the directories under it that XDG names for configuration
// and state.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// A File is what the file says. A field left out is not set, and what
// conn would have done without a file at all still stands.
type File struct {
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
	DarkGround  = "dark"
	LightGround = "light"
)

// GroundNamed reads a ground the file names: whether it is dark, and
// whether conn has a ground by that name at all. Nothing named is not a
// mistake — it is the terminal's to answer.
func GroundNamed(name string) (dark, ok bool) {
	switch name {
	case DarkGround:
		return true, true
	case LightGround:
		return false, true
	}
	return false, false
}

// Home is where a program's configuration goes: XDG_CONFIG_HOME,
// or ~/.config. conn keeps its own under here, and finds other
// programs' directories the same way when it has something to write
// them — a colorscheme for nvim.
func Home(home string) string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return dir
	}
	return filepath.Join(home, ".config")
}

// Path is the file conn reads its configuration from.
func Path(home string) string {
	return filepath.Join(Home(home), "conn", "config.json")
}

// Read reads the file. No file is not an error: a machine without
// one is the ordinary case and every default holds. A file that is
// there and will not parse is an error, and the error names the file,
// since the reader's next move is to open it.
func Read(home string) (File, error) {
	path := Path(home)
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return File{}, nil
	}
	if err != nil {
		return File{}, err
	}
	var c File
	if err := json.Unmarshal(b, &c); err != nil {
		return File{}, fmt.Errorf("%s: %w", Tilde(path, home), err)
	}
	return c, nil
}

// ExpandHome is a path as conn will use it: a leading ~ is the home,
// and everything else is left as it was written.
func ExpandHome(path, home string) string {
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

// RootSource is which of the three answers the roots were taken from,
// for the console to say where what it is showing came from.
type RootSource int

const (
	rootsNone RootSource = iota
	RootsEnv
	RootsFile
)

// ResolveRoots is Roots with the source it took them from.
func ResolveRoots(home string) ([]string, RootSource, error) {
	if out := splitRoots(os.Getenv("CONN_ROOTS"), home); len(out) > 0 {
		return out, RootsEnv, nil
	}
	c, err := Read(home)
	if err != nil {
		return nil, rootsNone, err
	}
	if out := CleanRoots(c.Roots, home); len(out) > 0 {
		return out, RootsFile, nil
	}
	return nil, rootsNone, nil
}

// A State is conn's configuration as the console found it: the
// file and how it read, and the roots in force with what each one turned
// out to be on this machine.
type State struct {
	Path    string
	Present bool
	Err     error
	// names is whether the file named roots conn understands. A file
	// that is there, parses, and names none is the quiet mistake this
	// has: conn goes on with the defaults and nothing says the file was
	// wasted. It is read whether or not the file is what is in force,
	// since the environment standing in front of it does not make an
	// unread file any less of a mistake.
	Names bool
	// theme is the theme the file names, as written. A file naming none
	// means conn's own, which is not a mistake.
	Theme string
	// ground is the ground the file names, as written, and
	// noSuchGround whether it is a word conn knows. A file naming none
	// means the terminal's own, which is not a mistake.
	ground       string
	NoSuchGround bool
	Source       RootSource
	Roots        []RootState
}

// A RootState is one configured directory and what is actually there.
type RootState struct {
	Path    string
	Problem string // "", rootMissing, rootNotDir
}

const (
	RootMissing = "missing"
	RootNotDir  = "not a dir"
)

// ReadState reads the configuration the way the console reports
// it: the file, the roots it settled on, and a look at each root, since
// a root that is not there is the mistake this is most often made with.
func ReadState(home string) State {
	s := State{Path: Path(home)}
	if _, err := os.Stat(s.Path); err == nil {
		s.Present = true
	}
	c, err := Read(home)
	s.Err = err
	s.Names = len(CleanRoots(c.Roots, home)) > 0
	s.Theme = c.Theme
	s.ground = c.Ground
	if _, ok := GroundNamed(c.Ground); c.Ground != "" && !ok {
		s.NoSuchGround = true
	}
	var roots []string
	roots, s.Source, _ = ResolveRoots(home)
	for _, r := range roots {
		s.Roots = append(s.Roots, RootStateOf(r))
	}
	return s
}

// RootStateOf is what a configured directory turned out to be.
func RootStateOf(path string) RootState {
	info, err := os.Stat(path)
	switch {
	case err != nil:
		return RootState{Path: path, Problem: RootMissing}
	case !info.IsDir():
		return RootState{Path: path, Problem: RootNotDir}
	}
	return RootState{Path: path}
}

// SaveRoots writes the roots into the config file.
func SaveRoots(home string, roots []string) error {
	return saveSetting(home, "roots", roots)
}

// SaveTheme writes the theme conn is to come up in.
func SaveTheme(home, name string) error {
	return saveSetting(home, "theme", name)
}

// SaveGround writes the ground conn is to come up on, and takes the key
// out again for a ground of nothing — which is not the same as a ground
// written empty. The file is what conn reads; a key that is there says
// somebody decided, and giving the choice back to the terminal is
// taking the decision out rather than writing down a blank one.
func SaveGround(home, name string) error {
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
	path := Path(home)
	fields := map[string]json.RawMessage{}
	if b, err := os.ReadFile(path); err == nil {
		// A file that will not parse is not merged into. conn cannot
		// tell what is in it, so it cannot keep it, and overwriting is
		// how somebody's file is lost.
		if err := json.Unmarshal(b, &fields); err != nil {
			return fmt.Errorf("%s will not parse, and conn will not write over it", Tilde(path, home))
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

// Roots are the directories conn looks for projects under: CONN_ROOTS,
// a list in the path list separator's spelling, and otherwise the ones
// the config file names. The environment is asked first because it is
// the nearer word — a conn started for one job, on one set of roots,
// ahead of the file that says what is usually meant. A root that is not
// on this machine is still a root; the walk decides whether it is
// there, not the environment.
//
// Told neither, conn has no roots and walks nothing. There was a
// ~/projects underneath this and there is not any more: a default is a
// guess at where somebody keeps their work, and a wrong guess is
// invisible, because conn walks the wrong tree and every line of the
// console still reads nominal. Where conn has not been told, it says so
// and waits, which is something the operator can see and put right.
//
// A config file that will not parse is an error and no roots. The file
// was meant to be read, it could not be, and conn is not going to
// invent what it was probably about to say.
func Roots(home string) ([]string, error) {
	roots, _, err := ResolveRoots(home)
	return roots, err
}

// splitRoots is a list of roots as the environment writes one, in the
// path list separator's spelling.
func splitRoots(list, home string) []string {
	if list == "" {
		return nil
	}
	return CleanRoots(filepath.SplitList(list), home)
}

// CleanRoots is the roots as conn will walk them: the blanks dropped,
// and a leading ~ made the home it stands for.
func CleanRoots(roots []string, home string) []string {
	var out []string
	for _, d := range roots {
		if d = strings.TrimSpace(d); d != "" {
			out = append(out, ExpandHome(d, home))
		}
	}
	return out
}

// Tilde writes a path under home from ~.
func Tilde(path, home string) string {
	if home != "" && (path == home || strings.HasPrefix(path, home+"/")) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

// StateHome is where state goes: XDG_STATE_HOME, or ~/.local/state.
func StateHome(home string) string {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return dir
	}
	return filepath.Join(home, ".local", "state")
}
