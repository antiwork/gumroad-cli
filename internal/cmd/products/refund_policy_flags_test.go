package products

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestUpdate_RefundPeriodAndFinePrint(t *testing.T) {
	var gotMethod, gotPath string
	var gotForm url.Values
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = r.ParseForm()
		gotForm = r.PostForm
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newUpdateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"prod1", "--refund-period", "none", "--refund-fine-print", "No refunds once downloaded."})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "PUT" {
		t.Errorf("got method %q, want PUT", gotMethod)
	}
	if gotPath != "/products/prod1" {
		t.Errorf("got path %q, want /products/prod1", gotPath)
	}
	if got := gotForm.Get("refund_period"); got != "none" {
		t.Errorf("got refund_period=%q, want %q", got, "none")
	}
	if got := gotForm.Get("refund_fine_print"); got != "No refunds once downloaded." {
		t.Errorf("got refund_fine_print=%q, want fine print", got)
	}
	if !strings.Contains(out, "updated") {
		t.Errorf("expected updated message, got: %q", out)
	}
}

func TestUpdate_RefundPeriodInherit(t *testing.T) {
	var gotForm url.Values
	var hasFinePrint bool
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.PostForm
		_, hasFinePrint = r.PostForm["refund_fine_print"]
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newUpdateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"prod1", "--refund-period", "inherit"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if got := gotForm.Get("refund_period"); got != "inherit" {
		t.Errorf("got refund_period=%q, want inherit", got)
	}
	if hasFinePrint {
		t.Errorf("refund_fine_print should not be sent when the flag is unset")
	}
}

func TestUpdate_RefundFinePrintEmptyClears(t *testing.T) {
	var gotForm url.Values
	var hasFinePrint bool
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.PostForm
		_, hasFinePrint = r.PostForm["refund_fine_print"]
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newUpdateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"prod1", "--refund-period", "30", "--refund-fine-print", ""})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !hasFinePrint {
		t.Fatalf("refund_fine_print should be sent (present but empty) to clear the fine print")
	}
	if got := gotForm.Get("refund_fine_print"); got != "" {
		t.Errorf("got refund_fine_print=%q, want empty string", got)
	}
}

func TestUpdate_InvalidRefundPeriod(t *testing.T) {
	reached := false
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		reached = true
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newUpdateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"prod1", "--refund-period", "45"})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error for invalid refund period")
	}
	if !strings.Contains(err.Error(), "--refund-period must be one of: inherit, none, 7, 14, 30, 183") {
		t.Errorf("unexpected error: %v", err)
	}
	if reached {
		t.Errorf("API should not be reached on invalid --refund-period")
	}
}

func TestUpdate_RefundFinePrintWithInheritRejected(t *testing.T) {
	reached := false
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		reached = true
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newUpdateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"prod1", "--refund-period", "inherit", "--refund-fine-print", "Nope."})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error for --refund-fine-print with inherit")
	}
	if !strings.Contains(err.Error(), "cannot be combined with --refund-period inherit") {
		t.Errorf("unexpected error: %v", err)
	}
	if reached {
		t.Errorf("API should not be reached when flags conflict")
	}
}

func TestUpdate_RefundPeriodAloneSatisfiesRequireAnyFlag(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newUpdateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"prod1", "--refund-period", "7"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
}

func TestCreate_RefundPolicyFlags(t *testing.T) {
	var gotMethod, gotPath string
	var gotForm url.Values
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = r.ParseForm()
		gotForm = r.PostForm
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{"id": "p1", "name": "Art Pack", "formatted_price": "$10"},
		})
	})

	cmd := testutil.Command(newCreateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--name", "Art Pack", "--price", "10", "--refund-period", "none", "--refund-fine-print", "All sales final."})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/products" {
		t.Errorf("got %s %s, want POST /products", gotMethod, gotPath)
	}
	if got := gotForm.Get("refund_period"); got != "none" {
		t.Errorf("got refund_period=%q, want none", got)
	}
	if got := gotForm.Get("refund_fine_print"); got != "All sales final." {
		t.Errorf("got refund_fine_print=%q, want fine print", got)
	}
}

func TestCreate_InvalidRefundPeriod(t *testing.T) {
	reached := false
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		reached = true
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newCreateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--name", "Art Pack", "--refund-period", "forever"})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error for invalid refund period")
	}
	if !strings.Contains(err.Error(), "--refund-period must be one of: inherit, none, 7, 14, 30, 183") {
		t.Errorf("unexpected error: %v", err)
	}
	if reached {
		t.Errorf("API should not be reached on invalid --refund-period")
	}
}

func TestCreate_RefundPolicyOmittedWhenFlagsUnset(t *testing.T) {
	var hasPeriod, hasFinePrint bool
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		_, hasPeriod = r.PostForm["refund_period"]
		_, hasFinePrint = r.PostForm["refund_fine_print"]
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{"id": "p1", "name": "Art Pack", "formatted_price": "$10"},
		})
	})

	cmd := testutil.Command(newCreateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--name", "Art Pack", "--price", "10"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if hasPeriod || hasFinePrint {
		t.Errorf("refund policy params should not be sent when flags are unset (period=%v finePrint=%v)", hasPeriod, hasFinePrint)
	}
}
