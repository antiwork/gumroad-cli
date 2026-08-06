package products

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func productCompsHandler(t *testing.T, wantTaxonomy, wantQuery string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/products/comps" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("taxonomy"); got != wantTaxonomy {
			t.Errorf("taxonomy param = %q, want %q", got, wantTaxonomy)
		}
		if got := r.URL.Query().Get("query"); got != wantQuery {
			t.Errorf("query param = %q, want %q", got, wantQuery)
		}
		testutil.JSON(t, w, map[string]any{
			"success": true,
			"comps": map[string]any{
				"count":       412,
				"price_cents": map[string]any{"p25": 900, "p50": 1500, "p75": 2900},
				"examples": []map[string]any{
					{"name": "Cinematic Whoosh Pack", "price": "$15", "url": "https://sfx.gumroad.com/l/whoosh"},
					{"name": "Transition SFX Bundle", "price": "$29", "url": "https://sfx.gumroad.com/l/transitions"},
				},
			},
		})
	}
}

func TestComps_Table(t *testing.T) {
	testutil.Setup(t, productCompsHandler(t, "music-and-sound-design", "whoosh sfx"))

	cmd := testutil.Command(newCompsCmd())
	cmd.SetArgs([]string{"--category", "music-and-sound-design", "--query", "whoosh sfx"})
	out := testutil.CaptureStdout(func() {
		testutil.MustExecute(t, cmd)
	})

	for _, want := range []string{"412", "$9", "$15", "$29", "Cinematic Whoosh Pack", "https://sfx.gumroad.com/l/whoosh"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %q", want, out)
		}
	}
}

func TestComps_JSON(t *testing.T) {
	testutil.Setup(t, productCompsHandler(t, "music-and-sound-design", ""))

	cmd := testutil.Command(newCompsCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--category", "music-and-sound-design"})
	out := testutil.CaptureStdout(func() {
		testutil.MustExecute(t, cmd)
	})

	var resp compsResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if resp.Comps.Count != 412 {
		t.Fatalf("count = %d, want 412", resp.Comps.Count)
	}
	if resp.Comps.PriceCents.P50 == nil || *resp.Comps.PriceCents.P50 != 1500 {
		t.Fatalf("p50 = %v, want 1500", resp.Comps.PriceCents.P50)
	}
	if len(resp.Comps.Examples) != 2 {
		t.Fatalf("got %d examples, want 2", len(resp.Comps.Examples))
	}
}

func TestComps_RequiresCategoryOrQuery(t *testing.T) {
	cmd := testutil.Command(newCompsCmd())
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--category or --query") {
		t.Fatalf("expected a missing-flag error, got %v", err)
	}
}

func TestComps_RejectsWhitespaceOnlyFilters(t *testing.T) {
	cmd := testutil.Command(newCompsCmd())
	cmd.SetArgs([]string{"--category", "   ", "--query", "	"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--category or --query") {
		t.Fatalf("expected a missing-flag error for whitespace-only filters, got %v", err)
	}
}
