package console

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/w0zro/conn/internal/config"
)

// The console's tests run on one station, as conn's own do and for the
// same reasons, which main's testmain_test.go gives: the roots from the
// environment and a config directory of the run's own, and the clock in
// the zone the files of record were written in.
func TestMain(m *testing.M) {
	time.Local = time.FixedZone("PDT", -7*60*60)
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

// writeConfig puts a config file where conn will look for it, under a
// config home of the test's own, and answers the home it was written
// for.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	home := t.TempDir()
	path := config.Path(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}
