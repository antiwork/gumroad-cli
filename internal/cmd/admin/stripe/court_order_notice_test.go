package stripe

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/adminconfig"
	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/testutil"
)

const (
	testStripeAccountID  = "acct_1Example"
	testFirmName         = "Example Law LLP"
	testFirmEmail        = "notices@example.com"
	testNoticeKey        = "acct_1Example:notices@example.com:abc123"
	testSellerID         = "seller-external-id"
	testSellerEmail      = "seller@example.com"
	testCommentID        = "comment-external-id"
	testCommentCreatedAt = "2026-10-15T12:00:00Z"
	testAcceptedAt       = "2026-10-15T12:01:00Z"
	testEmailBody        = "<div>\n  <p>Hi,</p>\n  <p>Law firm: Example Law LLP<br>\n  Email: <a href=\"mailto:notices@example.com\">notices@example.com</a></p>\n</div>\n"
)

func courtOrderNoticeArgs() []string {
	return []string{
		"--stripe-account-id", testStripeAccountID,
		"--firm-name", testFirmName,
		"--firm-email", testFirmEmail,
		"--notice-key", testNoticeKey,
	}
}

func sellerFixture() map[string]any {
	return map[string]any{"user_id": testSellerID, "email": testSellerEmail, "name": "Seller One"}
}

type noticeState int

const (
	noticeNew noticeState = iota
	noticePending
	noticeAccepted
)

func completionFixture(state noticeState) map[string]any {
	switch state {
	case noticePending:
		return map[string]any{"already_sent": false, "delivery_status": "pending", "accepted_at": nil, "comment_id": testCommentID}
	case noticeAccepted:
		return map[string]any{"already_sent": true, "delivery_status": "accepted", "accepted_at": testAcceptedAt, "comment_id": testCommentID}
	default:
		return map[string]any{"already_sent": false, "delivery_status": nil, "accepted_at": nil, "comment_id": nil}
	}
}

func previewFixture(state noticeState) map[string]any {
	preview := map[string]any{
		"status": "dry_run",
		"user":   sellerFixture(),
		"email": map[string]any{
			"from":      "Gumroad <noreply@example.com>",
			"reply_to":  []string{"support@example.com"},
			"to":        []string{testSellerEmail},
			"subject":   "Important notice regarding your Gumroad payment account",
			"html_body": testEmailBody,
		},
	}
	for key, value := range completionFixture(state) {
		preview[key] = value
	}
	return preview
}

func sendFixture(status string) map[string]any {
	state := noticePending
	if status == courtOrderNoticeStatusAlreadySent {
		state = noticeAccepted
	}
	result := map[string]any{
		"status":  status,
		"user":    sellerFixture(),
		"comment": map[string]any{"content": "Stripe court order notice for " + testStripeAccountID, "created_at": testCommentCreatedAt},
	}
	for key, value := range completionFixture(state) {
		result[key] = value
	}
	return result
}

type recordedRequests struct {
	bodies []courtOrderNoticeRequest
	raw    []map[string]any
	auth   string
}

func recordingHandler(t *testing.T, recorded *recordedRequests, respond func(http.ResponseWriter)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/internal/admin/stripe_restraint_notices" {
			t.Errorf("got %s %s, want POST /internal/admin/stripe_restraint_notices", r.Method, r.URL.Path)
		}
		var raw map[string]any
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		encoded, _ := json.Marshal(raw)
		var body courtOrderNoticeRequest
		if err := json.Unmarshal(encoded, &body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		recorded.bodies = append(recorded.bodies, body)
		recorded.raw = append(recorded.raw, raw)
		recorded.auth = r.Header.Get("Authorization")
		respond(w)
	}
}

func respondJSON(t *testing.T, data map[string]any) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) { testutil.JSON(t, w, data) }
}

func respondError(status int, message string) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "message": message})
	}
}

