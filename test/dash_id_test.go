package test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// TestDashPrefixedIDArgs verifies that every command accepting a positional ID
// argument correctly handles IDs starting with "-". Cobra normally interprets
// these as shorthand flags. The CLI catches this error and retries with "--"
// inserted before the offending arg, so the ID is treated as a positional arg.
func TestDashPrefixedIDArgs(t *testing.T) {
	bin := buildBinary(t)

	var mu sync.Mutex
	var lastPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		lastPath = r.URL.Path
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":          true,
			"product":          map[string]any{"id": "p1", "name": "Test"},
			"offer_code":       map[string]any{"id": "oc1", "name": "TEST"},
			"variant_category": map[string]any{"id": "vc1", "title": "Size"},
			"variant":          map[string]any{"id": "v1", "name": "Small"},
			"sale":             map[string]any{"id": "s1", "email": "test@example.com", "product_name": "Test"},
			"subscriber":       map[string]any{"id": "sub1", "email_address": "test@example.com"},
			"payout":           map[string]any{"id": "pay1", "display_payout_period": "Jan 2026", "formatted_amount": "$100"},
			"skus":             []map[string]any{},
		})
	}))
	t.Cleanup(srv.Close)

	cfgDir := setupConfig(t)
	env := []string{
		"XDG_CONFIG_HOME=" + cfgDir,
		"GUMROAD_API_BASE_URL=" + srv.URL,
		"GUMROAD_ACCESS_TOKEN=",
	}

	dashID := "-cGksPcArAUU8j_XTYsrnQ=="

	tests := []struct {
		name string
		args []string
	}{
		// products
		{"products view", []string{"products", "view", dashID}},
		{"products update", []string{"products", "update", "--name", "x", dashID}},
		{"products delete", []string{"products", "delete", "--yes", dashID}},
		{"products publish", []string{"products", "publish", dashID}},
		{"products unpublish", []string{"products", "unpublish", dashID}},
		{"products skus", []string{"products", "skus", dashID}},

		// offer-codes
		{"offer-codes view", []string{"offer-codes", "view", "--product", "p1", dashID}},
		{"offer-codes update", []string{"offer-codes", "update", "--product", "p1", "--max-purchase-count", "5", dashID}},
		{"offer-codes delete", []string{"offer-codes", "delete", "--product", "p1", "--yes", dashID}},

		// variant-categories
		{"variant-categories view", []string{"variant-categories", "view", "--product", "p1", dashID}},
		{"variant-categories update", []string{"variant-categories", "update", "--product", "p1", "--title", "Color", dashID}},
		{"variant-categories delete", []string{"variant-categories", "delete", "--product", "p1", "--yes", dashID}},

		// variants
		{"variants view", []string{"variants", "view", "--product", "p1", "--category", "c1", dashID}},
		{"variants update", []string{"variants", "update", "--product", "p1", "--category", "c1", "--name", "Large", dashID}},
		{"variants delete", []string{"variants", "delete", "--product", "p1", "--category", "c1", "--yes", dashID}},

		// sales
		{"sales view", []string{"sales", "view", dashID}},
		{"sales refund", []string{"sales", "refund", "--yes", dashID}},
		{"sales resend-receipt", []string{"sales", "resend-receipt", dashID}},
		{"sales ship", []string{"sales", "ship", dashID}},

		// subscribers
		{"subscribers view", []string{"subscribers", "view", dashID}},

		// payouts
		{"payouts view", []string{"payouts", "view", dashID}},

		// webhooks
		{"webhooks delete", []string{"webhooks", "delete", "--yes", dashID}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mu.Lock()
			lastPath = ""
			mu.Unlock()
			args := append(tt.args, "--no-color")
			out, err := runGR(t, bin, env, args...)
			if err != nil {
				if strings.Contains(out, "unknown shorthand flag") {
					t.Fatalf("dash-prefixed ID %q was parsed as a flag:\n%s", dashID, out)
				}
				if strings.Contains(out, "missing required argument") {
					t.Fatalf("dash-prefixed ID %q was discarded during flag parsing:\n%s", dashID, out)
				}
				// Other errors (e.g. API returns unexpected shape) are acceptable —
				// the point is that the ID was passed through to the API.
			}
			mu.Lock()
			path := lastPath
			mu.Unlock()
			if path == "" {
				t.Fatalf("API was never called — dash-prefixed ID %q was not passed through", dashID)
			}
			if !strings.Contains(path, dashID) {
				t.Errorf("API path %q does not contain expected ID %q", path, dashID)
			}
		})
	}
}
