package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// A project's tasks — its tests, its build, its lint — run the way the
// project already says, not the way a file of conn's own would have it:
// conn reads the tool the project has — a Makefile target, a package.json
// script, the ecosystem's own runner — and runs that. A project the guess
// is wrong for says so in its Makefile, which conn checks first. "Run
// the tests" is an intent, and the point of conn knowing what it means
// here is that conn starts the run, and so sees how it ends.

// verb is one task a place can be asked for by name: how it is spoken of,
// and what its ecosystems run for it. The name is what the shell running
// it is called — the name its window shows and its row reads — and the
// recipe a project's own files would name it by.
type verb struct {
	name    string // the task, and the shell's name: test, build, lint
	key     string // the navigator's key for it
	label   string // the pane's line: tests, build, lint
	doing   string // the status line's word while it runs: testing
	idle    string // the pane's word before it has run: not run
	done    string // the pane's word when it ended well: passed
	unknown string // the status line's complaint for a place that says nothing: how its tests run
	// natives is each ecosystem's own command for the task, by the file
	// that marks a project as its own — and, for the ones whose command
	// depends on a second file, that file. Order settles a project that
	// is two things at once: the more specific first.
	natives []native
}

type native struct{ marks, needs, run string }

// testName is the test verb's name, which the pane and the tests share.
const testName = "test"

// verbs is every task, in the order the pane lists them.
var verbs = []*verb{
	{name: testName, key: "t", label: "tests", doing: "testing", idle: "not run", done: "passed", unknown: "how its tests run",
		natives: []native{
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
		}},
	{name: "build", key: "b", label: "build", doing: "building", idle: "not built", done: "built", unknown: "how it builds",
		natives: []native{
			{"go.mod", "", "go build ./..."},
			{"Cargo.toml", "", "cargo build"},
			{"mix.exs", "", "mix compile"},
			{"gradlew", "", "./gradlew build"},
			{"build.gradle", "", "gradle build"},
			{"build.gradle.kts", "", "gradle build"},
			{"pom.xml", "", "mvn package"},
			{"Package.swift", "", "swift build"},
			{"dune-project", "", "dune build"},
		}},
	// Only the linters an ecosystem ships with are guessed at: a project
	// on ruff or eslint names them in a script or a Makefile, and a guess
	// that is not installed fails the run for nothing.
	{name: "lint", key: "l", label: "lint", doing: "linting", idle: "not linted", done: "clean", unknown: "how it lints",
		natives: []native{
			{"go.mod", "", "go vet ./..."},
			{"Cargo.toml", "", "cargo clippy"},
			{"deno.json", "", "deno lint"},
			{"deno.jsonc", "", "deno lint"},
		}},
}

// verbNamed is the verb by its name, or nil.
func verbNamed(name string) *verb {
	for _, v := range verbs {
		if v.name == name {
			return v
		}
	}
	return nil
}

// command is how a place runs the verb, and where that was learned from.
// The conventions are read in the order they are trustworthy: what the
// project wrote down for the purpose — a Makefile target, a justfile
// recipe, a Taskfile task, a package.json script, each named for the
// verb — over what its ecosystem does by default.
func (v *verb) command(dir string) (run, source string, ok bool) {
	for _, name := range []string{"Makefile", "makefile", "GNUmakefile"} {
		if hasRecipe(filepath.Join(dir, name), makeTarget(v.name)) {
			return "make " + v.name, name, true
		}
	}
	for _, name := range []string{"justfile", "Justfile", ".justfile"} {
		if hasRecipe(filepath.Join(dir, name), justRecipe(v.name)) {
			return "just " + v.name, name, true
		}
	}
	for _, name := range []string{"Taskfile.yml", "Taskfile.yaml"} {
		if hasRecipe(filepath.Join(dir, name), taskfileTask(v.name)) {
			return "task " + v.name, name, true
		}
	}
	if hasScript(filepath.Join(dir, "package.json"), v.name) {
		if v.name == testName {
			return "npm test", "package.json", true
		}
		return "npm run " + v.name, "package.json", true
	}
	for _, r := range v.natives {
		if exists(filepath.Join(dir, r.marks)) && (r.needs == "" || exists(filepath.Join(dir, r.needs))) {
			return r.run, r.marks, true
		}
	}
	return "", "", false
}

// The shapes a recipe takes in the files that hold recipes: a make target's
// name before its colon, a just recipe's name before its parameters and
// colon, a Taskfile task two spaces in under tasks. A name followed by :=
// is a variable in make and in just, not a recipe.
func makeTarget(name string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(name) + `\s*:([^=]|$)`)
}

func justRecipe(name string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^@?` + regexp.QuoteMeta(name) + `(\s+[^:]*)?:([^=]|$)`)
}

func taskfileTask(name string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^  ` + regexp.QuoteMeta(name) + `:`)
}

// hasRecipe reports whether a recipe file names the recipe.
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
