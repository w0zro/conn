package main

import (
	"os"
	"testing"
)

// The tests run on one station, whatever machine they run on. A model
// made by newModel reads conn's configuration the way conn does — the
// environment, then the file under the user's config directory — and
// a test that came up on the developer's own file came up told where
// the work was, while the same test on a runner with no file came up
// told nothing and went to the asking view instead of the processes
// view. The suite passed here and failed there for the same code.
//
// So every test starts from the same answer: the roots the fixtures
// already name, given by the environment, and a config directory of
// the run's own with nothing in it. A test about the configuration
// itself sets both for itself, as those tests already do, and what it
// sets stands over this.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "conn-test-config-")
	if err != nil {
		panic(err)
	}
	for name, value := range map[string]string{"XDG_CONFIG_HOME": dir, "CONN_ROOTS": "/Users/w0zro/projects"} {
		if err := os.Setenv(name, value); err != nil {
			panic(err)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(dir) // a temp directory the system sweeps anyway
	os.Exit(code)
}
