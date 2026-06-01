package refundpolicy

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
	"github.com/spf13/cobra"
)

func refundPolicyPayload(period, title string, finePrint any, inEffect bool) map[string]any {
	return map[string]any{
		"refund_policy": map[string]any{
			"refund_period": period,
			"title":         title,
			"fine_print":    finePrint,
			"in_effect":     inEffect,
		},
	}
}

func TestView_RendersRefundPolicy(t *testing.T) {
	var gotMethod, gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		testutil.JSON(t, w, refundPolicyPayload(
			"30",
			"30-day money back guarantee",
			"Refund requests are reviewed within 2 business days.",
			true,
		))
	})

	cmd := newViewCmd()
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodGet || gotPath != refundPolicyPath {
		t.Fatalf("got %s %s, want GET %s", gotMethod, gotPath, refundPolicyPath)
	}
	for _, want := range []string{
		"Period: 30",
		"Title: 30-day money back guarantee",
		"Fine print: Refund requests are reviewed within 2 business days.",
		"In effect: yes",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestNewRefundPolicyCmd_RegistersSubcommands(t *testing.T) {
	cmd := NewRefundPolicyCmd()

	for _, args := range [][]string{{"view"}, {"set", "--period", "30"}} {
		found, _, err := cmd.Find(args)
		if err != nil {
			t.Fatalf("Find(%v) failed: %v", args, err)
		}
		if found == nil {
			t.Fatalf("Find(%v) returned nil command", args)
		}
	}
}

func TestView_NilFinePrintAndInactivePolicy(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, refundPolicyPayload("none", "No refunds allowed", nil, false))
	})

	cmd := newViewCmd()
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{
		"Fine print: (none)",
		"In effect: no",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestView_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, refundPolicyPayload("none", "No refunds allowed", nil, false))
	})

	cmd := testutil.Command(newViewCmd(), testutil.JSONOutput())
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		RefundPolicy struct {
			RefundPeriod string `json:"refund_period"`
			InEffect     bool   `json:"in_effect"`
		} `json:"refund_policy"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if resp.RefundPolicy.RefundPeriod != "none" || resp.RefundPolicy.InEffect {
		t.Fatalf("unexpected JSON response: %+v", resp.RefundPolicy)
	}
}

func TestView_JQ(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, refundPolicyPayload("14", "14-day money back guarantee", nil, true))
	})

	cmd := testutil.Command(newViewCmd(), testutil.JQ(".refund_policy.refund_period"))
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if strings.TrimSpace(out) != `"14"` {
		t.Fatalf("got %q, want %q", strings.TrimSpace(out), `"14"`)
	}
}

func TestView_Plain(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, refundPolicyPayload("7", "7-day money back guarantee", "Short terms.", true))
	})

	cmd := testutil.Command(newViewCmd(), testutil.PlainOutput())
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if strings.TrimSpace(out) != "7\t7-day money back guarantee\tShort terms.\ttrue" {
		t.Fatalf("unexpected plain output: %q", out)
	}
}

func TestSet_SendsRefundPolicyParams(t *testing.T) {
	var gotMethod, gotPath, gotPeriod, gotFinePrint string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotPeriod = r.PostForm.Get("refund_period")
		gotFinePrint = r.PostForm.Get("fine_print")
		testutil.JSON(t, w, refundPolicyPayload(
			"30",
			"30-day money back guarantee",
			"Refund requests are reviewed within 2 business days.",
			true,
		))
	})

	cmd := newSetCmd()
	cmd.SetArgs([]string{"--period", "30", "--fine-print", "Refund requests are reviewed within 2 business days."})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodPut || gotPath != refundPolicyPath {
		t.Fatalf("got %s %s, want PUT %s", gotMethod, gotPath, refundPolicyPath)
	}
	if gotPeriod != "30" {
		t.Fatalf("refund_period = %q, want 30", gotPeriod)
	}
	if gotFinePrint != "Refund requests are reviewed within 2 business days." {
		t.Fatalf("fine_print = %q", gotFinePrint)
	}
	if !strings.Contains(out, "Updated refund policy.") || !strings.Contains(out, "Period: 30") {
		t.Fatalf("output missing updated policy: %q", out)
	}
}

func TestSet_OmitsFinePrintWhenFlagNotProvided(t *testing.T) {
	var hasFinePrint bool
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		_, hasFinePrint = r.PostForm["fine_print"]
		testutil.JSON(t, w, refundPolicyPayload("7", "7-day money back guarantee", nil, true))
	})

	cmd := testutil.Command(newSetCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--period", "7"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if hasFinePrint {
		t.Fatal("fine_print must be omitted when --fine-print is not provided")
	}
}

func TestSet_QuietSuppressesHumanOutput(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, refundPolicyPayload("7", "7-day money back guarantee", nil, true))
	})

	cmd := testutil.Command(newSetCmd())
	cmd.SetArgs([]string{"--period", "7"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if out != "" {
		t.Fatalf("quiet set command should not print human output, got %q", out)
	}
}

func TestSet_EmptyFinePrintClearsPolicy(t *testing.T) {
	var gotValues []string
	var hasFinePrint bool
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotValues, hasFinePrint = r.PostForm["fine_print"]
		testutil.JSON(t, w, refundPolicyPayload("none", "No refunds allowed", nil, true))
	})

	cmd := newSetCmd()
	cmd.SetArgs([]string{"--period", "none", "--fine-print", ""})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !hasFinePrint {
		t.Fatal("fine_print must be sent when --fine-print is provided")
	}
	if len(gotValues) != 1 || gotValues[0] != "" {
		t.Fatalf("fine_print values = %#v, want one empty string", gotValues)
	}
}

func TestSet_InvalidPeriodErrorsLocally(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("invalid period must not reach API")
	})

	cmd := newSetCmd()
	cmd.SetArgs([]string{"--period", "0"})
	err := cmd.Execute()

	if err == nil {
		t.Fatal("expected invalid period error")
	}
	if !strings.Contains(err.Error(), "--period must be one of: none, 7, 14, 30, 183") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSet_PeriodRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("missing period must not reach API")
	})

	cmd := newSetCmd()
	err := cmd.Execute()

	if err == nil || !strings.Contains(err.Error(), "missing required flag: --period") {
		t.Fatalf("expected missing period error, got: %v", err)
	}
}

func TestSet_DryRunDoesNotReachAPI(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run must not reach API")
	})

	cmd := testutil.Command(newSetCmd(), testutil.DryRun(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--period", "183", "--fine-print", "Longer refund window."})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{
		"Dry run: PUT /refund_policy",
		"refund_period: 183",
		"fine_print: Longer refund window.",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q: %q", want, out)
		}
	}
}

func TestSet_NotInEffectRejectionSurfaces(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, `{"success":false,"message":"The account-level refund policy is not in effect for this seller."}`)
	})

	cmd := newSetCmd()
	cmd.SetArgs([]string{"--period", "7", "--fine-print", "Updated fine print"})
	err := cmd.Execute()

	if err == nil {
		t.Fatal("expected API error")
	}
	if !strings.Contains(err.Error(), "The account-level refund policy is not in effect for this seller.") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRefundPeriodCompletion(t *testing.T) {
	values, directive := refundPeriodCompletion(nil, nil, "")

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("directive = %v, want NoFileComp", directive)
	}
	if strings.Join(values, ",") != "none,7,14,30,183" {
		t.Fatalf("values = %#v", values)
	}
}
