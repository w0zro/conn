package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestMain lets the test binary stand in for a process to inspect: run
// with the probe variable set, it sleeps instead of testing, a child
// whose environment and binary are known. A platform binary would not
// do: macOS keeps the environment of its own to itself.
func TestMain(m *testing.M) {
	if os.Getenv("CONN_PROBE") == "sleep" {
		time.Sleep(10 * time.Second)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// probe starts the test binary as a process to inspect, with an
// environment of its own, and returns its pid.
func probe(t *testing.T, env ...string) int {
	t.Helper()
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(env, "CONN_PROBE=sleep")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	return cmd.Process.Pid
}

func TestTheBinaryAndEnvironmentOfAProcessAreRead(t *testing.T) {
	pid := probe(t, "NODE_ENV=production", "DATABASE_URL=postgres://app:hunter2@db:5432/app", "API_TOKEN=abc", "PATH=/usr/bin")

	exe, err := filepath.EvalSymlinks(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := filepath.EvalSymlinks(binaryOf(pid)); got != exe {
		t.Errorf("binaryOf = %q, want the test binary %q", got, exe)
	}

	// The environment takes a moment to be readable after the start.
	var env []string
	for range 50 {
		if env = environOf(pid); slices.Contains(env, "NODE_ENV=production") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !slices.Contains(env, "NODE_ENV=production") {
		t.Fatalf("environOf = %v, want the child's environment", env)
	}
	got := tellingEnv(env)
	want := []string{"DATABASE_URL=postgres://app:•••@db:5432/app", "NODE_ENV=production"}
	if !slices.Equal(got, want) {
		t.Errorf("tellingEnv = %v, want %v: the telling ones, the token out, the password masked", got, want)
	}
}

func TestTheEnvironmentIsReadOutOfPsE(t *testing.T) {
	// ps -E prints the command line and then the environment, a space
	// between everything; a value with a space in it runs on until the
	// next name.
	argv := "node server.js --name a=b"
	both := argv + " HOME=/Users/me NODE_OPTIONS=--max-old-space-size=4096 GREETING=hello there PORT=3000"
	got := parseEnviron(both, argv)
	want := []string{"HOME=/Users/me", "NODE_OPTIONS=--max-old-space-size=4096", "GREETING=hello there", "PORT=3000"}
	if !slices.Equal(got, want) {
		t.Errorf("parseEnviron = %v, want %v", got, want)
	}
	if got := parseEnviron(argv, argv); got != nil {
		t.Errorf("no environment printed = %v, want nothing", got)
	}
}

func TestTellingEnvPicksWhatSaysWhereAndWhat(t *testing.T) {
	env := []string{
		"RAILS_ENV=production", "PORT=3000", "HOST=0.0.0.0", "REDIS_URL=redis://cache:6379/1",
		"SMTP_HOST=mail", "RUST_LOG=debug", "DJANGO_SETTINGS_MODULE=app.settings",
		"PATH=/usr/bin", "HOME=/Users/me", "SHELL=/bin/zsh", "TERM=xterm", "LANG=en_US.UTF-8",
		"SECRET_KEY_BASE=deadbeef", "AWS_ACCESS_KEY_ID=AKIA", "GITHUB_TOKEN=ghp", "DB_PASSWORD=pw", "AUTH_URL=https://a",
		"MONGO_URI=mongodb://u:p@m/db", "broken",
	}
	got := tellingEnv(env)
	want := []string{
		"DJANGO_SETTINGS_MODULE=app.settings", "HOST=0.0.0.0", "MONGO_URI=mongodb://u:•••@m/db", "PORT=3000",
		"RAILS_ENV=production", "REDIS_URL=redis://cache:6379/1", "RUST_LOG=debug", "SMTP_HOST=mail",
	}
	if !slices.Equal(got, want) {
		t.Errorf("tellingEnv = %v, want %v: sorted, secrets out, the password masked, at most %d", got, want, envShown)
	}
}

func TestMaskURLKeepsTheRest(t *testing.T) {
	for in, want := range map[string]string{
		"postgres://app:hunter2@db/app":      "postgres://app:•••@db/app",
		"postgres://db/app":                  "postgres://db/app",
		"redis://:pw@cache:6379":             "redis://:•••@cache:6379",
		"https://user@host/x, amqp://a:b@q/": "https://user@host/x, amqp://a:•••@q/",
		"plain":                              "plain",
	} {
		if got := maskURL(in); got != want {
			t.Errorf("maskURL(%q) = %q, want %q", in, got, want)
		}
	}
	home, _ := os.UserHomeDir()
	if got := homely(filepath.Join(home, ".nvm", "bin", "node")); !strings.HasPrefix(got, "~/") {
		t.Errorf("homely = %q, want the home as ~", got)
	}
	if got := homely("/usr/bin/node"); got != "/usr/bin/node" {
		t.Errorf("homely = %q, want a path outside home as it is", got)
	}
}
