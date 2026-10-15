package stripe

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

const (
	courtOrderNoticePath = "stripe_court_order_notices"

	courtOrderNoticeStatusDryRun      = "dry_run"
	courtOrderNoticeStatusQueued      = "queued"
	courtOrderNoticeStatusAlreadySent = "already_sent"

	maxFirmNameLength  = 200
	maxFirmEmailLength = 254

	courtOrderNoticeConfirmationMessage = "Email the seller who owns %s about Stripe's court order, with %s (%s) as the law firm contact? This records an admin comment and queues the email."
)

var (
	stripeAccountIDFormat = regexp.MustCompile(`^acct_[A-Za-z0-9]{1,100}$`)
	noticeKeyFormat       = regexp.MustCompile(`^[\x21-\x7E]{1,200}$`)
)

type courtOrderNoticeRequest struct {
	StripeAccountID string `json:"stripe_account_id"`
	FirmName        string `json:"firm_name"`
	FirmEmail       string `json:"firm_email"`
	NoticeKey       string `json:"notice_key"`
	DryRun          bool   `json:"dry_run"`
}

type courtOrderNoticeSeller struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Name   string `json:"name"`
}

type courtOrderNoticePreview struct {
	Status      string                 `json:"status"`
	AlreadySent bool                   `json:"already_sent"`
	User        courtOrderNoticeSeller `json:"user"`
	CommentID   string                 `json:"comment_id"`
	Email       struct {
		From     string   `json:"from"`
		ReplyTo  []string `json:"reply_to"`
		To       []string `json:"to"`
		Subject  string   `json:"subject"`
		HTMLBody string   `json:"html_body"`
	} `json:"email"`
}

type courtOrderNoticeResult struct {
	Status    string                 `json:"status"`
	User      courtOrderNoticeSeller `json:"user"`
	CommentID string                 `json:"comment_id"`
	Comment   struct {
		CreatedAt string `json:"created_at"`
	} `json:"comment"`
}

func newCourtOrderNoticeCmd() *cobra.Command {
	var req courtOrderNoticeRequest

	cmd := &cobra.Command{
		Use:   "court-order-notice",
		Short: "Tell a seller that Stripe paused their account under a court order",
		Long: `Email the seller who owns a Stripe connected account that Stripe paused the
account under a court order, with the law firm's contact.

The server matches the one live Gumroad account with an email address that uses
--stripe-account-id, records an admin comment keyed by --notice-key, and queues
the email from noreply@ with replies going to support@. The comment is the
dedup record: running the command again with the same --notice-key answers
already_sent and queues nothing. A queued email has not been delivered yet;
delivery runs in a background job.

Pass --dry-run to have the server render the email without sending it. It
prints the matched seller and the complete email, and records nothing.`,
		Example: `  gumroad admin stripe court-order-notice --stripe-account-id acct_1Example --firm-name "Example Law LLP" --firm-email notices@example.com --notice-key notice-1 --dry-run
  gumroad admin stripe court-order-notice --stripe-account-id acct_1Example --firm-name "Example Law LLP" --firm-email notices@example.com --notice-key notice-1 --yes`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if err := normalizeCourtOrderNoticeRequest(c, &req); err != nil {
				return err
			}
			req.DryRun = opts.DryRun

			info, err := admincmd.ResolveMutationToken(opts)
			if err != nil {
				return err
			}
			if err := admincmd.WriteActorBanner(opts, info); err != nil {
				return err
			}

			if !req.DryRun {
				ok, err := cmdutil.ConfirmAction(opts, fmt.Sprintf(courtOrderNoticeConfirmationMessage, req.StripeAccountID, req.FirmName, req.FirmEmail))
				if err != nil {
					return err
				}
				if !ok {
					return cmdutil.PrintCancelledAction(opts, "court order notice for "+req.StripeAccountID, req.StripeAccountID)
				}
			}

			data, err := postCourtOrderNotice(opts, admincmd.NewAPIClient(opts, info.Value), req)
			if err != nil {
				if req.DryRun {
					return err
				}
				return wrapCourtOrderNoticeSendError(req, err)
			}
			if opts.UsesJSONOutput() {
				return cmdutil.PrintJSONResponse(opts, data)
			}

			if req.DryRun {
				preview, err := cmdutil.DecodeJSON[courtOrderNoticePreview](data)
				if err != nil {
					return err
				}
				return renderCourtOrderNoticePreview(opts, preview)
			}
			result, err := cmdutil.DecodeJSON[courtOrderNoticeResult](data)
			if err != nil {
				return err
			}
			return renderCourtOrderNoticeResult(opts, result)
		},
	}

	cmd.Flags().StringVar(&req.StripeAccountID, "stripe-account-id", "", "Stripe connected account ID named in the notice, acct_... (required)")
	cmd.Flags().StringVar(&req.FirmName, "firm-name", "", "Name of the law firm that sent the court order (required)")
	cmd.Flags().StringVar(&req.FirmEmail, "firm-email", "", "Email address of the law firm, shown to the seller (required)")
	cmd.Flags().StringVar(&req.NoticeKey, "notice-key", "", "Unique key for this notice; repeat calls with the same key send nothing (required)")

	return cmd
}

