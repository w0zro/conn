package theme

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// A theme is a sheet, NAME.json, worn by that name: one in conn's
// themes directory, or one conn ships with. A sheet in the directory
// stands in for one conn ships under the same name.
//
// conn ships three. conn is its own. datum is the colorscheme at
// datum.w0zro.com, worn the way its terminal ports do: the sixteen,
// the ground, the ink, the cursor and the selection are its Ghostty
// port's, the washes, the bands and the quietest text its Claude Code
// port's, and where datum has no name for a role the nearest thing it
// does have stands in. skelly is skelly's brand, the palette it calls
// Glow: neon is the reds; black light the magentas and a mark at work;
// a thing running has no hue and is drawn in ink. Phosphor is the
// accent: --acc at night, spent as conn's and datum's accent is. By
// day --acc is a fill too pale to read as text, so the accent is
// --acc-text, deep enough to read, and --acc is its fill: the block,
// with ink on it, the cursor, and the row under the cursor, with
// --acc-text on it beside a bar of --acc-text. Its sixteen, ground, ink
// and night cursor are what the brand's palette.mjs --terminal writes,
// and the rest are the brand's rules on its five colors. The tests hold both to copies of
// their sources under testdata; refreshing one is copying those again.

//go:embed sheets/*.json
var shipped embed.FS

// shippedSheet is the sheet conn ships under a name, if it ships one.
func shippedSheet(name string) ([]byte, bool) {
	b, err := shipped.ReadFile(path.Join("sheets", name+".json"))
	return b, err == nil
}

// Load is the theme by its name: the sheet in dir, or the one conn
// ships.
func Load(dir, name string) (Theme, error) {
	if name == "" || strings.ContainsAny(name, `/\`) || strings.HasPrefix(name, ".") {
		return Theme{}, fmt.Errorf("%q cannot be a theme's name", name)
	}
	b, err := os.ReadFile(filepath.Join(dir, name+".json"))
	if errors.Is(err, fs.ErrNotExist) {
		var ok bool
		if b, ok = shippedSheet(name); !ok {
			return Theme{}, fmt.Errorf("there is no theme %s; there are %s, in %s", name, strings.Join(Names(dir), ", "), dir)
		}
	} else if err != nil {
		return Theme{}, err
	}
	return Parse(name, b)
}

// Names are the themes there are: the ones conn ships, and every
// sheet in dir.
func Names(dir string) []string {
	shippedNames, _ := fs.Glob(shipped, "sheets/*.json")
	sheets, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	var names []string
	for _, s := range append(shippedNames, sheets...) {
		if n := strings.TrimSuffix(filepath.Base(s), ".json"); !slices.Contains(names, n) {
			names = append(names, n)
		}
	}
	slices.Sort(names)
	return names
}
