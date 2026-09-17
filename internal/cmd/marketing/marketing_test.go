package marketing

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	_ "unsafe"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/testutil"
)

//go:linkname promptIsTerminal github.com/antiwork/gumroad-cli/internal/prompt.isTerminal
var promptIsTerminal func(int) bool

func actionPayload() map[string]any {
	return map[string]any{"id": "action-id", "idempotency_key": "stable-key", "confirmation_token": "preview-token", "status": "recommended", "post_text": "Launch\n\nhttps://example.com/link", "link_url": "https://example.com/link", "external_url": "", "error_code": ""}
}

func TestMarketingLifecycle(t *testing.T) {
	state := "recommended"
	posts := 0
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		item := actionPayload()
		if r.URL.Path == "/products/product-id/marketing/recommendations" {
			if r.Method != http.MethodGet {
				t.Errorf("method: %s", r.Method)
			}
			testutil.JSON(t, w, map[string]any{"channels": []any{map[string]any{"channel": "x", "live": true, "handle": "seller", "action": item}, map[string]any{"channel": "instagram", "live": false}}})
			return
		}
		if r.Method == http.MethodPost {
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			if r.PostForm.Get("idempotency_key") != "stable-key" {
				t.Errorf("key: %v", r.PostForm)
			}
			switch r.URL.Path {
			case "/marketing_actions/action-id/approve":
				if state != "posted" {
					state = "approved"
				}
			case "/marketing_actions/action-id/execute":
				if state == "approved" {
					posts++
					state = "posted"
				}
			case "/marketing_actions/action-id/cancel":
				state = "cancelled"
			default:
				t.Errorf("unexpected route %s", r.URL.Path)
			}
		} else if r.URL.Path != "/marketing_actions/action-id" {
			t.Errorf("unexpected route %s", r.URL.Path)
		}
		item["status"] = state
		testutil.JSON(t, w, map[string]any{"marketing_action": item, "handle": "seller"})
	})
	for _, args := range [][]string{{"recommend", "product-id"}, {"approve", "action-id"}, {"schedule", "action-id"}, {"approve", "action-id"}, {"schedule", "action-id"}, {"status", "action-id"}, {"cancel", "action-id"}} {
		var out, preview bytes.Buffer
		cmd := testutil.Command(NewMarketingCmd(), testutil.JSONOutput(), testutil.Yes(true), testutil.Stdout(&out), func(opts *cmdutil.Options) { opts.Stderr = &preview })
		if args[0] == "approve" || args[0] == "schedule" {
			args = append(args, "--confirmation-token", "preview-token")
		}
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		if !json.Valid(out.Bytes()) {
			t.Fatalf("not one JSON response: %s", out.String())
		}
		if args[0] == "approve" || args[0] == "schedule" || args[0] == "cancel" {
			for _, text := range []string{"Account: @seller", `Exact post text: "Launch\n\nhttps://example.com/link"`, `Link: "https://example.com/link"`} {
				if !strings.Contains(preview.String(), text) {
					t.Errorf("missing %q in %q", text, preview.String())
				}
			}
		}
	}
	if posts != 1 {
		t.Fatalf("posts: %d", posts)
	}
}

func TestMutationRequiresConfirmation(t *testing.T) {
	for _, verb := range []string{"approve", "schedule", "cancel"} {
		t.Run(verb, func(t *testing.T) {
			writes := 0
			testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					writes++
				}
				testutil.JSON(t, w, map[string]any{"marketing_action": actionPayload(), "handle": "seller"})
			})
			cmd := testutil.Command(NewMarketingCmd(), testutil.NoInput(true))
			cmd.SetArgs([]string{verb, "action-id"})
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "confirmation required") {
				t.Fatalf("error: %v", err)
			}
			if writes != 0 {
				t.Fatalf("unconfirmed writes: %d", writes)
			}
		})
	}
}

func TestDryRunFetchesButDoesNotWrite(t *testing.T) {
	for _, verb := range []string{"approve", "schedule", "cancel"} {
		t.Run(verb, func(t *testing.T) {
			reads := 0
			testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("unexpected write: %s", r.Method)
				}
				reads++
				testutil.JSON(t, w, map[string]any{"marketing_action": actionPayload(), "handle": "seller"})
			})
			var out bytes.Buffer
			cmd := testutil.Command(NewMarketingCmd(), testutil.DryRun(true), testutil.JSONOutput(), testutil.Stdout(&out))
			cmd.SetArgs([]string{verb, "action-id"})
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if reads != 1 || !strings.Contains(out.String(), `"dry_run": true`) || !strings.Contains(out.String(), "stable-key") {
				t.Fatalf("reads=%d out=%s", reads, out.String())
			}
		})
	}
}

