package usertarget

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func newTestCmd() *cobra.Command {
	return &cobra.Command{Use: "test"}
}

func TestAddLookupFlags(t *testing.T) {
	cmd := newTestCmd()
	var flags LookupFlags

	AddLookupFlags(cmd, &flags)

	for _, name := range []string{"email", "user-id", "username", "external-id"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Fatalf("missing --%s flag", name)
		}
	}
	if !cmd.Flags().Lookup("external-id").Hidden {
		t.Fatal("expected --external-id to be hidden")
	}

	for flag, value := range map[string]string{
		"email":       "seller@example.com",
		"user-id":     "2245593582708",
		"username":    "sellerone",
		"external-id": "legacy-id",
	} {
		if err := cmd.Flags().Set(flag, value); err != nil {
			t.Fatalf("set --%s: %v", flag, err)
		}
	}

	if flags.Email != "seller@example.com" || flags.UserID != "2245593582708" || flags.Username != "sellerone" || flags.ExternalIDAlias != "legacy-id" {
		t.Fatalf("flags not wired correctly: %+v", flags)
	}
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

func TestResolveLookupTargetUsesExternalIDAlias(t *testing.T) {
	cmd := newTestCmd()
	flags := LookupFlags{ExternalIDAlias: "2245593582708"}

	target, err := ResolveLookupTarget(cmd, flags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.UserID != "2245593582708" {
		t.Fatalf("got user_id %q, want 2245593582708", target.UserID)
	}
}

func TestResolveLookupTargetRejectsConflictingUserIDAliases(t *testing.T) {
	cmd := newTestCmd()
	flags := LookupFlags{UserID: "2245593582708", ExternalIDAlias: "other-id"}

	_, err := ResolveLookupTarget(cmd, flags)
	if err == nil || !strings.Contains(err.Error(), "--user-id and --external-id must match") {
		t.Fatalf("expected conflicting alias error, got %v", err)
	}
}

func TestResolveLookupTargetRequiresOne(t *testing.T) {
	cmd := newTestCmd()

	_, err := ResolveLookupTarget(cmd, LookupFlags{})
	if err == nil {
		t.Fatal("expected error when no lookup flag is provided")
	}
}

func TestAddMutationFlags(t *testing.T) {
	cmd := newTestCmd()
	var flags MutationFlags

	AddMutationFlags(cmd, &flags)

	for _, name := range []string{"user-id", "expected-email", "external-id", "email"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Fatalf("missing --%s flag", name)
		}
	}
	for _, name := range []string{"external-id", "email"} {
		if !cmd.Flags().Lookup(name).Hidden {
			t.Fatalf("expected --%s to be hidden", name)
		}
	}

	for flag, value := range map[string]string{
		"user-id":        "2245593582708",
		"expected-email": "seller@example.com",
		"external-id":    "legacy-id",
		"email":          "legacy@example.com",
	} {
		if err := cmd.Flags().Set(flag, value); err != nil {
			t.Fatalf("set --%s: %v", flag, err)
		}
	}

	if flags.UserID != "2245593582708" || flags.ExpectedEmail != "seller@example.com" || flags.ExternalIDAlias != "legacy-id" || flags.ExpectedEmailAlias != "legacy@example.com" {
		t.Fatalf("flags not wired correctly: %+v", flags)
	}
}

func TestResolveMutationTarget(t *testing.T) {
	tests := []struct {
		name              string
		flags             MutationFlags
		wantUserID        string
		wantExpectedEmail string
	}{
		{
			name:              "direct flags",
			flags:             MutationFlags{UserID: "2245593582708", ExpectedEmail: "seller@example.com"},
			wantUserID:        "2245593582708",
			wantExpectedEmail: "seller@example.com",
		},
		{
			name:              "hidden aliases",
			flags:             MutationFlags{ExternalIDAlias: "2245593582708", ExpectedEmailAlias: "seller@example.com"},
			wantUserID:        "2245593582708",
			wantExpectedEmail: "seller@example.com",
		},
		{
			name:       "expected email omitted",
			flags:      MutationFlags{UserID: "2245593582708"},
			wantUserID: "2245593582708",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, err := ResolveMutationTarget(newTestCmd(), tt.flags)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if target.UserID != tt.wantUserID {
				t.Fatalf("got user_id %q, want %q", target.UserID, tt.wantUserID)
			}
			if target.ExpectedEmail != tt.wantExpectedEmail {
				t.Fatalf("got expected_email %q, want %q", target.ExpectedEmail, tt.wantExpectedEmail)
			}
			if target.Identifier() != tt.wantUserID {
				t.Fatalf("got identifier %q, want %q", target.Identifier(), tt.wantUserID)
			}
			if target.Subject() != "user_id "+tt.wantUserID {
				t.Fatalf("got subject %q, want user_id %s", target.Subject(), tt.wantUserID)
			}

			params := MutationParams(target)
			if params.Get("user_id") != tt.wantUserID {
				t.Fatalf("got user_id param %q, want %q", params.Get("user_id"), tt.wantUserID)
			}
			if params.Get("expected_email") != tt.wantExpectedEmail {
				t.Fatalf("got expected_email param %q, want %q", params.Get("expected_email"), tt.wantExpectedEmail)
			}
		})
	}
}

func TestResolveMutationTargetRequiresUserID(t *testing.T) {
	_, err := ResolveMutationTarget(newTestCmd(), MutationFlags{})
	if err == nil || !strings.Contains(err.Error(), "missing required flag: --user-id") {
		t.Fatalf("expected missing user-id error, got %v", err)
	}
}

func TestResolveMutationTargetRejectsConflictingUserIDAliases(t *testing.T) {
	flags := MutationFlags{UserID: "2245593582708", ExternalIDAlias: "other-id"}

	_, err := ResolveMutationTarget(newTestCmd(), flags)
	if err == nil || !strings.Contains(err.Error(), "--user-id and --external-id must match") {
		t.Fatalf("expected conflicting user-id alias error, got %v", err)
	}
}

func TestResolveMutationTargetRejectsConflictingExpectedEmailAliases(t *testing.T) {
	flags := MutationFlags{
		UserID:             "2245593582708",
		ExpectedEmail:      "seller@example.com",
		ExpectedEmailAlias: "other@example.com",
	}

	_, err := ResolveMutationTarget(newTestCmd(), flags)
	if err == nil || !strings.Contains(err.Error(), "--expected-email and --email must match") {
		t.Fatalf("expected conflicting expected-email alias error, got %v", err)
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

func TestFallback(t *testing.T) {
	if got := Fallback("", "alt"); got != "alt" {
		t.Errorf("empty value: got %q, want alt", got)
	}
	if got := Fallback("value", "alt"); got != "value" {
		t.Errorf("present value: got %q, want value", got)
	}
}
