package products

import (
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestPageDevRejectsStdinPath(t *testing.T) {
	cmd := testutil.Command(newPageDevCmd())
	cmd.SetArgs([]string{"prod1", "-"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected stdin path error")
	}
	if !strings.Contains(err.Error(), "page dev needs a file path") {
		t.Fatalf("unexpected error: %v", err)
	}
}
