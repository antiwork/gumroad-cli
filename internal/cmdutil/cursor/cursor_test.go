package cursor

import (
	"net/url"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestAddFlagsRegistersCursorAndLimit(t *testing.T) {
	var flags Flags
	cmd := &cobra.Command{Use: "test"}

	AddFlags(cmd, &flags)
	cmd.SetArgs([]string{"--cursor", "cur-1", "--limit", "25"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if flags.Cursor != "cur-1" {
		t.Fatalf("Cursor = %q, want cur-1", flags.Cursor)
	}
	if flags.Limit != 25 {
		t.Fatalf("Limit = %d, want 25", flags.Limit)
	}
}

func TestApplySetsNonZeroValues(t *testing.T) {
	params := url.Values{}

	Apply(params, Flags{Cursor: "cur-1", Limit: 25})

	if got := params.Get("cursor"); got != "cur-1" {
		t.Fatalf("cursor = %q, want cur-1", got)
	}
	if got := params.Get("limit"); got != "25" {
		t.Fatalf("limit = %q, want 25", got)
	}
}

func TestApplyOmitsZeroValues(t *testing.T) {
	params := url.Values{}

	Apply(params, Flags{})

	if got := params.Encode(); got != "" {
		t.Fatalf("expected empty params, got %q", got)
	}
}

func TestWriteMoreFooter(t *testing.T) {
	var b strings.Builder

	if err := WriteMoreFooter(&b, Pagination{Next: "cur-next"}); err != nil {
		t.Fatalf("WriteMoreFooter() error = %v", err)
	}
	if got, want := b.String(), "\nMore results: --cursor cur-next\n"; got != want {
		t.Fatalf("footer = %q, want %q", got, want)
	}
}

func TestWriteMoreFooterSkipsEmptyNext(t *testing.T) {
	var b strings.Builder

	if err := WriteMoreFooter(&b, Pagination{}); err != nil {
		t.Fatalf("WriteMoreFooter() error = %v", err)
	}
	if got := b.String(); got != "" {
		t.Fatalf("footer = %q, want empty string", got)
	}
}
