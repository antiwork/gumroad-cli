package users

import (
	"bytes"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

const shadowFixture = `{"mode":"historical_shadow","notice":"Stored shadow evidence only; not current eligibility or payout authorization.","evaluated_on":"2026-09-02","recorded_at":"2026-09-03T12:00:00Z","score":70,"would_have_released":true,"hold_source":"risk_state_not_reviewed","signals":{"platform":"twitter","components":{"followers":25}},"future_field":"preserved"}`

func TestSocialShadowOutput(t *testing.T) {
	for _, state := range []string{"missing", "null", "present", "no connections", "zero", "escaping"} {
		t.Run(state, func(t *testing.T) {
			payload := map[string]any{}
			if err := json.Unmarshal([]byte(socialEvidenceFixture), &payload); err != nil {
				t.Fatal(err)
			}
			if state != "missing" {
				payload["latest_shadow_evaluation"] = nil
			}
			if state != "missing" && state != "null" {
				shadow := map[string]any{}
				if err := json.Unmarshal([]byte(shadowFixture), &shadow); err != nil {
					t.Fatal(err)
				}
				if state == "zero" {
					shadow["score"] = float64(0)
					shadow["would_have_released"] = false
					shadow["signals"] = nil
				}
				if state == "escaping" {
					shadow["hold_source"] = "bad\n\x1b[31m"
					shadow["recorded_at"] = "date\n\x1b[31m"
				}
				payload["latest_shadow_evaluation"] = shadow
			}
			if state == "no connections" {
				payload["social_connections"] = []any{}
			}
			data, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			for _, mode := range []string{"human", "json", "jq", "plain", "quiet"} {
				t.Run(mode, func(t *testing.T) {
					requests := 0
					testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
						requests++
						if r.Method != http.MethodGet || r.URL.Path != "/internal/admin/users/social_connections" {
							t.Errorf("unexpected request: %s %s", r.Method, r.URL)
						}
						testutil.RawJSON(t, w, string(data))
					})
					var out bytes.Buffer
					cmd := testutil.Command(newSocialConnectionsCmd(), testutil.Stdout(&out), testutil.Quiet(false))
					switch mode {
					case "json":
						cmd = testutil.Command(newSocialConnectionsCmd(), testutil.Stdout(&out), testutil.JSONOutput())
					case "jq":
						cmd = testutil.Command(newSocialConnectionsCmd(), testutil.Stdout(&out), testutil.JQ(".latest_shadow_evaluation"))
					case "plain":
						cmd = testutil.Command(newSocialConnectionsCmd(), testutil.Stdout(&out), testutil.PlainOutput())
					case "quiet":
						cmd = testutil.Command(newSocialConnectionsCmd(), testutil.Stdout(&out), testutil.Quiet(true))
					}
					cmd.SetArgs([]string{"--email", "seller@example.com"})
					testutil.MustExecute(t, cmd)
					if requests != 1 {
						t.Fatalf("requests = %d", requests)
					}
					switch mode {
					case "json", "jq":
						var got any
						if err := json.Unmarshal(out.Bytes(), &got); err != nil {
							t.Fatal(err)
						}
						var want any = payload
						if mode == "jq" {
							want = payload["latest_shadow_evaluation"]
						}
						if !reflect.DeepEqual(got, want) {
							t.Fatalf("output changed: %s", out.String())
						}
					case "quiet":
						if out.Len() != 0 {
							t.Fatal(out.String())
						}
					case "plain":
						want := "instagram\t1234\tseller\tfalse\t2026-09-01T12:00:00Z\t\tunknown\t0\t\t2\n"
						if state == "no connections" {
							want = ""
						}
						if out.String() != want {
							t.Fatalf("plain schema changed: %q", out.String())
						}
					case "human":
						if state == "missing" || state == "null" {
							if !strings.Contains(out.String(), "No stored shadow evaluation supplied (not a negative score).") || strings.Contains(out.String(), "Stored score:") {
								t.Fatal(out.String())
							}
						} else {
							for _, want := range []string{"Historical SHADOW evaluation (not current eligibility or payout authorization)", "Evaluated on: 2026-09-02", "Stored signals:"} {
								if !strings.Contains(out.String(), want) {
									t.Fatalf("missing %q: %s", want, out.String())
								}
							}
							score, outcome := "70", "true"
							if state == "zero" {
								score, outcome = "0", "false"
							}
							if !strings.Contains(out.String(), "Stored score: "+score) || !strings.Contains(out.String(), "Would have released at evaluation time (shadow only): "+outcome) {
								t.Fatal(out.String())
							}
							if strings.Contains(out.String(), "\x1b[31m") || strings.Contains(out.String(), "bad\n") {
								t.Fatalf("unescaped: %q", out.String())
							}
						}
						if state != "no connections" && !strings.Contains(out.String(), "Currently linked: false") {
							t.Fatal("lost connections")
						}
					}
				})
			}
		})
	}
}
