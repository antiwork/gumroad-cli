package emails

import "github.com/spf13/cobra"

const (
	emailAudienceAll       = "all"
	emailAudienceCustomers = "customers"
	emailAudienceFollowers = "followers"
	emailAudienceProduct   = "product"

	emailAPIAudienceAll       = "audience"
	emailAPIAudienceCustomers = "seller"
	emailAPIAudienceFollowers = "follower"

	emailStatePublished = "published"
	emailStateScheduled = "scheduled"
	emailStateDraft     = "draft"
)

type emailRecord struct {
	ID           string `json:"id"`
	Subject      string `json:"subject"`
	AudienceType string `json:"audience_type"`
	ProductID    string `json:"product_id"`
	State        string `json:"state"`
	PublishedAt  string `json:"published_at"`
	ScheduledAt  string `json:"scheduled_at"`
	SendEmails   bool   `json:"send_emails"`
	URL          string `json:"url"`
}

func NewEmailsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "emails",
		Short: "Manage audience emails",
		Long: "Manage Gumroad audience emails.\n\n" +
			"Draft, preview, send, list, view, and delete broadcast emails. " +
			"New emails are created as drafts by default; use `gumroad emails send-preview <id>` to review the preview URL before `gumroad emails send <id>`.",
		Example: `  gumroad emails create --subject "New release" --body ./email.html
  gumroad emails create --subject "Product update" --body ./email.html --audience product --product <id>
  gumroad emails send-preview <id>
  gumroad emails list --state draft --json
  gumroad emails view <id>
  gumroad emails send <id> --yes
  gumroad emails delete <id> --yes`,
	}

	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newSendPreviewCmd())
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newViewCmd())
	cmd.AddCommand(newSendCmd())
	cmd.AddCommand(newDeleteCmd())

	return cmd
}

func emailValidAudienceValues() []string {
	return []string{emailAudienceAll, emailAudienceCustomers, emailAudienceFollowers, emailAudienceProduct}
}

func emailValidStateValues() []string {
	return []string{emailStatePublished, emailStateScheduled, emailStateDraft}
}

func emailValidValue(value string, valid []string) bool {
	for _, item := range valid {
		if value == item {
			return true
		}
	}
	return false
}

func emailBool(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func emailAudienceLabel(item emailRecord) string {
	switch item.AudienceType {
	case emailAPIAudienceAll:
		return emailAudienceAll
	case emailAPIAudienceCustomers:
		return emailAudienceCustomers
	case emailAPIAudienceFollowers:
		return emailAudienceFollowers
	default:
		return item.AudienceType
	}
}

func emailDisplayDate(item emailRecord) string {
	if item.State == emailStateScheduled && item.ScheduledAt != "" {
		return item.ScheduledAt
	}
	if item.PublishedAt != "" {
		return item.PublishedAt
	}
	return item.ScheduledAt
}