func TestCourtOrderNoticeRejectsMalformedInputWithoutNetwork(t *testing.T) {
	withFlag := func(flag, value string) []string {
		args := courtOrderNoticeArgs()
		for i := range args {
			if args[i] == flag {
				args[i+1] = value
			}
		}
		return args
	}

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"missing account", withFlag("--stripe-account-id", " "), "missing required flag: --stripe-account-id"},
		{"missing firm name", withFlag("--firm-name", ""), "missing required flag: --firm-name"},
		{"missing firm email", withFlag("--firm-email", ""), "missing required flag: --firm-email"},
		{"missing notice key", withFlag("--notice-key", ""), "missing required flag: --notice-key"},
		{"non-Stripe account", withFlag("--stripe-account-id", "cus_1Example"), "--stripe-account-id must be a Stripe account ID"},
		{"firm name with line break", withFlag("--firm-name", "Example\nLaw"), "--firm-name cannot contain line breaks"},
		{"firm name too long", withFlag("--firm-name", strings.Repeat("a", maxFirmNameLength+1)), "--firm-name must be at most"},
		{"firm email with display name", withFlag("--firm-email", "Example <notices@example.com>"), "--firm-email must be a single email address"},
		{"firm email list", withFlag("--firm-email", "a@example.com, b@example.com"), "--firm-email must be a single email address"},
		{"notice key with space", withFlag("--notice-key", "notice 1"), "--notice-key must be 1-200 printable ASCII characters"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
				t.Error("malformed input must not reach the API")
			})

			cmd := testutil.Command(newCourtOrderNoticeCmd(), testutil.Yes(true))
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want error containing %q", err, tt.want)
			}
		})
	}
}

func TestCourtOrderNoticeDryRunPostsOnceAndPrintsSellerAndEmail(t *testing.T) {
	var recorded recordedRequests
	testutil.SetupAdmin(t, recordingHandler(t, &recorded, respondJSON(t, previewFixture(noticeNew))))

	var stderr bytes.Buffer
	cmd := testutil.Command(newCourtOrderNoticeCmd(), testutil.DryRun(true), testutil.NoInput(true), testutil.Quiet(false), testutil.Stderr(&stderr))
	cmd.SetArgs(courtOrderNoticeArgs())
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if len(recorded.bodies) != 1 {
		t.Fatalf("got %d requests, want 1", len(recorded.bodies))
	}
	want := courtOrderNoticeRequest{
		StripeAccountID: testStripeAccountID,
		FirmName:        testFirmName,
		FirmEmail:       testFirmEmail,
		NoticeKey:       testNoticeKey,
		DryRun:          true,
	}
	if recorded.bodies[0] != want {
		t.Fatalf("got body %+v, want %+v", recorded.bodies[0], want)
	}
	if recorded.auth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want stored admin token", recorded.auth)
	}
	if !strings.Contains(stderr.String(), "Admin actor: Test Admin (admin@example.com)") {
		t.Fatalf("expected actor banner, got stderr %q", stderr.String())
	}
	for _, want := range []string{
		"Dry run: nothing was sent or recorded.",
		"Delivery status: none (not recorded yet)",
		"User ID: " + testSellerID,
		"Email: " + testSellerEmail,
		"Reply-To: support@example.com",
		"To: " + testSellerEmail,
		"Subject: Important notice regarding your Gumroad payment account",
		strings.TrimRight(testEmailBody, "\n"),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "accepted") || strings.Contains(out, "Comment ID") {
		t.Errorf("a new notice must not report a recorded or accepted notice:\n%s", out)
	}
}

func TestCourtOrderNoticeDryRunReportsRecordedNoticeDeliveryStatus(t *testing.T) {
	tests := []struct {
		name    string
		state   noticeState
		want    []string
		notWant string
	}{
		{"pending", noticePending, []string{"Delivery status: pending", "the mail service has not accepted it yet, so a real call would queue the email again", "Comment ID: " + testCommentID}, "Accepted at"},
		{"accepted", noticeAccepted, []string{"Delivery status: accepted", "The mail service already accepted this notice, so a real call would answer already_sent and queue nothing.", "Accepted at: " + testAcceptedAt, "Comment ID: " + testCommentID}, "queue the email again"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.SetupAdmin(t, respondingHandler(respondJSON(t, previewFixture(tt.state))))

			cmd := testutil.Command(newCourtOrderNoticeCmd(), testutil.DryRun(true), testutil.Quiet(false))
			cmd.SetArgs(courtOrderNoticeArgs())
			out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q:\n%s", want, out)
				}
			}
			if strings.Contains(out, tt.notWant) {
				t.Errorf("output must not contain %q:\n%s", tt.notWant, out)
			}
		})
	}
}

