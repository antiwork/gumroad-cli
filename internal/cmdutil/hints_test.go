package cmdutil

import (
	"strings"
	"testing"
)

func withHintEnv(t *testing.T, goos string, env map[string]string) {
	t.Helper()

	origGOOS := hintGOOS
	origGetenv := hintGetenv
	t.Cleanup(func() {
		hintGOOS = origGOOS
		hintGetenv = origGetenv
	})

	hintGOOS = goos
	hintGetenv = func(key string) string {
		return env[key]
	}
}

func TestReplayCommand_QuotesShellSensitiveValues_POSIX(t *testing.T) {
	withHintEnv(t, "darwin", map[string]string{})

	got := ReplayCommand("gumroad sales list",
		CommandArg{Flag: "--email", Value: "user name@example.com"},
		CommandArg{Flag: "--page-key", Value: "$NEXT_PAGE"},
	)

	for _, want := range []string{
		"--email 'user name@example.com'",
		"--page-key '$NEXT_PAGE'",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
}

func TestReplayCommand_EscapesSingleQuotes_POSIX(t *testing.T) {
	withHintEnv(t, "linux", map[string]string{})

	got := ReplayCommand("gumroad products list", CommandArg{Flag: "--name", Value: "O'Reilly"})
	want := "--name 'O'\"'\"'Reilly'"
	if !strings.Contains(got, want) {
		t.Fatalf("missing %q in %q", want, got)
	}
}

func TestReplayCommand_UsesPowerShellQuotingWhenDetected(t *testing.T) {
	withHintEnv(t, "windows", map[string]string{"PSModulePath": "C:\\Program Files\\PowerShell"})

	got := ReplayCommand("gumroad sales list",
		CommandArg{Flag: "--email", Value: "user name@example.com"},
		CommandArg{Flag: "--page-key", Value: "$NEXT_PAGE"},
	)

	for _, want := range []string{
		"--email 'user name@example.com'",
		"--page-key '$NEXT_PAGE'",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
}

func TestReplayCommand_UsesCmdQuotingAsWindowsFallback(t *testing.T) {
	withHintEnv(t, "windows", map[string]string{})

	got := ReplayCommand("gumroad sales list",
		CommandArg{Flag: "--email", Value: "user name@example.com"},
		CommandArg{Flag: "--name", Value: `He said "hi"`},
	)

	for _, want := range []string{
		`--email "user name@example.com"`,
		`--name "He said ""hi"""`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
}

func TestReplayCommand_PrefersPOSIXOnWindowsWhenShellIsPresent(t *testing.T) {
	withHintEnv(t, "windows", map[string]string{"SHELL": "/usr/bin/bash"})

	got := ReplayCommand("gumroad sales list", CommandArg{Flag: "--page-key", Value: "$NEXT_PAGE"})
	want := "--page-key '$NEXT_PAGE'"
	if !strings.Contains(got, want) {
		t.Fatalf("missing %q in %q", want, got)
	}
}
