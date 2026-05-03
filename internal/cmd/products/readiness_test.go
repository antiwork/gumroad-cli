package products

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func readinessFixture() map[string]any {
	return map[string]any{
		"success": true,
		"readiness": map[string]any{
			"overall":     67,
			"severity":    "ok",
			"computed_at": "2026-05-03T19:00:00Z",
			"categories": []map[string]any{
				{"key": "name", "label": "Name", "weight": 15, "score": 87, "severity": "good", "note": "Clear and specific", "details": []string{"Length 28 chars", "Contains a number"}},
				{"key": "description", "label": "Description", "weight": 30, "score": 50, "severity": "weak", "note": "Lacks structure", "details": []string{"180 words — short"}},
				{"key": "cover", "label": "Cover", "weight": 20, "score": 60, "severity": "ok", "note": "Decent", "details": []string{"1 cover uploaded"}},
				{"key": "pricing", "label": "Pricing", "weight": 15, "score": 85, "severity": "good", "note": "Strong", "details": []string{"Charm pricing applied"}},
				{"key": "social_proof", "label": "Social Proof", "weight": 20, "score": 70, "severity": "ok", "note": "Solid", "details": []string{"42 reviews"}},
			},
		},
	}
}

func TestReadiness_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/products/abc/readiness" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		testutil.JSON(t, w, readinessFixture())
	})

	cmd := testutil.Command(newReadinessCmd(), testutil.JSONOutput())
	var err error
	out := testutil.CaptureStdout(func() { err = cmd.RunE(cmd, []string{"abc"}) })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]any
	if jsonErr := json.Unmarshal([]byte(out), &resp); jsonErr != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", jsonErr, out)
	}
	readiness, ok := resp["readiness"].(map[string]any)
	if !ok {
		t.Fatalf("readiness object missing from output: %s", out)
	}
	if readiness["overall"] != float64(67) {
		t.Errorf("overall = %v, want 67", readiness["overall"])
	}
	cats, ok := readiness["categories"].([]any)
	if !ok || len(cats) != 5 {
		t.Errorf("expected 5 categories, got %v", readiness["categories"])
	}
}

func TestReadiness_Table(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, readinessFixture())
	})

	cmd := newReadinessCmd()
	var err error
	out := testutil.CaptureStdout(func() { err = cmd.RunE(cmd, []string{"abc"}) })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{"Page readiness", "Overall:", "67/100", "Name", "Description", "Cover", "Pricing", "Social Proof", "CATEGORY", "WEIGHT", "SCORE"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestReadiness_Plain(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, readinessFixture())
	})

	cmd := testutil.Command(newReadinessCmd(), testutil.PlainOutput())
	var err error
	out := testutil.CaptureStdout(func() { err = cmd.RunE(cmd, []string{"abc"}) })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "overall\t67\tok") {
		t.Errorf("plain output missing overall row: %q", out)
	}
	if !strings.Contains(out, "name\t87\tgood") {
		t.Errorf("plain output missing name category row: %q", out)
	}
	if !strings.Contains(out, "social_proof\t70\tok") {
		t.Errorf("plain output missing social_proof category row: %q", out)
	}
}

func TestReadiness_Details(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, readinessFixture())
	})

	cmd := newReadinessCmd()
	cmd.SetArgs([]string{"abc", "--details"})
	if err := cmd.Flags().Parse([]string{"--details"}); err != nil {
		t.Fatalf("flag parse error: %v", err)
	}
	var err error
	out := testutil.CaptureStdout(func() { err = cmd.RunE(cmd, []string{"abc"}) })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{"Length 28 chars", "Contains a number", "Charm pricing applied", "42 reviews"} {
		if !strings.Contains(out, want) {
			t.Errorf("--details output missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestReadiness_RequiresID(t *testing.T) {
	cmd := newReadinessCmd()
	if err := cmd.Args(cmd, []string{}); err == nil {
		t.Errorf("expected error when no id provided, got nil")
	}
	if err := cmd.Args(cmd, []string{"a", "b"}); err == nil {
		t.Errorf("expected error when extra args provided, got nil")
	}
	if err := cmd.Args(cmd, []string{"abc"}); err != nil {
		t.Errorf("expected no error with single id, got %v", err)
	}
}
