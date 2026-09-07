package main

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// How a task ended is an exit status; what the task said is in its
// transcript, and the tools say it in a line at the end that a reader
// looks for: 3 failed, 12 passed, 5 issues. conn reads that line by its
// shape rather than by the tool that printed it, so a Makefile target that
// wraps the tool reads the same as the tool run bare. When no line has a
// shape conn knows, the exit status stands alone.

// exitWord is what an exit status says beyond its number, for the ones
// that say anything: a shell reports a command a signal ended as 128 and
// the signal, and 137 read as killed, 143 as terminated and 130 as
// interrupted is the difference between a crash, a kill and a ctrl-c. 127
// and 126 are the shell's own: the command was not found, or could not be
// run. An ordinary failure is its number alone.
func exitWord(state string) string {
	n, err := strconv.Atoi(state)
	if err != nil {
		return ""
	}
	switch {
	case n == 126:
		return "not executable"
	case n == 127:
		return "not found"
	case n > 128 && n < 160:
		return signalWord(n - 128)
	}
	return ""
}

// signalWord names a signal by what it did, in the words a reader uses:
// killed, not SIGKILL.
func signalWord(sig int) string {
	switch sig {
	case 1:
		return "hung up"
	case 2:
		return "interrupted"
	case 3:
		return "quit"
	case 6:
		return "aborted"
	case 8:
		return "floating point error"
	case 9:
		return "killed"
	case 11:
		return "segfault"
	case 13:
		return "broken pipe"
	case 14:
		return "alarm"
	case 15:
		return "terminated"
	}
	if sig == 7 || sig == 10 {
		return "bus error"
	}
	return "signal " + strconv.Itoa(sig)
}

// summarize is what a transcript's end says of the run, in a few words,
// or nothing when no line of it has a shape conn knows. The last line
// that has one speaks: a tool's summary comes last, after the details.
func summarize(lines []string) string {
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(ansi.Strip(lines[i]))
		for _, s := range summaryShapes {
			if m := s.shape.FindStringSubmatch(line); m != nil {
				return s.say(m)
			}
		}
	}
	// Tools with no summary line say it one failure at a time, and the
	// failures are counted: go test's --- FAIL lines, and a compiler's or
	// vet's file:line:col errors.
	if n := countLines(lines, goTestFail); n > 0 {
		return counted(n, "failed", "failed")
	}
	if n := countLines(lines, goError); n > 0 {
		return counted(n, "error", "errors")
	}
	if countLines(lines, goBuildFailed) > 0 {
		return "build failed"
	}
	return ""
}

// summaryShape is one tool's summary line, and what to say for it.
type summaryShape struct {
	shape *regexp.Regexp
	say   func(m []string) string
}

// failedOrPassed says failures when there were any, else what passed —
// and nothing for a clean run that counted nothing.
func failedOrPassed(failed, passed string) string {
	if f, _ := strconv.Atoi(failed); f > 0 {
		return counted(f, "failed", "failed")
	}
	if p, _ := strconv.Atoi(passed); p > 0 {
		return counted(p, "passed", "passed")
	}
	return ""
}

// counted is a count and its noun, singular or plural; 0 is nothing to say.
func counted(n int, one, many string) string {
	switch n {
	case 0:
		return ""
	case 1:
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

var summaryShapes = []summaryShape{
	// pytest: === 2 failed, 10 passed in 1.23s ===
	{regexp.MustCompile(`^=+ (.*) in [\d.]+s`), func(m []string) string {
		return failedOrPassed(find(m[1], `(\d+) failed`), find(m[1], `(\d+) passed`))
	}},
	// jest: Tests:  2 failed, 10 passed, 12 total
	{regexp.MustCompile(`^Tests:\s+(.*), \d+ total$`), func(m []string) string {
		return failedOrPassed(find(m[1], `(\d+) failed`), find(m[1], `(\d+) passed`))
	}},
	// vitest: Tests  2 failed | 10 passed (12)
	{regexp.MustCompile(`^Tests\s+(.*)\(\d+\)$`), func(m []string) string {
		return failedOrPassed(find(m[1], `(\d+) failed`), find(m[1], `(\d+) passed`))
	}},
	// mocha: 2 failing / 10 passing, each on its own line, failing last
	{regexp.MustCompile(`^(\d+) failing$`), func(m []string) string { return failedOrPassed(m[1], "") }},
	{regexp.MustCompile(`^(\d+) passing`), func(m []string) string { return failedOrPassed("", m[1]) }},
	// cargo test: test result: FAILED. 10 passed; 2 failed; 0 ignored; ...
	{regexp.MustCompile(`^test result: \w+\. (\d+) passed; (\d+) failed`), func(m []string) string {
		return failedOrPassed(m[2], m[1])
	}},
	// rspec: 12 examples, 2 failures — and ExUnit: 3 doctests, 12 tests,
	// 2 failures
	{regexp.MustCompile(`(\d+) (?:examples?|tests?), (\d+) failures?`), func(m []string) string {
		return failedOrPassed(m[2], m[1])
	}},
	// golangci-lint: 5 issues.
	{regexp.MustCompile(`^(\d+) issues?\.$`), func(m []string) string {
		n, _ := strconv.Atoi(m[1])
		return counted(n, "issue", "issues")
	}},
	// tsc and ruff: Found 3 errors.
	{regexp.MustCompile(`^Found (\d+) errors?`), func(m []string) string {
		n, _ := strconv.Atoi(m[1])
		return counted(n, "error", "errors")
	}},
	// eslint: ✖ 3 problems (2 errors, 1 warning)
	{regexp.MustCompile(`^✖ (\d+) problems?`), func(m []string) string {
		n, _ := strconv.Atoi(m[1])
		return counted(n, "problem", "problems")
	}},
	// cargo: error: could not compile `x` due to 3 previous errors
	{regexp.MustCompile(`^error: could not compile .* due to (\d+) previous errors?`), func(m []string) string {
		n, _ := strconv.Atoi(m[1])
		return counted(n, "error", "errors")
	}},
	{regexp.MustCompile(`^error: could not compile .* due to 1 previous error`), func([]string) string { return "1 error" }},
}

var (
	goTestFail    = regexp.MustCompile(`^\s*--- FAIL: `)
	goError       = regexp.MustCompile(`^\S+\.go:\d+:\d+: `)
	goBuildFailed = regexp.MustCompile(`\[build failed\]`)
)

// find is the first group of a pattern in s, or nothing.
func find(s, pattern string) string {
	if m := regexp.MustCompile(pattern).FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

// countLines is how many lines match a shape.
func countLines(lines []string, shape *regexp.Regexp) int {
	n := 0
	for _, l := range lines {
		if shape.MatchString(ansi.Strip(l)) {
			n++
		}
	}
	return n
}
