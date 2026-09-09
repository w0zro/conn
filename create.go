package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// A new project is a directory with a repository in it, made where the
// others are. Opening what does not exist creates it, like a new file: a
// name typed at the finder that matches nothing offers to make the repo
// and open its shell, and enter takes the offer. The name goes under the
// first of the roots; a path with folders in it makes the folders on the
// way, a new group being one; an absolute path, or one from ~, goes where
// it says. Nothing here is a key of its own any more — creation is the
// finder's, the way everything openable is.

// newProjectPath is where a typed name says the project goes: a name, or a
// path with the name on the end, under the root; an absolute path, or one
// from ~, is where it says. A path that climbs out of the root would put
// the project somewhere the line did not say.
func newProjectPath(root, typed string) (string, error) {
	typed = strings.TrimSpace(typed)
	if typed == "" {
		return "", errors.New("a name is needed")
	}
	dir := expandPath(typed)
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(root, dir)
	}
	dir = filepath.Clean(dir)
	if dir == filepath.Clean(root) || strings.Contains(typed, "..") {
		return "", errors.New("a name, or a path to one: " + typed)
	}
	return dir, nil
}

// createProject makes the directory — and the folders on the way to it,
// a new group being one — and the repository in it. A directory already
// there is not made over — it may well be a project, and this is no way
// to find out — and a git that fails leaves nothing behind.
func createProject(dir string) error {
	if _, err := os.Lstat(dir); err == nil {
		return errors.New(dir + " is already there")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "git", "init", "-q", dir).CombinedOutput(); err != nil {
		_ = os.Remove(dir)
		said := strings.TrimSpace(string(out))
		if said == "" {
			said = err.Error()
		}
		return errors.New("git init: " + said)
	}
	return nil
}