func TestMissingPreviewRefusesMutation(t *testing.T) {
	for _, field := range []string{"id", "idempotency_key", "confirmation_token", "post_text", "link_url", "handle"} {
		t.Run(field, func(t *testing.T) {
			testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("unexpected write")
				}
				item := actionPayload()
				handle := "seller"
				if field == "handle" {
					handle = ""
				} else {
					item[field] = ""
				}
				testutil.JSON(t, w, map[string]any{"marketing_action": item, "handle": handle})
			})
			cmd := testutil.Command(NewMarketingCmd(), testutil.Yes(true))
			cmd.SetArgs([]string{"approve", "action-id"})
			if err := cmd.Execute(); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestReadOutputModes(t *testing.T) {
	for _, verb := range []string{"recommend", "status"} {
		for _, mode := range []string{"plain", "human", "quiet", "json"} {
			t.Run(verb+"/"+mode, func(t *testing.T) {
				testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
					item := actionPayload()
					item["post_text"] = "Launch\n\x1b[31m"
					if verb == "recommend" {
						testutil.JSON(t, w, map[string]any{"channels": []any{map[string]any{"channel": "x", "live": true, "handle": "seller", "action": item}, map[string]any{"channel": "instagram", "live": false}}})
					} else {
						testutil.JSON(t, w, map[string]any{"marketing_action": item, "handle": "seller"})
					}
				})
				var out bytes.Buffer
				cmd := testutil.Command(NewMarketingCmd(), testutil.Stdout(&out), func(opts *cmdutil.Options) {
					opts.Quiet = mode == "quiet"
					opts.PlainOutput = mode == "plain"
					opts.JSONOutput = mode == "json"
				})
				cmd.SetArgs([]string{verb, "action-id"})
				if err := cmd.Execute(); err != nil {
					t.Fatal(err)
				}
				if mode == "quiet" {
					if out.Len() != 0 {
						t.Fatal(out.String())
					}
					return
				}
				if !strings.Contains(out.String(), "action-id") || strings.Contains(out.String(), "\x1b") {
					t.Fatalf("unsafe or missing output: %q", out.String())
				}
			})
		}
	}
}

func TestAPIErrors(t *testing.T) {
	for _, verb := range []string{"recommend", "status", "approve"} {
		t.Run(verb, func(t *testing.T) {
			testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				testutil.RawJSON(t, w, `{"success":false,"message":"Not found"}`)
			})
			cmd := testutil.Command(NewMarketingCmd(), testutil.Yes(true))
			cmd.SetArgs([]string{verb, "action-id"})
			if err := cmd.Execute(); err == nil {
				t.Fatal("expected API error")
			}
		})
	}
}

func TestDeclinedConfirmation(t *testing.T) {
	old := promptIsTerminal
	promptIsTerminal = func(int) bool { return true }
	t.Cleanup(func() { promptIsTerminal = old })
	input, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = input.Close() })
	if _, err := io.WriteString(writer, "n\n"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Error("unconfirmed write")
		}
		testutil.JSON(t, w, map[string]any{"marketing_action": actionPayload(), "handle": "seller"})
	})
	var out bytes.Buffer
	cmd := testutil.Command(NewMarketingCmd(), testutil.Quiet(false), testutil.Stdout(&out), func(opts *cmdutil.Options) { opts.Stdin = input })
	cmd.SetArgs([]string{"approve", "action-id"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Cancelled") {
		t.Fatal(out.String())
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("broken output") }

func TestPreviewWriteFailurePreventsMutation(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Error("unexpected write")
		}
		testutil.JSON(t, w, map[string]any{"marketing_action": actionPayload(), "handle": "seller"})
	})
	cmd := testutil.Command(NewMarketingCmd(), testutil.Yes(true), func(opts *cmdutil.Options) { opts.Stderr = brokenWriter{} })
	cmd.SetArgs([]string{"approve", "action-id"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected output error")
	}
}
