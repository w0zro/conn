package main

import (
	"regexp"
	"runtime/debug"
	"strings"
	"time"

	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

// A build is where this binary came from: the release it is on or past,
// whether it is exactly that release, and the commit it was built from.
// The version row says the release; the build row says the commit.
type build struct {
	tag      string    // the release: 0.7.0; blank when no tag is known
	exact    bool      // built at the tag, unmodified
	commit   string    // the commit, short
	time     time.Time // when the commit was made
	modified bool      // the tree had changes past the commit
}

// version is stamped by the release build, with what git describe says.
// A build that came another way answers from the module system, which
// records the commit and, past Go 1.24, a version derived from the tag.
var version string

// readBuild is the build, from the stamp and the module system together.
func readBuild() build {
	var b build
	info, ok := debug.ReadBuildInfo()
	if ok {
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				b.commit = shortCommit(s.Value)
			case "vcs.time":
				b.time, _ = time.Parse(time.RFC3339, s.Value)
			case "vcs.modified":
				b.modified = s.Value == "true"
			}
		}
	}
	d, described := parseDescribe(version)
	if !described && ok {
		d = parseModuleVersion(info.Main.Version)
	}
	b.tag = d.tag
	b.modified = b.modified || d.dirty
	b.exact = d.tag != "" && d.ahead == 0 && !b.modified
	if b.commit == "" {
		b.commit = d.commit
	}
	return b
}

// A described version: the tag, how many commits past it, the commit,
// and whether the tree was dirty.
type described struct {
	tag    string
	ahead  int
	commit string
	dirty  bool
}

var (
	// v0.7.0, v0.7.0-59-g0c69871, v0.7.0-dirty, v0.7.0-59-g0c69871-dirty
	describeTagged = regexp.MustCompile(`^v?(\d+\.\d+\.\d+)(?:-(\d+)-g([0-9a-f]+))?(-dirty)?$`)
	// 0c69871, 0c69871-dirty: git describe --always with no tag to reach
	describeBare = regexp.MustCompile(`^([0-9a-f]{7,40})(-dirty)?$`)
)

// parseDescribe reads what git describe --tags --always --dirty says.
func parseDescribe(s string) (described, bool) {
	if m := describeTagged.FindStringSubmatch(s); m != nil {
		d := described{tag: m[1], commit: shortCommit(m[3]), dirty: m[4] != ""}
		for _, c := range m[2] {
			d.ahead = d.ahead*10 + int(c-'0')
		}
		return d, true
	}
	if m := describeBare.FindStringSubmatch(s); m != nil {
		return described{ahead: 1, commit: shortCommit(m[1]), dirty: m[2] != ""}, true
	}
	return described{}, false
}

// parseModuleVersion reads the module system's version for the main
// module: a tag, v0.7.0; a pseudo-version past one, whose base is the
// tag below it, v0.7.1-0.20260909025524-0c6987146b46, with +dirty when
// the tree had changes; or (devel), which says nothing.
func parseModuleVersion(v string) described {
	var d described
	if v == "" || v == "(devel)" {
		return described{ahead: 1}
	}
	d.dirty = semver.Build(v) == "+dirty"
	v = strings.TrimSuffix(v, "+dirty")
	if module.IsPseudoVersion(v) {
		d.ahead = 1
		d.commit, _ = module.PseudoVersionRev(v)
		d.commit = shortCommit(d.commit)
		base, _ := module.PseudoVersionBase(v)
		d.tag = strings.TrimPrefix(base, "v")
		return d
	}
	d.tag = strings.TrimPrefix(v, "v")
	return d
}

// shortCommit is the first seven characters of a commit.
func shortCommit(c string) string {
	if len(c) > 7 {
		return c[:7]
	}
	return c
}
