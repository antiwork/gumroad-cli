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
	"time"
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
	courtOrderNoticePath = "stripe_restraint_notices"

	courtOrderNoticeStatusDryRun      = "dry_run"
	courtOrderNoticeStatusQueued      = "queued"
	courtOrderNoticeStatusAlreadySent = "already_sent"

	deliveryStatusPending  = "pending"
	deliveryStatusAccepted = "accepted"

	maxFirmNameLength  = 200
	maxFirmEmailLength = 254

	courtOrderNoticeConfirmationMessage = "Email the seller who owns %s about Stripe's court order, with %s (%s) as the law firm contact? This queues the email unless the mail service already accepted this notice."
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

type courtOrderNoticeReceipt struct {
	Status         string                 `json:"status"`
	AlreadySent    *bool                  `json:"already_sent"`
	DeliveryStatus *string                `json:"delivery_status"`
	AcceptedAt     *string                `json:"accepted_at"`
	CommentID      *string                `json:"comment_id"`
	User           courtOrderNoticeSeller `json:"user"`
}

type courtOrderNoticePreview struct {
	courtOrderNoticeReceipt
	Email struct {
		From     string   `json:"from"`
		ReplyTo  []string `json:"reply_to"`
		To       []string `json:"to"`
		Subject  string   `json:"subject"`
		HTMLBody string   `json:"html_body"`
	} `json:"email"`
}

type courtOrderNoticeResult struct {
	courtOrderNoticeReceipt
	Comment struct {
		CreatedAt string `json:"created_at"`
	} `json:"comment"`
}