func TestCourtOrderNoticeDryRunRefusesInconsistentCompletion(t *testing.T) {
	acceptedWithoutTimestamp := previewFixture(noticeAccepted)
	acceptedWithoutTimestamp["accepted_at"] = nil
	missingAlreadySent := previewFixture(noticeNew)
	delete(missingAlreadySent, "already_sent")
	pendingWithoutComment := previewFixture(noticeNew)
	pendingWithoutComment["delivery_status"] = deliveryStatusPending

	tests := []struct {
		name string
		resp map[string]any
		want string
	}{
		{"accepted without accepted_at", acceptedWithoutTimestamp, `already_sent is true but accepted_at is ""`},
		{"missing already_sent", missingAlreadySent, "already_sent is missing"},
		{"pending without comment", pendingWithoutComment, `delivery_status is "pending" without a comment_id`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.SetupAdmin(t, respondingHandler(respondJSON(t, tt.resp)))

			cmd := testutil.Command(newCourtOrderNoticeCmd(), testutil.DryRun(true), testutil.Quiet(false))
			cmd.SetArgs(courtOrderNoticeArgs())
			var err error
			out := testutil.CaptureStdout(func() { err = cmd.Execute() })

			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want error containing %q", err, tt.want)
			}
			if out != "" {
				t.Fatalf("must not print a preview for an inconsistent receipt, got:\n%s", out)
			}
		})
	}
}

func TestCourtOrderNoticeDryRunJSONPassesResponseThrough(t *testing.T) {
	testutil.SetupAdmin(t, respondingHandler(respondJSON(t, previewFixture(noticePending))))

	cmd := testutil.Command(newCourtOrderNoticeCmd(), testutil.DryRun(true), testutil.JSONOutput())
	cmd.SetArgs(courtOrderNoticeArgs())
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp courtOrderNoticePreview
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if resp.Status != courtOrderNoticeStatusDryRun || resp.User.UserID != testSellerID || resp.Email.HTMLBody != testEmailBody ||
		resp.AlreadySent == nil || *resp.AlreadySent || stringValue(resp.DeliveryStatus) != deliveryStatusPending || stringValue(resp.CommentID) != testCommentID {
		t.Fatalf("unexpected JSON payload: %s", out)
	}
}

func TestCourtOrderNoticeDryRunPlainOutputIncludesEscapedEmail(t *testing.T) {
	testutil.SetupAdmin(t, respondingHandler(respondJSON(t, previewFixture(noticeAccepted))))

	cmd := testutil.Command(newCourtOrderNoticeCmd(), testutil.DryRun(true), testutil.PlainOutput())
	cmd.SetArgs(courtOrderNoticeArgs())
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	fields := strings.Split(strings.TrimSuffix(out, "\n"), "\t")
	wantCompletion := []string{courtOrderNoticeStatusDryRun, "true", deliveryStatusAccepted, testAcceptedAt, testCommentID}
	if len(fields) < len(wantCompletion) || strings.Join(fields[:len(wantCompletion)], "\t") != strings.Join(wantCompletion, "\t") || fields[len(fields)-1] != `<div>\n  <p>Hi,</p>\n  <p>Law firm: Example Law LLP<br>\n  Email: <a href="mailto:notices@example.com">notices@example.com</a></p>\n</div>\n` {
		t.Fatalf("unexpected plain output: %q", out)
	}
}

