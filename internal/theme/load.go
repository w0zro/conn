package theme

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// A theme is a sheet in conn's themes directory, NAME.json, worn by
// that name. conn's own is there by default, and a sheet named conn
// stands in for it.

// Load is the theme by its name: the sheet in dir, or conn's own.
func Load(dir, name string) (Theme, error) {
	if name == "" || strings.ContainsAny(name, `/\`) || strings.HasPrefix(name, ".") {
		return Theme{}, fmt.Errorf("%q cannot be a theme's name", name)
	}
	b, err := os.ReadFile(filepath.Join(dir, name+".json"))
	switch {
	case errors.Is(err, fs.ErrNotExist) && name == Conn.Name:
		return Conn, nil
	case errors.Is(err, fs.ErrNotExist):
		return Theme{}, fmt.Errorf("there is no theme %s; there are %s, in %s", name, strings.Join(Names(dir), ", "), dir)
	case err != nil:
		return Theme{}, err
	}
	return Parse(name, b)
}

// Names are the themes there are: conn's own, and every sheet in dir.
func Names(dir string) []string {
	names := []string{Conn.Name}
	sheets, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	for _, s := range sheets {
		if n := strings.TrimSuffix(filepath.Base(s), ".json"); !slices.Contains(names, n) {
			names = append(names, n)
		}
	}
	slices.Sort(names)
	return names
}
