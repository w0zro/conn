package main

import "testing"

// What git describe says, in each of its forms.
func TestDescribeIsParsed(t *testing.T) {
	for _, c := range []struct {
		in   string
		want described
		ok   bool
	}{
		{"v0.7.0", described{tag: "0.7.0"}, true},
		{"v0.7.0-dirty", described{tag: "0.7.0", dirty: true}, true},
		{"v0.7.0-59-g0c69871", described{tag: "0.7.0", ahead: 59, commit: "0c69871"}, true},
		{"v0.7.0-59-g0c6987146b46-dirty", described{tag: "0.7.0", ahead: 59, commit: "0c69871", dirty: true}, true},
		{"0c69871", described{ahead: 1, commit: "0c69871"}, true},
		{"0c69871-dirty", described{ahead: 1, commit: "0c69871", dirty: true}, true},
		{"", described{}, false},
		{"garbage", described{}, false},
	} {
		got, ok := parseDescribe(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("%q: %+v %v, want %+v %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

// What the module system says of the main module, in each of its forms.
// A pseudo-version's base is the tag below it, so a build past v0.7.0
// says 0.7.0, never 0.7.1.
func TestModuleVersionIsParsed(t *testing.T) {
	for _, c := range []struct {
		in   string
		want described
	}{
		{"v0.7.0", described{tag: "0.7.0"}},
		{"v0.7.1-0.20260909025524-0c6987146b46", described{tag: "0.7.0", ahead: 1, commit: "0c69871"}},
		{"v0.7.1-0.20260909025524-0c6987146b46+dirty", described{tag: "0.7.0", ahead: 1, commit: "0c69871", dirty: true}},
		{"v0.0.0-20260909025524-0c6987146b46", described{ahead: 1, commit: "0c69871"}},
		{"(devel)", described{ahead: 1}},
		{"", described{ahead: 1}},
	} {
		if got := parseModuleVersion(c.in); got != c.want {
			t.Errorf("%q: %+v, want %+v", c.in, got, c.want)
		}
	}
}

// The build as this test binary knows it hangs together, whatever way
// it was built.
func TestBuildIsRead(t *testing.T) {
	b := readBuild()
	if b.exact && (b.tag == "" || b.modified) {
		t.Errorf("an exact build with no tag or with changes: %+v", b)
	}
	if b.commit != "" && len(b.commit) != 7 {
		t.Errorf("commit is not short: %+v", b)
	}
}