func TestCourtOrderNoticeDryRunRefusesResponseThatIsNotADryRun(t *testing.T) {
	testutil.SetupAdmin(t, respondingHandler(respondJSON(t, sendFixture(courtOrderNoticeStatusQueued))))

	cmd := testutil.Command(newCourtOrderNoticeCmd(), testutil.DryRun(true), testutil.Quiet(false))
	cmd.SetArgs(courtOrderNoticeArgs())
	var err error
	out := testutil.CaptureStdout(func() { err = cmd.Execute() })

	if err == nil || !strings.Contains(err.Error(), `server answered status "queued" instead of "dry_run"`) {
		t.Fatalf("expected non-dry-run status error, got %v", err)
	}
	if strings.Contains(out, "nothing was sent") {
		t.Fatalf("must not claim nothing was sent:\n%s", out)
	}
}

func TestCourtOrderNoticeSendRequiresConfirmation(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("must not POST without confirmation")
	})

	var stderr bytes.Buffer
	cmd := testutil.Command(newCourtOrderNoticeCmd(), testutil.NoInput(true), testutil.Stderr(&stderr))
	cmd.SetArgs(courtOrderNoticeArgs())

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("expected --yes error, got %v", err)
	}
	if !strings.Contains(stderr.String(), "Admin actor: Test Admin (admin@example.com)") {
		t.Fatalf("expected actor banner before confirmation failure, got stderr %q", stderr.String())
	}
}

