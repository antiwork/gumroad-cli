package usertarget

import (
	"testing"

	"github.com/spf13/cobra"
)

func newTestCmd() *cobra.Command {
	return &cobra.Command{Use: "test"}
}

func TestResolveLookupTargetByUsername(t *testing.T) {
	cmd := newTestCmd()
	flags := LookupFlags{Username: "sellerone"}

	target, err := ResolveLookupTarget(cmd, flags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.Username != "sellerone" {
		t.Fatalf("got username %q, want sellerone", target.Username)
	}
	if got := target.Values().Get("username"); got != "sellerone" {
		t.Fatalf("got username param %q, want sellerone", got)
	}
	if target.Identifier() != "sellerone" {
		t.Fatalf("got identifier %q, want sellerone", target.Identifier())
	}
}

func TestResolveLookupTargetPrecedence(t *testing.T) {
	cmd := newTestCmd()
	flags := LookupFlags{Email: "seller@example.com", UserID: "2245593582708", Username: "sellerone"}

	target, err := ResolveLookupTarget(cmd, flags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	values := target.Values()
	if values.Get("email") != "seller@example.com" {
		t.Errorf("missing email param: %v", values)
	}
	if values.Get("user_id") != "2245593582708" {
		t.Errorf("missing user_id param: %v", values)
	}
	if values.Get("username") != "sellerone" {
		t.Errorf("missing username param: %v", values)
	}
	// Identifier prefers user_id over email/username.
	if target.Identifier() != "2245593582708" {
		t.Errorf("got identifier %q, want 2245593582708", target.Identifier())
	}
}

func TestResolveLookupTargetRequiresOne(t *testing.T) {
	cmd := newTestCmd()

	_, err := ResolveLookupTarget(cmd, LookupFlags{})
	if err == nil {
		t.Fatal("expected error when no lookup flag is provided")
	}
}

func TestUserIdentifierFallback(t *testing.T) {
	if got := UserIdentifier("seller@example.com", "", ""); got != "seller@example.com" {
		t.Errorf("email fallback: got %q", got)
	}
	if got := UserIdentifier("", "", "sellerone"); got != "sellerone" {
		t.Errorf("username fallback: got %q", got)
	}
	if got := UserIdentifier("seller@example.com", "uid", "sellerone"); got != "uid" {
		t.Errorf("user_id precedence: got %q", got)
	}
}
