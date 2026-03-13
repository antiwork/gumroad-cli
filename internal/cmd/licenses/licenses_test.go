package licenses

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestVerify_UsesFromTopLevel(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		// Gumroad returns uses at top level, not inside purchase
		testutil.JSON(t, w, map[string]any{
			"uses": 42,
			"purchase": map[string]any{
				"email":      "buyer@example.com",
				"product_id": "prod1",
			},
		})
	})

	cmd := testutil.Command(newVerifyCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--product", "prod1", "--key", "ABC-123"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if !strings.Contains(out, "Uses: 42") {
		t.Errorf("expected 'Uses: 42', got: %q", out)
	}
	if !strings.Contains(out, "buyer@example.com") {
		t.Errorf("expected email in output, got: %q", out)
	}
}

func TestVerify_UsesFromTopLevel_Plain(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"uses":     7,
			"purchase": map[string]any{"email": "x@y.com", "product_id": "p1"},
		})
	})

	cmd := testutil.Command(newVerifyCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--product", "p1", "--key", "K1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if !strings.Contains(out, "7") {
		t.Errorf("plain output should contain uses count 7: %q", out)
	}
}

func TestVerify_ReadsKeyFromStdin(t *testing.T) {
	var gotKey string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotKey = r.PostForm.Get("license_key")
		testutil.JSON(t, w, map[string]any{
			"uses":     7,
			"purchase": map[string]any{"email": "x@y.com", "product_id": "p1"},
		})
	})

	cmd := testutil.Command(newVerifyCmd(), testutil.PlainOutput(), testutil.Stdin(strings.NewReader("PIPE-KEY\n")))
	cmd.SetArgs([]string{"--product", "p1"})
	testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if gotKey != "PIPE-KEY" {
		t.Fatalf("got license_key=%q, want PIPE-KEY", gotKey)
	}
}

func TestVerify_EmptyStdinShowsKeySpecificError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach API when the license key is empty")
	})

	cmd := testutil.Command(newVerifyCmd(), testutil.Stdin(strings.NewReader("\n")))
	cmd.SetArgs([]string{"--product", "p1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for empty stdin license key")
	}
	if strings.Contains(err.Error(), "missing required flag: --key") {
		t.Fatalf("unexpected fallback-to-flag error: %v", err)
	}
	if !strings.Contains(err.Error(), "license key cannot be empty") {
		t.Fatalf("expected empty key guidance, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Pipe it via stdin or pass --key") {
		t.Fatalf("expected stdin guidance, got: %v", err)
	}
}

func TestVerify_UsesFromTopLevelFloat(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, `{
			"success": true,
			"uses": 42.0,
			"purchase": {
				"email": "buyer@example.com",
				"product_id": "prod1"
			}
		}`)
	})

	cmd := testutil.Command(newVerifyCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--product", "prod1", "--key", "ABC-123"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if !strings.Contains(out, "Uses: 42") {
		t.Fatalf("expected float-backed uses in output, got: %q", out)
	}
}

func TestVerify_UsesFromTopLevel_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"uses":     99,
			"purchase": map[string]any{"email": "x@y.com", "product_id": "p1"},
		})
	})

	cmd := testutil.Command(newVerifyCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--product", "p1", "--key", "K1"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if resp["uses"] != float64(99) {
		t.Errorf("JSON uses=%v, want 99", resp["uses"])
	}
}

func TestVerify_NoIncrementFlag(t *testing.T) {
	var gotIncrement string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotIncrement = r.PostForm.Get("increment_uses_count")
		testutil.JSON(t, w, map[string]any{
			"uses":     0,
			"purchase": map[string]any{"email": "x@y.com", "product_id": "p1"},
		})
	})

	cmd := newVerifyCmd()
	cmd.SetArgs([]string{"--product", "p1", "--key", "K1", "--no-increment"})
	testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if gotIncrement != "false" {
		t.Errorf("got increment_uses_count=%q, want 'false'", gotIncrement)
	}
}

func TestVerify_DefaultIncrementsUses(t *testing.T) {
	var gotIncrement string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotIncrement = r.PostForm.Get("increment_uses_count")
		testutil.JSON(t, w, map[string]any{
			"uses":     1,
			"purchase": map[string]any{"email": "x@y.com", "product_id": "p1"},
		})
	})

	cmd := newVerifyCmd()
	cmd.SetArgs([]string{"--product", "p1", "--key", "K1"})
	testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})

	// Without --no-increment, increment_uses_count should NOT be set
	// (API defaults to incrementing)
	if gotIncrement != "" {
		t.Errorf("without --no-increment, should not send increment_uses_count, got %q", gotIncrement)
	}
}

func TestVerify_DryRunRedactsKey(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry run should not reach API")
	})

	cmd := testutil.Command(newVerifyCmd(), testutil.DryRun(true), testutil.JSONOutput())
	cmd.SetArgs([]string{"--product", "p1", "--key", "SUPER-SECRET"})
	out := testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})

	if strings.Contains(out, "SUPER-SECRET") {
		t.Fatalf("expected dry-run output to redact the key, got %q", out)
	}
	if !strings.Contains(out, "REDACTED") {
		t.Fatalf("expected redacted value in dry-run output, got %q", out)
	}
}

func TestVerify_UsesPostMethod(t *testing.T) {
	var gotMethod string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		testutil.JSON(t, w, map[string]any{
			"uses":     0,
			"purchase": map[string]any{"email": "x@y.com", "product_id": "p1"},
		})
	})

	cmd := newVerifyCmd()
	cmd.SetArgs([]string{"--product", "p1", "--key", "K1"})
	testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})

	if gotMethod != "POST" {
		t.Errorf("got method %q, want POST", gotMethod)
	}
}

