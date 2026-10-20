package main

import (
	"errors"
	"strings"
	"testing"
)

// conn answers --version with the build and help with the uses, and
// says what was wrong with a line it could not read, which main shows
// the uses under and exits 2 on.
func TestRunAnswersVersionHelpAndAWrongLine(t *testing.T) {
	var out strings.Builder
	if err := run(&out, []string{"--version"}); err != nil || !strings.HasPrefix(out.String(), "conn") {
		t.Errorf("--version: %q, %v", out.String(), err)
	}
	for _, help := range []string{"help", "-h", "--help"} {
		out.Reset()
		if err := run(&out, []string{help}); err != nil || !strings.HasPrefix(out.String(), "usage: conn") || !strings.Contains(out.String(), "conn u [NAME] [-w] -- COMMAND...") {
			t.Errorf("%s: %q, %v", help, out.String(), err)
		}
	}
	var wrong usageError
	for _, line := range [][]string{{"frob"}, {"--theme"}, {"--dark", "--light"}} {
		out.Reset()
		err := run(&out, line)
		if !errors.As(err, &wrong) || wrong.wrong == "" || out.Len() != 0 {
			t.Errorf("%v: %v, wrote %q", line, err, out.String())
		}
	}
	if err := run(&out, []string{"frob"}); err == nil || err.Error() != "no verb frob" {
		t.Errorf("a verb conn has not: %v", err)
	}
}

// The build is said as a release when it is exactly one, and with the
// commit it was built from when it is not.
func TestVersionLine(t *testing.T) {
	if got := versionLine("v0.25.0"); got != "conn 0.25.0" {
		t.Errorf("an exact release: %q", got)
	}
	if got := versionLine("v0.25.0-3-g4cdb20d-dirty"); !strings.HasPrefix(got, "conn 0.25.0 (devel)") || !strings.Contains(got, "4cdb20d") || !strings.HasSuffix(got, "modified") {
		t.Errorf("a build past the release: %q", got)
	}
}