func TestCourtOrderNoticeSendQueuesOnceAndPrintsCommentID(t *testing.T) {
	var recorded recordedRequests
	testutil.SetupAdmin(t, recordingHandler(t, &recorded, respondJSON(t, sendFixture(courtOrderNoticeStatusQueued))))

	cmd := testutil.Command(newCourtOrderNoticeCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs(courtOrderNoticeArgs())
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if len(recorded.bodies) != 1 {
		t.Fatalf("got %d requests, want 1", len(recorded.bodies))
	}
	if dryRun, ok := recorded.raw[0]["dry_run"]; !ok || dryRun != false {
		t.Fatalf("send must pass an explicit dry_run false, got %v", recorded.raw[0])
	}
	for _, want := range []string{
		"Queued the court order notice email. The mail service has not accepted it yet.",
		"run the same command with --notice-key " + testNoticeKey + " until it answers already_sent",
		"Status: queued",
		"Delivery status: pending",
		"Seller: Seller One <" + testSellerEmail + ">",
		"User ID: " + testSellerID,
		"Comment ID: " + testCommentID,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	for _, notWant := range []string{"Already sent", "Accepted at", "the mail service accepted"} {
		if strings.Contains(out, notWant) {
			t.Errorf("pending notice must not be reported as accepted (%q):\n%s", notWant, out)
		}
	}
}

func TestCourtOrderNoticeSendReportsAlreadySent(t *testing.T) {
	testutil.SetupAdmin(t, respondingHandler(respondJSON(t, sendFixture(courtOrderNoticeStatusAlreadySent))))

	cmd := testutil.Command(newCourtOrderNoticeCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs(courtOrderNoticeArgs())
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{
		"Already sent: the mail service accepted the court order notice email, so nothing new was queued.",
		"Acceptance by the mail service is not confirmation of delivery to the seller's inbox.",
		"Status: already_sent",
		"Delivery status: accepted",
		"Accepted at: " + testAcceptedAt,
		"Comment ID: " + testCommentID,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Queued the court order notice") {
		t.Fatalf("already_sent must not be reported as queued:\n%s", out)
	}
}

func TestCourtOrderNoticeSendRefusesUnrecognizedResponses(t *testing.T) {
	withFields := func(status string, fields map[string]any) map[string]any {
		resp := sendFixture(status)
		for key, value := range fields {
			if value == nil {
				delete(resp, key)
				continue
			}
			resp[key] = value
		}
		return resp
	}
	queued, alreadySent := courtOrderNoticeStatusQueued, courtOrderNoticeStatusAlreadySent

	tests := []struct {
		name string
		resp map[string]any
		want string
	}{
		{"pending status", sendFixture("pending"), `server answered unexpected status "pending"`},
		{"missing status", sendFixture(""), `server answered unexpected status ""`},
		{"queued without comment", withFields(queued, map[string]any{"comment_id": nil, "delivery_status": nil}), "server answered queued without an admin comment ID"},
		{"queued pending without comment", withFields(queued, map[string]any{"comment_id": nil}), `delivery_status is "pending" without a comment_id`},
		{"queued marked accepted", withFields(queued, completionFixture(noticeAccepted)), "inconsistent queued receipt: already_sent is true"},
		{"queued with accepted_at", withFields(queued, map[string]any{"accepted_at": testAcceptedAt}), "already_sent is false but accepted_at is set"},
		{"queued with accepted delivery", withFields(queued, map[string]any{"delivery_status": deliveryStatusAccepted}), "already_sent is false but delivery_status is accepted"},
		{"queued without delivery status", withFields(queued, map[string]any{"delivery_status": nil}), `a recorded notice has delivery_status ""`},
		{"queued without already_sent", withFields(queued, map[string]any{"already_sent": nil}), "already_sent is missing"},
		{"already_sent still pending", withFields(alreadySent, completionFixture(noticePending)), "inconsistent already_sent receipt: already_sent is false"},
		{"already_sent with pending delivery", withFields(alreadySent, map[string]any{"delivery_status": deliveryStatusPending}), `already_sent is true but delivery_status is "pending"`},
		{"already_sent without accepted_at", withFields(alreadySent, map[string]any{"accepted_at": nil}), `already_sent is true but accepted_at is ""`},
		{"already_sent with malformed accepted_at", withFields(alreadySent, map[string]any{"accepted_at": "yesterday"}), `already_sent is true but accepted_at is "yesterday"`},
		{"already_sent without comment", withFields(alreadySent, map[string]any{"comment_id": nil}), "already_sent is true without a comment_id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.SetupAdmin(t, respondingHandler(respondJSON(t, tt.resp)))

			cmd := testutil.Command(newCourtOrderNoticeCmd(), testutil.Yes(true), testutil.Quiet(false))
			cmd.SetArgs(courtOrderNoticeArgs())
			var err error
			out := testutil.CaptureStdout(func() { err = cmd.Execute() })

			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want error containing %q", err, tt.want)
			}
			if out != "" {
				t.Fatalf("must not print a result for an unrecognized response, got:\n%s", out)
			}
		})
	}
}

func TestCourtOrderNoticeSendUndecodableResponseExplainsRerun(t *testing.T) {
	mistyped := sendFixture(courtOrderNoticeStatusAlreadySent)
	mistyped["already_sent"] = "true"
	testutil.SetupAdmin(t, respondingHandler(respondJSON(t, mistyped)))

	cmd := testutil.Command(newCourtOrderNoticeCmd(), testutil.Yes(true), testutil.Quiet(false))
	cmd.SetArgs(courtOrderNoticeArgs())
	var err error
	out := testutil.CaptureStdout(func() { err = cmd.Execute() })

	if err == nil || !strings.Contains(err.Error(), "The email may have been queued") || !strings.Contains(err.Error(), "--notice-key "+testNoticeKey) {
		t.Fatalf("expected rerun guidance on an undecodable send response, got %v", err)
	}
	if out != "" {
		t.Fatalf("must not print a result for an undecodable response, got:\n%s", out)
	}
}

func TestCourtOrderNoticeSendPlainOutputIncludesDeliveryStatus(t *testing.T) {
	tests := []struct {
		status string
		want   []string
	}{
		{courtOrderNoticeStatusQueued, []string{courtOrderNoticeStatusQueued, deliveryStatusPending, "", testSellerID, testSellerEmail, testCommentID, testCommentCreatedAt}},
		{courtOrderNoticeStatusAlreadySent, []string{courtOrderNoticeStatusAlreadySent, deliveryStatusAccepted, testAcceptedAt, testSellerID, testSellerEmail, testCommentID, testCommentCreatedAt}},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			testutil.SetupAdmin(t, respondingHandler(respondJSON(t, sendFixture(tt.status))))

			cmd := testutil.Command(newCourtOrderNoticeCmd(), testutil.Yes(true), testutil.PlainOutput())
			cmd.SetArgs(courtOrderNoticeArgs())
			out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

			want := strings.Join(tt.want, "\t")
			if strings.TrimSuffix(out, "\n") != want {
				t.Fatalf("got plain output %q, want %q", out, want)
			}
		})
	}
}

