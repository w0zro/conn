package shell

import "testing"

func TestAPathIsQuotedForTheShell(t *testing.T) {
	for in, want := range map[string]string{
		"/Users/w0zro/projects":        "'/Users/w0zro/projects'",
		"/Users/w0zro/SkellyLabs, Inc": "'/Users/w0zro/SkellyLabs, Inc'",
		"/tmp/it's":                    `'/tmp/it'\''s'`,
		"":                             "''",
	} {
		if got := Quote(in); got != want {
			t.Errorf("Quote(%q) = %s, want %s", in, got, want)
		}
	}
}
