package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestThePlacesTestsRunTheWayThePlaceSays(t *testing.T) {
	// What the project wrote down for the purpose comes before what its
	// ecosystem does by default, and a Makefile's test target over
	// everything: it is where a project says the guess is wrong.
	cases := []struct {
		name  string
		files map[string]string
		dirs  []string
		run   string
		from  string
	}{
		{"a Makefile target over go", map[string]string{"Makefile": "build:\n\tgo build\n\ntest:\n\tgo test -race ./...\n", "go.mod": "module x\n"}, nil, "make test", "Makefile"},
		{"a Makefile without a test target does not count", map[string]string{"Makefile": "build:\n\tgo build\n", "go.mod": "module x\n"}, nil, "go test ./...", "go.mod"},
		{"a make variable named test is not a target", map[string]string{"Makefile": "test := yes\n", "go.mod": "module x\n"}, nil, "go test ./...", "go.mod"},
		{"a just recipe with parameters", map[string]string{"justfile": "build:\n  cargo build\ntest filter='':\n  cargo test {{filter}}\n", "Cargo.toml": ""}, nil, "just test", "justfile"},
		{"a Taskfile task", map[string]string{"Taskfile.yml": "version: '3'\ntasks:\n  build:\n    cmds: [go build]\n  test:\n    cmds: [go test ./...]\n"}, nil, "task test", "Taskfile.yml"},
		{"a package.json test script", map[string]string{"package.json": `{"scripts":{"test":"vitest run","dev":"vite"}}`}, nil, "npm test", "package.json"},
		{"a package.json without one falls through", map[string]string{"package.json": `{"scripts":{"dev":"vite"}}`}, nil, "", ""},
		{"go", map[string]string{"go.mod": "module x\n"}, nil, "go test ./...", "go.mod"},
		{"cargo", map[string]string{"Cargo.toml": ""}, nil, "cargo test", "Cargo.toml"},
		{"mix", map[string]string{"mix.exs": ""}, nil, "mix test", "mix.exs"},
		{"django before pytest", map[string]string{"manage.py": "", "pyproject.toml": ""}, nil, "python3 manage.py test", "manage.py"},
		{"pytest", map[string]string{"pyproject.toml": ""}, nil, "pytest", "pyproject.toml"},
		{"rspec when there is a spec directory", map[string]string{"Gemfile": ""}, []string{"spec"}, "bundle exec rspec", "Gemfile"},
		{"rake when there is a Rakefile and no spec", map[string]string{"Gemfile": "", "Rakefile": ""}, nil, "bundle exec rake test", "Gemfile"},
		{"a gemfile alone says nothing", map[string]string{"Gemfile": ""}, nil, "", ""},
		{"gradle wrapper over gradle", map[string]string{"gradlew": "", "build.gradle": ""}, nil, "./gradlew test", "gradlew"},
		{"nothing", map[string]string{"README.md": ""}, nil, "", ""},
	}
	for _, tc := range cases {
		dir := t.TempDir()
		for name, body := range tc.files {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		for _, d := range tc.dirs {
			if err := os.Mkdir(filepath.Join(dir, d), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		run, from, ok := testCommand(dir)
		if ok != (tc.run != "") || run != tc.run || from != tc.from {
			t.Errorf("%s: testCommand = %q from %q (%v), want %q from %q", tc.name, run, from, ok, tc.run, tc.from)
		}
	}
}