func TestCourtOrderNoticeSendQuietPrintsNothing(t *testing.T) {
	testutil.SetupAdmin(t, respondingHandler(respondJSON(t, sendFixture(courtOrderNoticeStatusQueued))))

	cmd := testutil.Command(newCourtOrderNoticeCmd(), testutil.Yes(true))
	cmd.SetArgs(courtOrderNoticeArgs())
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if out != "" {
		t.Fatalf("expected no output with --quiet, got %q", out)
	}
}

func TestCourtOrderNoticeSendRefusalSurfacesServerMessage(t *testing.T) {
	var recorded recordedRequests
	testutil.SetupAdmin(t, recordingHandler(t, &recorded, respondError(http.StatusConflict, "notice_key was already used for a different notice")))

	cmd := testutil.Command(newCourtOrderNoticeCmd(), testutil.Yes(true))
	cmd.SetArgs(courtOrderNoticeArgs())
	err := cmd.Execute()

	var apiErr *api.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 API error, got %v", err)
	}
	if apiErr.Message != "notice_key was already used for a different notice" {
		t.Fatalf("got message %q", apiErr.Message)
	}
	if len(recorded.bodies) != 1 {
		t.Fatalf("got %d requests, want 1", len(recorded.bodies))
	}
}

func TestCourtOrderNoticeSendServerErrorIsNotReplayedAndExplainsRerun(t *testing.T) {
	var recorded recordedRequests
	testutil.SetupAdmin(t, recordingHandler(t, &recorded, respondError(http.StatusInternalServerError, "send failed")))

	cmd := testutil.Command(newCourtOrderNoticeCmd(), testutil.Yes(true))
	cmd.SetArgs(courtOrderNoticeArgs())
	err := cmd.Execute()

	if len(recorded.bodies) != 1 {
		t.Fatalf("got %d requests, want exactly 1 with no automatic replay", len(recorded.bodies))
	}
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500 API error, got %v", err)
	}
	for _, want := range []string{"send failed", "The email may have been queued", "--notice-key " + testNoticeKey, "answers already_sent if the mail service has accepted the notice, and queues the email again if not"} {
		if !strings.Contains(apiErr.Message, want) {
			t.Errorf("error missing %q: %s", want, apiErr.Message)
		}
	}
}

func TestCourtOrderNoticeSendServiceUnavailableLeavesOutcomeUnknown(t *testing.T) {
	tests := []struct {
		name    string
		respond func(http.ResponseWriter)
		want    string
	}{
		{"server message", respondError(http.StatusServiceUnavailable, "the email job could not be queued"), "the email job could not be queued"},
		{"proxy page", func(w http.ResponseWriter) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("<html><body>503 Service Unavailable</body></html>"))
		}, "API error (HTTP 503)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var recorded recordedRequests
			testutil.SetupAdmin(t, recordingHandler(t, &recorded, tt.respond))

			cmd := testutil.Command(newCourtOrderNoticeCmd(), testutil.Yes(true))
			cmd.SetArgs(courtOrderNoticeArgs())
			err := cmd.Execute()

			if len(recorded.bodies) != 1 {
				t.Fatalf("got %d requests, want exactly 1 with no automatic replay", len(recorded.bodies))
			}
			var apiErr *api.APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusServiceUnavailable {
				t.Fatalf("expected 503 API error, got %v", err)
			}
			for _, want := range []string{tt.want, "The email may have been queued", "--notice-key " + testNoticeKey} {
				if !strings.Contains(apiErr.Message, want) {
					t.Errorf("error missing %q: %s", want, apiErr.Message)
				}
			}
		})
	}
}

