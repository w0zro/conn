package main

import (
	_ "embed"
	"os"
	"path/filepath"
)

// The manual conn carries. It is the page written from docs/index.html
// by tools/man, built into the binary rather than looked for on the
// machine: a conn run from a build directory has no installed page, and
// an installed conn may have one from an older release beside a newer
// binary. The manual a conn shows is the manual that conn was built
// with, which is the only one guaranteed to describe it.
//
//go:embed man/conn.1
var manPage []byte

// manPath is where conn puts the page for man to read, beside its own
// state. It is written on each showing rather than kept current,
// because it costs nothing and a stale copy is the thing this exists to
// avoid.
func manPath(home string) string {
	return filepath.Join(stateHome(home), "conn", "conn.1")
}

// writeManPage puts the manual where man can be pointed at it.
func writeManPage(home string) (string, error) {
	path := manPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, manPage, 0o644)
}

// manCommand is what conn runs to show the manual: man, pointed at the
// page conn just wrote. man takes a path where it finds a separator in
// it, on both the platforms conn runs on.
func manCommand(path string) string {
	return "man " + shellQuote(path)
}