type courtOrderNoticeCompletion struct {
	accepted   bool
	acceptedAt string
	commentID  string
	delivery   string
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
the email from noreply@ with replies going to support@.

The answer is "queued" until the mail service accepts the email, and
"already_sent" after that. Run the same command with the same --notice-key to
check: while the notice is still pending, each call queues the email again so a
failed or lost job is retried; once the mail service has accepted it, the call
answers already_sent and queues nothing. Acceptance by the mail service is not
delivery to the seller's inbox. Do not confirm the notice to Stripe or archive
it until a call answers already_sent. The server cannot rule out a duplicate
email if it fails between the mail service accepting the email and recording
that acceptance.

Pass --dry-run to have the server render the email without sending it. It
prints the matched seller, the notice's current delivery status and the
complete email, and records nothing.`,
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
				return fmt.Errorf("%w. %s", err, unknownOutcomeGuidance(req.NoticeKey))
			}
			return renderCourtOrderNoticeResult(opts, req.NoticeKey, result)
		},
	}

	cmd.Flags().StringVar(&req.StripeAccountID, "stripe-account-id", "", "Stripe connected account ID named in the notice, acct_... (required)")
	cmd.Flags().StringVar(&req.FirmName, "firm-name", "", "Name of the law firm that sent the court order (required)")
	cmd.Flags().StringVar(&req.FirmEmail, "firm-email", "", "Email address of the law firm, shown to the seller (required)")
	cmd.Flags().StringVar(&req.NoticeKey, "notice-key", "", "Unique key for this notice; reuse it to check or retry the same notice (required)")

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
	verify := unknownOutcomeGuidance(req.NoticeKey)

	var apiErr *api.APIError
	if errors.As(err, &apiErr) {
		if apiErr.StatusCode < http.StatusInternalServerError {
			return err
		}
		return &api.APIError{
			StatusCode: apiErr.StatusCode,
			Message:    fmt.Sprintf("court order notice request failed: %s. %s", apiErr.Message, verify),
			Hint:       apiErr.Hint,
		}
	}
	return fmt.Errorf("court order notice request failed: %w. %s", err, verify)
}

func unknownOutcomeGuidance(noticeKey string) string {
	return "The email may have been queued. Run the same command again with --notice-key " + noticeKey +
		": it answers already_sent if the mail service has accepted the notice, and queues the email again if not"
}

func (r courtOrderNoticeReceipt) completion() (courtOrderNoticeCompletion, error) {
	inconsistent := func(reason string) (courtOrderNoticeCompletion, error) {
		return courtOrderNoticeCompletion{}, fmt.Errorf("server answered an inconsistent %s receipt: %s; check the seller's admin comments before confirming the notice", r.Status, reason)
	}

	if r.AlreadySent == nil {
		return inconsistent("already_sent is missing")
	}
	c := courtOrderNoticeCompletion{
		accepted:   *r.AlreadySent,
		acceptedAt: stringValue(r.AcceptedAt),
		commentID:  stringValue(r.CommentID),
		delivery:   stringValue(r.DeliveryStatus),
	}

	if c.accepted {
		switch {
		case c.delivery != deliveryStatusAccepted:
			return inconsistent(fmt.Sprintf("already_sent is true but delivery_status is %q", c.delivery))
		case c.commentID == "":
			return inconsistent("already_sent is true without a comment_id")
		case !isTimestamp(c.acceptedAt):
			return inconsistent(fmt.Sprintf("already_sent is true but accepted_at is %q", c.acceptedAt))
		}
		return c, nil
	}

	switch {
	case c.acceptedAt != "":
		return inconsistent("already_sent is false but accepted_at is set")
	case c.delivery == deliveryStatusAccepted:
		return inconsistent("already_sent is false but delivery_status is accepted")
	case c.commentID != "" && c.delivery != deliveryStatusPending:
		return inconsistent(fmt.Sprintf("a recorded notice has delivery_status %q", c.delivery))
	case c.commentID == "" && c.delivery != "":
		return inconsistent(fmt.Sprintf("delivery_status is %q without a comment_id", c.delivery))
	}
	return c, nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func isTimestamp(value string) bool {
	_, err := time.Parse(time.RFC3339, value)
	return err == nil
}

func renderCourtOrderNoticePreview(opts cmdutil.Options, preview courtOrderNoticePreview) error {
	if preview.Status != courtOrderNoticeStatusDryRun {
		return fmt.Errorf("server answered status %q instead of %q to a dry run; check the seller's admin comments before sending", preview.Status, courtOrderNoticeStatusDryRun)
	}
	completion, err := preview.completion()
	if err != nil {
		return err
	}

	user := preview.User
	email := preview.Email
	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{
			preview.Status,
			strconv.FormatBool(completion.accepted),
			completion.delivery,
			completion.acceptedAt,
			completion.commentID,
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
	delivery := completion.delivery
	if delivery == "" {
		delivery = "none (not recorded yet)"
	}
	lines := []string{
		style.Yellow("Dry run: nothing was sent or recorded."),
		"Delivery status: " + escape(delivery),
	}
	switch {
	case completion.accepted:
		lines = append(lines,
			style.Yellow("The mail service already accepted this notice, so a real call would answer already_sent and queue nothing."),
			"Accepted at: "+escape(completion.acceptedAt),
			"Comment ID: "+escape(completion.commentID),
		)
	case completion.commentID != "":
		lines = append(lines,
			style.Yellow("This notice was recorded but the mail service has not accepted it yet, so a real call would queue the email again."),
			"Comment ID: "+escape(completion.commentID),
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

func renderCourtOrderNoticeResult(opts cmdutil.Options, noticeKey string, result courtOrderNoticeResult) error {
	if result.Status != courtOrderNoticeStatusQueued && result.Status != courtOrderNoticeStatusAlreadySent {
		return fmt.Errorf("server answered unexpected status %q; check the seller's admin comments before retrying", result.Status)
	}
	completion, err := result.completion()
	if err != nil {
		return err
	}
	if completion.commentID == "" {
		return fmt.Errorf("server answered %s without an admin comment ID; check the seller's admin comments before retrying", result.Status)
	}
	if completion.accepted != (result.Status == courtOrderNoticeStatusAlreadySent) {
		return fmt.Errorf("server answered an inconsistent %s receipt: already_sent is %t; check the seller's admin comments before confirming the notice", result.Status, completion.accepted)
	}

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{
			result.Status,
			completion.delivery,
			completion.acceptedAt,
			result.User.UserID,
			result.User.Email,
			completion.commentID,
			result.Comment.CreatedAt,
		}})
	}
	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	var lines []string
	if completion.accepted {
		lines = []string{
			style.Green("Already sent: the mail service accepted the court order notice email, so nothing new was queued."),
			"Acceptance by the mail service is not confirmation of delivery to the seller's inbox.",
			"Status: " + result.Status,
			"Delivery status: " + escape(completion.delivery),
			"Accepted at: " + escape(completion.acceptedAt),
		}
	} else {
		lines = []string{
			style.Yellow("Queued the court order notice email. The mail service has not accepted it yet."),
			"Before confirming to Stripe or archiving the notice, run the same command with --notice-key " + escape(noticeKey) +
				" until it answers already_sent. While the notice is pending, each run queues the email again.",
			"Status: " + result.Status,
			"Delivery status: " + escape(completion.delivery),
		}
	}
	lines = append(lines,
		"Seller: "+escape(result.User.Name)+" <"+escape(result.User.Email)+">",
		"User ID: "+escape(result.User.UserID),
		"Comment ID: "+escape(completion.commentID),
	)
	return output.Writeln(opts.Out(), strings.Join(lines, "\n"))
}