func TestCourtOrderNoticeEscapesControlCharactersFromServer(t *testing.T) {
	const hostileName = "Seller\x1b[2K\x1b[1AOne"
	preview := previewFixture(noticeNew)
	preview["user"] = map[string]any{"user_id": testSellerID, "email": testSellerEmail, "name": hostileName}
	preview["email"].(map[string]any)["html_body"] = "<p>Hi,</p>\r\n<p>\x1b]0;title\x07Law firm</p>\r\n"
	sent := sendFixture(courtOrderNoticeStatusQueued)
	sent["user"] = preview["user"]

	tests := []struct {
		name    string
		resp    map[string]any
		mutator testutil.OptionsMutator
		want    []string
	}{
		{"dry run", preview, testutil.DryRun(true), []string{`Name: Seller\x1b[2K\x1b[1AOne`, "<p>Hi,</p>\n<p>\\x1b]0;title\\x07Law firm</p>"}},
		{"send", sent, testutil.Yes(true), []string{`Seller: Seller\x1b[2K\x1b[1AOne <` + testSellerEmail + ">"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.SetupAdmin(t, respondingHandler(respondJSON(t, tt.resp)))

			cmd := testutil.Command(newCourtOrderNoticeCmd(), tt.mutator, testutil.Quiet(false))
			cmd.SetArgs(courtOrderNoticeArgs())
			out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

			if strings.ContainsAny(out, "\x1b\x07\r") {
				t.Fatalf("output contains raw control characters: %q", out)
			}
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q:\n%s", want, out)
				}
			}
		})
	}
}

func TestCourtOrderNoticeSendTransportErrorExplainsRerun(t *testing.T) {
	srv := testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {})
	srv.Close()

	cmd := testutil.Command(newCourtOrderNoticeCmd(), testutil.Yes(true))
	cmd.SetArgs(courtOrderNoticeArgs())
	err := cmd.Execute()

	if err == nil || !strings.Contains(err.Error(), "court order notice request failed") || !strings.Contains(err.Error(), "The email may have been queued") {
		t.Fatalf("expected unknown-outcome error, got %v", err)
	}
}

func TestCourtOrderNoticeDryRunErrorIsNotDescribedAsPossiblySent(t *testing.T) {
	testutil.SetupAdmin(t, respondingHandler(respondError(http.StatusInternalServerError, "render failed")))

	cmd := testutil.Command(newCourtOrderNoticeCmd(), testutil.DryRun(true))
	cmd.SetArgs(courtOrderNoticeArgs())
	err := cmd.Execute()

	if err == nil || strings.Contains(err.Error(), "may have been queued") {
		t.Fatalf("dry-run failure must not suggest a send, got %v", err)
	}
}

func TestCourtOrderNoticeRequiresStoredAdminAuthUnlessNonInteractive(t *testing.T) {
	var recorded recordedRequests
	testutil.SetupAdmin(t, recordingHandler(t, &recorded, respondJSON(t, sendFixture(courtOrderNoticeStatusQueued))))
	if err := adminconfig.Delete(); err != nil {
		t.Fatalf("delete admin config: %v", err)
	}
	t.Setenv(adminconfig.EnvAccessToken, "env-admin-token")

	interactive := testutil.Command(newCourtOrderNoticeCmd(), testutil.Yes(true))
	interactive.SetArgs(courtOrderNoticeArgs())
	err := interactive.Execute()
	if !errors.Is(err, adminconfig.ErrNotAuthenticated) || !strings.Contains(err.Error(), "--non-interactive") {
		t.Fatalf("expected stored-auth error naming --non-interactive, got %v", err)
	}
	if len(recorded.bodies) != 0 {
		t.Fatalf("got %d requests before auth was resolved, want 0", len(recorded.bodies))
	}

	nonInteractive := testutil.Command(newCourtOrderNoticeCmd(), testutil.Yes(true), testutil.NonInteractive(true))
	nonInteractive.SetArgs(courtOrderNoticeArgs())
	testutil.MustExecute(t, nonInteractive)
	if recorded.auth != "Bearer env-admin-token" {
		t.Fatalf("got auth %q, want Bearer env-admin-token", recorded.auth)
	}
}

func TestStripeCommandWiresCourtOrderNotice(t *testing.T) {
	for _, sub := range NewStripeCmd().Commands() {
		if sub.Name() == "court-order-notice" {
			return
		}
	}
	t.Fatal("missing court-order-notice subcommand")
}

func respondingHandler(respond func(http.ResponseWriter)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { respond(w) }
}
