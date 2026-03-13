package completion

import (
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
	"github.com/spf13/cobra"
)

func rootWithCompletion() *cobra.Command {
	root := &cobra.Command{Use: "gumroad"}
	root.AddCommand(NewCompletionCmd())
	return root
}

func TestCompletion_Bash(t *testing.T) {
	root := rootWithCompletion()
	root.SetArgs([]string{"completion", "bash"})
	var err error
	out := testutil.CaptureStdout(func() { err = root.Execute() })
	if err != nil {
		t.Fatalf("completion bash failed: %v", err)
	}

	if !strings.Contains(out, "bash") && !strings.Contains(out, "complete") {
		t.Errorf("expected bash completion script, got: %q", out[:min(len(out), 200)])
	}
}

func TestCompletion_Zsh(t *testing.T) {
	root := rootWithCompletion()
	root.SetArgs([]string{"completion", "zsh"})
	var err error
	out := testutil.CaptureStdout(func() { err = root.Execute() })
	if err != nil {
		t.Fatalf("completion zsh failed: %v", err)
	}

	if !strings.Contains(out, "zsh") && !strings.Contains(out, "compdef") {
		t.Errorf("expected zsh completion script, got: %q", out[:min(len(out), 200)])
	}
}

func TestCompletion_Fish(t *testing.T) {
	root := rootWithCompletion()
	root.SetArgs([]string{"completion", "fish"})
	var err error
	out := testutil.CaptureStdout(func() { err = root.Execute() })
	if err != nil {
		t.Fatalf("completion fish failed: %v", err)
	}

	if !strings.Contains(out, "fish") && !strings.Contains(out, "complete") {
		t.Errorf("expected fish completion script, got: %q", out[:min(len(out), 200)])
	}
}

func TestCompletion_Powershell(t *testing.T) {
	root := rootWithCompletion()
	root.SetArgs([]string{"completion", "powershell"})
	var err error
	out := testutil.CaptureStdout(func() { err = root.Execute() })
	if err != nil {
		t.Fatalf("completion powershell failed: %v", err)
	}

	if out == "" {
		t.Error("expected powershell completion output, got empty")
	}
}

func TestCompletion_NoArgs(t *testing.T) {
	root := rootWithCompletion()
	root.SetArgs([]string{"completion"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error without shell arg")
	}
	for _, want := range []string{
		"missing required argument: <bash|zsh|fish|powershell>",
		"Usage:",
		"completion <bash|zsh|fish|powershell>",
		"Examples:",
		"Run \"gumroad completion --help\" for more information.",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing %q in %q", want, err.Error())
		}
	}
}

func TestCompletion_InvalidShell(t *testing.T) {
	root := rootWithCompletion()
	root.SetArgs([]string{"completion", "tcsh"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for invalid shell")
	}
	for _, want := range []string{
		"invalid shell: tcsh",
		"Usage:",
		"completion <bash|zsh|fish|powershell>",
		"Examples:",
		"Run \"gumroad completion --help\" for more information.",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing %q in %q", want, err.Error())
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
