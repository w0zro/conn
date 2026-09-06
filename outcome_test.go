package main

import (
	"strings"
	"testing"
)

func TestATranscriptsEndIsReadByItsShape(t *testing.T) {
	// The tools say it in a line at the end; conn reads that line the way
	// a reader would, whichever tool printed it, and says nothing for a
	// shape it does not know.
	cases := map[string]string{
		// go test, counted a failure at a time; a clean run says nothing
		"=== RUN   TestA\n--- FAIL: TestA (0.00s)\n    a_test.go:9: nope\n--- FAIL: TestB (0.00s)\nFAIL\nFAIL\tx\t0.3s\n": "2 failed",
		"ok  \tx\t0.3s\n": "",
		"# x\n./a.go:12:3: undefined: foo\n./b.go:4:1: missing return\nFAIL\tx [build failed]\n": "2 errors",
		"FAIL\tx [build failed]\n": "build failed",
		// go vet, and a compiler
		"./a.go:12:3: unreachable code\n": "1 error",
		// pytest
		"a.py F..\n=========== 1 failed, 2 passed in 0.12s ===========\n": "1 failed",
		"=========== 12 passed in 0.12s ===========\n":                    "12 passed",
		// jest and vitest
		"Tests:       2 failed, 10 passed, 12 total\nTime:        1.2 s\n": "2 failed",
		" Test Files  1 passed (1)\n      Tests  10 passed (10)\n":         "10 passed",
		"      Tests  2 failed | 10 passed (12)\n":                         "2 failed",
		// mocha, passing then failing
		"  10 passing (40ms)\n  2 failing\n\n  1) a\n": "2 failed",
		"  10 passing (40ms)\n":                        "10 passed",
		// cargo test, rspec, ExUnit
		"test result: FAILED. 10 passed; 2 failed; 0 ignored; 0 measured\n": "2 failed",
		"test result: ok. 10 passed; 0 failed; 0 ignored\n":                 "10 passed",
		"Finished in 0.5 seconds\n12 examples, 2 failures\n":                "2 failed",
		"12 examples, 0 failures\n":                                         "12 passed",
		"3 doctests, 12 tests, 1 failure\n":                                 "1 failed",
		// linters and compilers with a summary line
		"a.go:1:1: x (revive)\n5 issues.\n":                       "5 issues",
		"0 issues.\n":                                             "",
		"Found 3 errors in 2 files.\n":                            "3 errors",
		"All checks passed!\n":                                    "",
		"✖ 3 problems (2 errors, 1 warning)\n":                    "3 problems",
		"error: could not compile `x` due to 3 previous errors\n": "3 errors",
		"error: could not compile `x` due to 1 previous error\n":  "1 error",
		// nothing conn knows the shape of
		"make: *** [test] Error 1\nsomething else\n": "",
	}
	for out, want := range cases {
		lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
		if got := summarize(lines); got != want {
			t.Errorf("summarize(%q) = %q, want %q", out, got, want)
		}
	}
	// The shell's colors do not hide the shape, and the prompt after the
	// run does not stand in for it.
	colored := []string{"\x1b[31m--- FAIL: TestA (0.00s)\x1b[0m", "FAIL", "~/p/x main", "$"}
	if got := summarize(colored); got != "1 failed" {
		t.Errorf("summarize(colored) = %q, want the failure counted through the color", got)
	}
}
