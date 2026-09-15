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
