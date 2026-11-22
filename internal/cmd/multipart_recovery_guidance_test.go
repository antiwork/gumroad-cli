package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestMultipartRecoverySellerGuidance(t *testing.T) {
	const manifest = `{"upload_id":"upload-one","key":"demo/file","completed_parts":[{"part_number":1,"etag":"etag-one"}]}`
	const missing = "multipart_upload_not_found"
	for _, tc := range []struct {
		name       string
		status     int
		serverCode string
		wantCode   string
		wantVerify bool
	}{
		{"tagged_missing_upload", 400, missing, "complete_state_unknown", true},
		{"untagged_rejection", 400, "", "complete_rejected", false},
		{"generic_ambiguous", 502, "", "complete_state_unknown", false},
		{"same_tag_other_status", 502, missing, "complete_state_unknown", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/files/complete" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				body := map[string]any{"error": "The specified upload does not exist."}
				if tc.serverCode != "" {
					body["error_code"] = tc.serverCode
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				if err := json.NewEncoder(w).Encode(body); err != nil {
					t.Errorf("write stub response: %v", err)
				}
			})

			var terminal, structured bytes.Buffer
			cmd := NewRootCmd()
			cmd.SetIn(strings.NewReader(manifest))
			cmd.SetErr(&terminal)
			cmd.SetArgs([]string{"files", "complete", "--recovery", "-", "--yes", "--no-input", "--no-color"})
			executed, err := cmd.ExecuteC()
			if err == nil {
				t.Fatal("expected completion failure")
			}
			printHumanCommandError(executed, err)
			if err := printStructuredCommandError(&structured, err); err != nil {
				t.Fatal(err)
			}
			var envelope commandErrorEnvelope
			if err := json.Unmarshal(structured.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			detail := envelope.Error
			if detail.Code != tc.wantCode {
				t.Fatalf("error code = %q, want %q", detail.Code, tc.wantCode)
			}
			recovery := detail.Recovery
			if recovery == nil || recovery.UploadID != "upload-one" || recovery.Key != "demo/file" ||
				len(recovery.CompletedParts) != 1 || recovery.CompletedParts[0] != (uploadCompletedPart{PartNumber: 1, ETag: "etag-one"}) {
				t.Errorf("unexpected structured recovery: %+v", recovery)
			}
			output := terminal.String()
			if !strings.Contains(output, "Hint: "+detail.Hint) || !strings.Contains(output, "part_number=1 etag=etag-one") {
				t.Errorf("missing shared hint or part details in terminal output: %s", output)
			}
			if tc.wantVerify && (!strings.Contains(detail.Hint, "Keep the recovery manifest") || !strings.Contains(detail.Hint, "verify")) {
				t.Errorf("missing tagged recovery guidance: %q", detail.Hint)
			}
			for _, action := range []string{"gumroad files complete", "gumroad files abort"} {
				if tc.wantVerify && (strings.Contains(output, action) || strings.Contains(detail.Hint, action)) {
					t.Errorf("unexpected action in tagged recovery output: %q", action)
				}
				if !tc.wantVerify && !strings.Contains(detail.Hint, action) {
					t.Errorf("existing recovery hint lost %q: %s", action, detail.Hint)
				}
			}
		})
	}
}