func normalizeCourtOrderNoticeRequest(c *cobra.Command, req *courtOrderNoticeRequest) error {
	req.StripeAccountID = strings.TrimSpace(req.StripeAccountID)
	req.FirmName = strings.TrimSpace(req.FirmName)
	req.FirmEmail = strings.TrimSpace(req.FirmEmail)

	switch {
	case req.StripeAccountID == "":
		return cmdutil.MissingFlagError(c, "--stripe-account-id")
	case req.FirmName == "":
		return cmdutil.MissingFlagError(c, "--firm-name")
	case req.FirmEmail == "":
		return cmdutil.MissingFlagError(c, "--firm-email")
	case req.NoticeKey == "":
		return cmdutil.MissingFlagError(c, "--notice-key")
	case !stripeAccountIDFormat.MatchString(req.StripeAccountID):
		return cmdutil.UsageErrorf(c, "--stripe-account-id must be a Stripe account ID (acct_...)")
	case utf8.RuneCountInString(req.FirmName) > maxFirmNameLength:
		return cmdutil.UsageErrorf(c, "--firm-name must be at most %d characters", maxFirmNameLength)
	case strings.IndexFunc(req.FirmName, isUnsafeRune) >= 0:
		return cmdutil.UsageErrorf(c, "--firm-name cannot contain line breaks or control characters")
	case !isBareEmailAddress(req.FirmEmail):
		return cmdutil.UsageErrorf(c, "--firm-email must be a single email address")
	case !noticeKeyFormat.MatchString(req.NoticeKey):
		return cmdutil.UsageErrorf(c, "--notice-key must be 1-200 printable ASCII characters without spaces")
	}
	return nil
}

func isUnsafeRune(r rune) bool {
	return unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r)
}

func isBareEmailAddress(value string) bool {
	if len(value) > maxFirmEmailLength {
		return false
	}
	address, err := mail.ParseAddress(value)
	return err == nil && address.Name == "" && address.Address == value
}

func postCourtOrderNotice(opts cmdutil.Options, client *adminapi.Client, req courtOrderNoticeRequest) (json.RawMessage, error) {
	if cmdutil.ShouldShowSpinner(opts) {
		message := "Queueing court order notice..."
		if req.DryRun {
			message = "Rendering court order notice..."
		}
		sp := output.NewSpinnerTo(message, opts.Err())
		sp.Start()
		defer sp.Stop()
	}
	return client.PostJSON(courtOrderNoticePath, req)
}