func TestVerify_ProductRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newVerifyCmd()
	cmd.SetArgs([]string{"--key", "K1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --product")
	}
	if !strings.Contains(err.Error(), "--product") {
		t.Errorf("error should mention --product: %v", err)
	}
}

func TestVerify_KeyRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := testutil.Command(newVerifyCmd(), testutil.Stdin(strings.NewReader("")))
	cmd.SetArgs([]string{"--product", "p1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --key")
	}
	if !strings.Contains(err.Error(), "--key") {
		t.Errorf("error should mention --key: %v", err)
	}
}

func TestEnable_FlagValidation(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newEnableCmd()
	cmd.SetArgs([]string{"--key", "K1"}) // missing --product
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --product")
	}
}

func TestEnable_CorrectEndpoint(t *testing.T) {
	var gotMethod, gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		if r.PostForm.Get("product_id") != "p1" {
			t.Errorf("missing product_id param")
		}
		if r.PostForm.Get("license_key") != "K1" {
			t.Errorf("missing license_key param")
		}
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := newEnableCmd()
	cmd.SetArgs([]string{"--product", "p1", "--key", "K1"})
	testutil.CaptureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	})

	if gotMethod != "PUT" || gotPath != "/licenses/enable" {
		t.Errorf("got %s %s, want PUT /licenses/enable", gotMethod, gotPath)
	}
}

func TestDisable_CorrectEndpoint(t *testing.T) {
	var gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := newDisableCmd()
	cmd.SetArgs([]string{"--product", "p1", "--key", "K1"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotPath != "/licenses/disable" {
		t.Errorf("got path %q, want /licenses/disable", gotPath)
	}
}

func TestDecrement_CorrectEndpoint(t *testing.T) {
	var gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := newDecrementCmd()
	cmd.SetArgs([]string{"--product", "p1", "--key", "K1"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotPath != "/licenses/decrement_uses_count" {
		t.Errorf("got path %q, want /licenses/decrement_uses_count", gotPath)
	}
}

func TestRotate_ReturnsNewKey(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"purchase": map[string]any{"license_key": "NEW-KEY-456"},
		})
	})

	cmd := testutil.Command(newRotateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--product", "p1", "--key", "OLD-KEY"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "NEW-KEY-456") {
		t.Errorf("output should contain new key, got: %q", out)
	}
}

func TestEnable_Output(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newEnableCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--product", "p1", "--key", "K1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "enabled") {
		t.Errorf("expected enabled message, got: %q", out)
	}
}

func TestDisable_Output(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newDisableCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--product", "p1", "--key", "K1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "disabled") {
		t.Errorf("expected disabled message, got: %q", out)
	}
}

func TestDecrement_Output(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newDecrementCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--product", "p1", "--key", "K1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "decremented") {
		t.Errorf("expected decremented message, got: %q", out)
	}
}

func TestRotate_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"purchase": map[string]any{"license_key": "NEW-KEY"},
		})
	})

	cmd := testutil.Command(newRotateCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--product", "p1", "--key", "K1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
}

func TestRotate_Plain(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"purchase": map[string]any{"license_key": "NEW-PLAIN"},
		})
	})

	cmd := testutil.Command(newRotateCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--product", "p1", "--key", "K1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "NEW-PLAIN") {
		t.Errorf("plain output missing key: %q", out)
	}
}

func TestRotate_QuietStillPrintsNewKey(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"purchase": map[string]any{"license_key": "NEW-QUIET"},
		})
	})

	cmd := testutil.Command(newRotateCmd(), testutil.Quiet(true))
	cmd.SetArgs([]string{"--product", "p1", "--key", "K1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if strings.TrimSpace(out) != "NEW-QUIET" {
		t.Fatalf("quiet output should contain only the new key, got %q", out)
	}
}

func TestEnable_APIError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		testutil.JSON(t, w, map[string]any{"message": "Error"})
	})

	cmd := newEnableCmd()
	cmd.SetArgs([]string{"--product", "p1", "--key", "K1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDisable_APIError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		testutil.JSON(t, w, map[string]any{"message": "Error"})
	})

	cmd := newDisableCmd()
	cmd.SetArgs([]string{"--product", "p1", "--key", "K1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDecrement_APIError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		testutil.JSON(t, w, map[string]any{"message": "Error"})
	})

	cmd := newDecrementCmd()
	cmd.SetArgs([]string{"--product", "p1", "--key", "K1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRotate_APIError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		testutil.JSON(t, w, map[string]any{"message": "Error"})
	})

	cmd := newRotateCmd()
	cmd.SetArgs([]string{"--product", "p1", "--key", "K1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDisable_FlagValidation(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := newDisableCmd()
	cmd.SetArgs([]string{"--key", "K1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --product")
	}
}

func TestDecrement_FlagValidation(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := testutil.Command(newDecrementCmd(), testutil.Stdin(strings.NewReader("")))
	cmd.SetArgs([]string{"--product", "p1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --key")
	}
}

func TestNewLicensesCmd(t *testing.T) {
	cmd := NewLicensesCmd()
	if cmd.Use != "licenses" {
		t.Errorf("got Use=%q, want licenses", cmd.Use)
	}
	subs := make(map[string]bool)
	for _, c := range cmd.Commands() {
		subs[c.Use] = true
	}
	for _, name := range []string{"verify", "enable", "disable", "decrement", "rotate"} {
		if !subs[name] {
			t.Errorf("missing subcommand %q", name)
		}
	}
}

func TestRotate_FlagValidation(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API")
	})

	cmd := testutil.Command(newRotateCmd(), testutil.Stdin(strings.NewReader("")))
	cmd.SetArgs([]string{"--product", "p1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --key")
	}
}
