package theme

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

// Every color of either ground parses, the dark ground is dark and
// the light one light, and a slot and a role are written alike.
func TestGrounds(t *testing.T) {
	for name, g := range map[string]Ground{"dark": Dark, "light": Light} {
		if (name == "dark") != g.Dark() {
			t.Errorf("%s ground reads as dark = %v", name, g.Dark())
		}
		for i, s := range g.Scheme {
			RGB(s)
			if Hex(RGB(s)) != s {
				t.Errorf("%s slot %d %s is not written as read", name, i, s)
			}
		}
		for role, h := range g.Roles() {
			if h == "" {
				t.Errorf("%s has no %s", name, role)
				continue
			}
			RGB(h)
		}
	}
	if !IsDark(color.RGBA{A: 255}) || IsDark(color.RGBA{R: 255, G: 255, B: 255, A: 255}) {
		t.Error("black is dark and white is light")
	}
}

// The ground a server came up on is read back as written, and a file
// from a build that named the theme too reads the same.
func TestMode(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "s")
	if _, ok := ReadMode(socket); ok {
		t.Error("a mode was read before one was written")
	}
	if err := WriteMode(socket, false); err != nil {
		t.Fatal(err)
	}
	if dark, ok := ReadMode(socket); !ok || dark {
		t.Errorf("ReadMode = %v, %v", dark, ok)
	}
	if err := os.WriteFile(ModePath(socket), []byte("dark\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if dark, ok := ReadMode(socket); !ok || !dark {
		t.Errorf("ReadMode of a one-word file = %v, %v", dark, ok)
	}
	ForgetMode(socket)
	if _, ok := ReadMode(socket); ok {
		t.Error("a mode was read after it was forgotten")
	}
}

// --light and --dark are read off the front of the arguments and no
// further, and both at once is a contradiction.
func TestParseFlags(t *testing.T) {
	rest, want, err := ParseFlags([]string{"ls", "--light"})
	if err != nil || want != nil || len(rest) != 2 {
		t.Errorf("a flag after the command is the command's: %v %v %v", rest, want, err)
	}
	rest, want, err = ParseFlags([]string{"--light", "--light", "u", "--dark"})
	if err != nil || want == nil || *want || len(rest) != 2 || rest[0] != "u" {
		t.Errorf("--light twice, then a command: %v %v %v", rest, want, err)
	}
	rest, want, err = ParseFlags([]string{"--dark"})
	if err != nil || want == nil || !*want || rest != nil {
		t.Errorf("--dark alone: %v %v %v", rest, want, err)
	}
	if _, _, err := ParseFlags([]string{"--dark", "--light"}); err == nil {
		t.Error("--dark --light was not a contradiction")
	}
}