func wrapCourtOrderNoticeSendError(req courtOrderNoticeRequest, err error) error {
	verify := "The email may have been queued. Run the same command again with --notice-key " + req.NoticeKey +
		": if the notice was recorded the server answers already_sent and queues nothing"

	var apiErr *api.APIError
	if errors.As(err, &apiErr) {
		if apiErr.StatusCode < http.StatusInternalServerError {
			return err
		}
		if apiErr.StatusCode == http.StatusServiceUnavailable && hasServerMessage(apiErr) {
			return &api.APIError{
				StatusCode: apiErr.StatusCode,
				Message: fmt.Sprintf("court order notice was not queued: %s. Run the same command again with --notice-key %s to retry",
					apiErr.Message, req.NoticeKey),
				Hint: apiErr.Hint,
			}
		}
		return &api.APIError{
			StatusCode: apiErr.StatusCode,
			Message:    fmt.Sprintf("court order notice request failed: %s. %s", apiErr.Message, verify),
			Hint:       apiErr.Hint,
		}
	}
	return fmt.Errorf("court order notice request failed: %w. %s", err, verify)
}

// The api client falls back to this generic text when the error body has no
// JSON message, for example a 503 page from a proxy rather than from Rails.
func hasServerMessage(apiErr *api.APIError) bool {
	return apiErr.Message != fmt.Sprintf("API error (HTTP %d)", apiErr.StatusCode)
}

func renderCourtOrderNoticePreview(opts cmdutil.Options, preview courtOrderNoticePreview) error {
	if preview.Status != courtOrderNoticeStatusDryRun {
		return fmt.Errorf("server answered status %q instead of %q to a dry run; check the seller's admin comments before sending", preview.Status, courtOrderNoticeStatusDryRun)
	}

	user := preview.User
	email := preview.Email
	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{
			preview.Status,
			strconv.FormatBool(preview.AlreadySent),
			preview.CommentID,
			user.UserID,
			user.Email,
			user.Name,
			email.From,
			strings.Join(email.ReplyTo, ","),
			strings.Join(email.To, ","),
			email.Subject,
			email.HTMLBody,
		}})
	}

	style := opts.Style()
	lines := []string{
		style.Yellow("Dry run: nothing was sent or recorded."),
	}
	if preview.AlreadySent {
		lines = append(lines,
			style.Yellow("This notice key was already used, so a real call would answer already_sent and queue nothing."),
			"Comment ID: "+escape(preview.CommentID),
		)
	}
	lines = append(lines,
		"",
		style.Bold("Seller"),
		"User ID: "+escape(user.UserID),
		"Name: "+escape(user.Name),
		"Email: "+escape(user.Email),
		"",
		style.Bold("Email"),
		"From: "+escape(email.From),
		"Reply-To: "+escape(strings.Join(email.ReplyTo, ", ")),
		"To: "+escape(strings.Join(email.To, ", ")),
		"Subject: "+escape(email.Subject),
		"",
	)
	lines = append(lines, escapeLines(email.HTMLBody)...)
	return output.Writeln(opts.Out(), strings.Join(lines, "\n"))
}

func escape(value string) string {
	return output.EscapePlainField(value)
}

func escapeLines(value string) []string {
	normalized := strings.TrimRight(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	lines := strings.Split(normalized, "\n")
	for i, line := range lines {
		lines[i] = escape(line)
	}
	return lines
}

func renderCourtOrderNoticeResult(opts cmdutil.Options, result courtOrderNoticeResult) error {
	var message string
	switch result.Status {
	case courtOrderNoticeStatusQueued:
		message = "Queued the court order notice email. It is not delivered yet: a background job sends it."
	case courtOrderNoticeStatusAlreadySent:
		message = "Already queued: an earlier call with this notice key queued the email, so nothing new was queued."
	default:
		return fmt.Errorf("server answered unexpected status %q; check the seller's admin comments before retrying", result.Status)
	}
	if result.CommentID == "" {
		return fmt.Errorf("server answered %s without an admin comment ID; check the seller's admin comments before retrying", result.Status)
	}

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{result.Status, result.User.UserID, result.User.Email, result.CommentID, result.Comment.CreatedAt}})
	}
	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	headline := style.Green(message)
	if result.Status == courtOrderNoticeStatusAlreadySent {
		headline = style.Yellow(message)
	}
	return output.Writeln(opts.Out(), strings.Join([]string{
		headline,
		"Status: " + result.Status,
		"Seller: " + escape(result.User.Name) + " <" + escape(result.User.Email) + ">",
		"User ID: " + escape(result.User.UserID),
		"Comment ID: " + escape(result.CommentID),
	}, "\n"))
}
