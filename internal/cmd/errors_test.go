package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/prompt"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestPrintStructuredCommandError(t *testing.T) {
	var buf bytes.Buffer
	err := printStructuredCommandError(&buf, &api.APIError{StatusCode: 403, Message: "Access denied: scope"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload commandErrorEnvelope
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("expected JSON output, got %v with %q", err, buf.String())
	}
	if payload.Success {
		t.Fatal("expected success=false")
	}
	if payload.Error.Type != "api_error" || payload.Error.Code != "access_denied" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestPrintStructuredCommandError_WithHint(t *testing.T) {
	var buf bytes.Buffer
	err := printStructuredCommandError(&buf, &api.APIError{StatusCode: 401, Message: "Not authenticated.", Hint: api.HintRunAuthLogin})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload commandErrorEnvelope
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("expected JSON output, got %v with %q", err, buf.String())
	}
	if payload.Error.Hint != api.HintRunAuthLogin {
		t.Errorf("got hint %q, want %q", payload.Error.Hint, api.HintRunAuthLogin)
	}
}

func TestClassifyCommandError_APIWithHint(t *testing.T) {
	detail := classifyCommandError(&api.APIError{StatusCode: 404, Message: "Resource not found.", Hint: "Check the resource ID and try again."})
	if detail.Hint != "Check the resource ID and try again." {
		t.Errorf("got hint %q", detail.Hint)
	}
}

func TestClassifyCommandError_AuthHint(t *testing.T) {
	detail := classifyCommandError(config.ErrNotAuthenticated)
	if detail.Hint != api.HintRunAuthLogin {
		t.Errorf("got hint %q, want %q", detail.Hint, api.HintRunAuthLogin)
	}
}

func TestClassifyCommandError_WrappedAPIError(t *testing.T) {
	wrapped := fmt.Errorf("invalid token: %w", &api.APIError{StatusCode: 401, Message: "Not authenticated.", Hint: api.HintRunAuthLogin})
	detail := classifyCommandError(wrapped)
	if detail.Type != "api_error" || detail.Code != "not_authenticated" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
	if detail.Hint != api.HintRunAuthLogin {
		t.Errorf("got hint %q, want %q", detail.Hint, api.HintRunAuthLogin)
	}
}

func TestClassifyCommandError_WrappedConfigAuth(t *testing.T) {
	wrapped := fmt.Errorf("setup failed: %w", config.ErrNotAuthenticated)
	detail := classifyCommandError(wrapped)
	if detail.Type != "auth_error" || detail.Code != "not_authenticated" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
	if detail.Hint != api.HintRunAuthLogin {
		t.Errorf("got hint %q, want %q", detail.Hint, api.HintRunAuthLogin)
	}
}

func TestClassifyCommandError_ConfigAuthWithRemediationInMessage(t *testing.T) {
	// Simulates the real error from config.ResolveToken which already embeds remediation.
	wrapped := fmt.Errorf("%w. Run `gumroad auth login` first or set `GUMROAD_ACCESS_TOKEN`", config.ErrNotAuthenticated)
	detail := classifyCommandError(wrapped)
	if detail.Type != "auth_error" || detail.Code != "not_authenticated" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
	if detail.Hint != "" {
		t.Errorf("expected empty hint when message already contains remediation, got %q", detail.Hint)
	}
}

func TestClassifyCommandError_Nil(t *testing.T) {
	detail := classifyCommandError(nil)
	if detail.Type != "internal_error" || detail.Code != "unknown_error" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
}

func TestClassifyCommandError_Usage(t *testing.T) {
	cmd := &cobra.Command{Use: "gumroad user"}
	detail := classifyCommandError(cmdutil.UsageErrorf(cmd, "bad input"))
	if detail.Type != "usage_error" || detail.Code != "invalid_input" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
}

func TestClassifyCommandError_API(t *testing.T) {
	detail := classifyCommandError(&api.APIError{StatusCode: 429, Message: "Rate limited"})
	if detail.Type != "api_error" || detail.Code != "rate_limited" || detail.StatusCode != 429 {
		t.Fatalf("unexpected detail: %+v", detail)
	}
}

func TestClassifyCommandError_Auth(t *testing.T) {
	detail := classifyCommandError(config.ErrNotAuthenticated)
	if detail.Type != "auth_error" || detail.Code != "not_authenticated" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
}

func TestClassifyCommandError_JQ(t *testing.T) {
	detail := classifyCommandError(errors.New("invalid jq expression: bad token"))
	if detail.Type != "usage_error" || detail.Code != "invalid_jq" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
}

func TestClassifyCommandError_LikelyUsageError(t *testing.T) {
	detail := classifyCommandError(errors.New("unknown command \"bad\" for \"gumroad\""))
	if detail.Type != "usage_error" || detail.Code != "invalid_input" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
}

func TestClassifyCommandError_ConfirmationUsageError(t *testing.T) {
	detail := classifyCommandError(fmt.Errorf("%w. Use --yes to skip confirmation", prompt.ErrConfirmationNoInput))
	if detail.Type != "usage_error" || detail.Code != "invalid_input" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
}

func TestAPIErrorCode(t *testing.T) {
	for statusCode, want := range map[int]string{
		401: "not_authenticated",
		403: "access_denied",
		404: "not_found",
		429: "rate_limited",
		500: "api_error",
	} {
		if got := apiErrorCode(statusCode); got != want {
			t.Fatalf("status %d: got %q, want %q", statusCode, got, want)
		}
	}
}

func TestStructuredOutputRequestedInFlagSet(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.Bool("json", false, "")
	flags.String("jq", "", "")

	if structuredOutputRequestedInFlagSet(flags) {
		t.Fatal("expected empty flag set to be false")
	}
	if err := flags.Set("json", "true"); err != nil {
		t.Fatalf("Set(json) failed: %v", err)
	}
	if !structuredOutputRequestedInFlagSet(flags) {
		t.Fatal("expected json=true to request structured output")
	}

	flags = pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.Bool("json", false, "")
	flags.String("jq", "", "")
	if err := flags.Set("jq", ".user.email"); err != nil {
		t.Fatalf("Set(jq) failed: %v", err)
	}
	if !structuredOutputRequestedInFlagSet(flags) {
		t.Fatal("expected jq to request structured output")
	}
}

func TestStructuredOutputRequestedFromCommandWithoutContext(t *testing.T) {
	cmd := stubCommand(nil)
	cmd.Flags().Bool("json", false, "")
	cmd.Flags().String("jq", "", "")

	if structuredOutputRequestedFromCommand(cmd) {
		t.Fatal("expected command without flags to be false")
	}
	if err := cmd.Flags().Set("jq", ".user.email"); err != nil {
		t.Fatalf("Set(jq) failed: %v", err)
	}
	if !structuredOutputRequestedFromCommand(cmd) {
		t.Fatal("expected jq flag to request structured output")
	}
}

func TestLikelyErrorHelpers(t *testing.T) {
	if !isLikelyUsageError(errors.New("flag needs an argument: --product")) {
		t.Fatal("expected usage-like error to be detected")
	}
	if isLikelyUsageError(errors.New("plain error")) {
		t.Fatal("did not expect plain error to be usage-like")
	}

	if !isLikelyJQError(errors.New("jq error: bad path")) {
		t.Fatal("expected jq-like error to be detected")
	}
	if isLikelyJQError(errors.New("plain error")) {
		t.Fatal("did not expect plain error to be jq-like")
	}
}

func TestPrintStructuredCommandError_MarshalFallback(t *testing.T) {
	err := printStructuredCommandError(bytes.NewBuffer(nil), errors.New(strings.Repeat("x", 8)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
