package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// A project's tests are run the way the project already says, not the way
// a file of conn's own would have it: conn reads the tool the project has —
// a Makefile target, a package.json script, the ecosystem's own runner —
// and runs that. A project the guess is wrong for says so in its Makefile,
// which conn believes first. "Run the tests" is an intent, and the point of
// conn knowing what it means here is that conn starts the run, and so sees
// how it ends.

// testName is what the shell running a place's tests is called: the name
// its window wears and its row reads, the way a plan entry's does.
const testName = "test"

// testCommand is how a place runs its tests, and where that was learned
// from. The conventions are read in the order they are trustworthy: what
// the project wrote down for the purpose — a Makefile target, a justfile
// recipe, a Taskfile task, a package.json script — over what its ecosystem
// does by default.
func testCommand(dir string) (run, source string, ok bool) {
	for _, name := range []string{"Makefile", "makefile", "GNUmakefile"} {
		if hasRecipe(filepath.Join(dir, name), makeTarget) {
			return "make test", name, true
		}
	}
	for _, name := range []string{"justfile", "Justfile", ".justfile"} {
		if hasRecipe(filepath.Join(dir, name), justRecipe) {
			return "just test", name, true
		}
	}
	for _, name := range []string{"Taskfile.yml", "Taskfile.yaml"} {
		if hasRecipe(filepath.Join(dir, name), taskfileTask) {
			return "task test", name, true
		}
	}
	if hasScript(filepath.Join(dir, "package.json"), "test") {
		return "npm test", "package.json", true
	}

	// The ecosystem's own runner, by the file that marks the ecosystem.
	for _, c := range ecosystemTests {
		if exists(filepath.Join(dir, c.marks)) && (c.needs == "" || exists(filepath.Join(dir, c.needs))) {
			return c.run, c.marks, true
		}
	}
	return "", "", false
}

// ecosystemTests is each ecosystem's test runner, by the file that marks a
// project as its own — and, for the ones whose runner depends on a second
// file, that file. Order settles a project that is two things at once:
// the more specific first.
var ecosystemTests = []struct{ marks, needs, run string }{
	{"manage.py", "", "python3 manage.py test"},
	{"go.mod", "", "go test ./..."},
	{"Cargo.toml", "", "cargo test"},
	{"mix.exs", "", "mix test"},
	{"deno.json", "", "deno test"},
	{"deno.jsonc", "", "deno test"},
	{"gradlew", "", "./gradlew test"},
	{"build.gradle", "", "gradle test"},
	{"build.gradle.kts", "", "gradle test"},
	{"pom.xml", "", "mvn test"},
	{"Package.swift", "", "swift test"},
	{"dune-project", "", "dune test"},
	{"Gemfile", "spec", "bundle exec rspec"},
	{"Gemfile", "Rakefile", "bundle exec rake test"},
	{"pyproject.toml", "", "pytest"},
	{"pytest.ini", "", "pytest"},
	{"setup.cfg", "", "pytest"},
	{"setup.py", "", "pytest"},
	{"tox.ini", "", "tox"},
}

// The shapes a test recipe takes in the files that hold recipes: a make
// target's name before its colon, a just recipe's name before its
// parameters and colon, a Taskfile task two spaces in under tasks.
var (
	makeTarget   = regexp.MustCompile(`(?m)^test\s*:([^=]|$)`)
	justRecipe   = regexp.MustCompile(`(?m)^@?test(\s+[^:]*)?:([^=]|$)`)
	taskfileTask = regexp.MustCompile(`(?m)^  test:`)
)

// hasRecipe reports whether a recipe file names a test recipe.
func hasRecipe(path string, shape *regexp.Regexp) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return shape.Match(b)
}

// hasScript reports whether a package.json has the named script.
func hasScript(path, name string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(b, &pkg); err != nil {
		return false
	}
	return strings.TrimSpace(pkg.Scripts[name]) != ""
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
