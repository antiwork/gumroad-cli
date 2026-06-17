package email

import "github.com/spf13/cobra"

const (
	emailAudienceAll       = "all"
	emailAudienceCustomers = "customers"
	emailAudienceFollowers = "followers"
	emailAudienceProduct   = "product"

	emailStatePublished = "published"
	emailStateScheduled = "scheduled"
	emailStateDraft     = "draft"
)

type emailInstallment struct {
	ID              string `json:"id"`
	Subject         string `json:"subject"`
	Message         string `json:"message"`
	AudienceType    string `json:"audience_type"`
	ProductID       string `json:"product_id"`
	State           string `json:"state"`
	PublishedAt     string `json:"published_at"`
	ScheduledAt     string `json:"scheduled_at"`
	SendEmails      bool   `json:"send_emails"`
	ShownOnProfile  bool   `json:"shown_on_profile"`
	AudienceCount   int    `json:"audience_count"`
	RecipientsCount int    `json:"recipients_count"`
	URL             string `json:"url"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

func NewEmailCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "email",
		Short: "Manage audience emails",
		Long: "Manage Gumroad audience emails.\n\n" +
			"Create, preview, list, view, send, and delete broadcast emails. " +
			"New emails are created as drafts by default; use `gumroad email preview <id>` to review the preview URL before `gumroad email send <id>`.",
		Example: `  gumroad email create --subject "New release" --body ./email.html
  gumroad email create --subject "Product update" --body ./email.html --audience product --product <id>
  gumroad email preview <id>
  gumroad email list --state draft --json
  gumroad email view <id>
  gumroad email send <id> --yes
  gumroad email delete <id> --yes`,
	}

	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newPreviewCmd())
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

func emailDisplayDate(item emailInstallment) string {
	if item.State == emailStateScheduled && item.ScheduledAt != "" {
		return item.ScheduledAt
	}
	if item.PublishedAt != "" {
		return item.PublishedAt
	}
	return item.ScheduledAt
}
