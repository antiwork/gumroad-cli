package workflows

import (
	"fmt"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/spf13/cobra"
)

const (
	workflowAudienceAll       = "all"
	workflowAudienceCustomers = "customers"
	workflowAudienceFollowers = "followers"

	workflowAPIAudienceAll       = "audience"
	workflowAPIAudienceCustomers = "seller"
	workflowAPIAudienceFollowers = "follower"
)

type workflowRecord struct {
	ID                  string                `json:"id"`
	Name                string                `json:"name"`
	AudienceType        string                `json:"audience_type"`
	Trigger             string                `json:"trigger"`
	ProductID           string                `json:"product_id"`
	VariantID           string                `json:"variant_id"`
	State               string                `json:"state"`
	PublishedAt         string                `json:"published_at"`
	FirstPublishedAt    string                `json:"first_published_at"`
	SendToPastCustomers bool                  `json:"send_to_past_customers"`
	EmailsCount         api.JSONInt           `json:"emails_count"`
	Emails              []workflowEmailRecord `json:"emails"`
}

type workflowEmailRecord struct {
	ID         string        `json:"id"`
	Subject    string        `json:"subject"`
	State      string        `json:"state"`
	SendEmails bool          `json:"send_emails"`
	Delay      workflowDelay `json:"delay"`
	SentCount  api.JSONInt   `json:"sent_count"`
	OpenCount  api.JSONInt   `json:"open_count"`
	OpenRate   *float64      `json:"open_rate"`
	ClickCount api.JSONInt   `json:"click_count"`
	ClickRate  *float64      `json:"click_rate"`
}

type workflowDelay struct {
	Amount api.JSONInt `json:"amount"`
	Unit   string      `json:"unit"`
}

func NewWorkflowsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflows",
		Short: "Manage email workflows",
		Long: "Manage Gumroad email workflows.\n\n" +
			"List workflows and view their email steps. Add a step or update selected step fields without changing the workflow publication state. " +
			"A new step on a published workflow can schedule recipients. A delay change can reschedule recipients.",
		Example: `  gumroad workflows list
  gumroad workflows list --json
  gumroad workflows view <id>
  gumroad workflows view <id> --json --jq '.workflow.emails[] | {subject, click_rate}'
  gumroad workflows add-email <workflow-id> --subject "Week four" --body ./email.html --delay "4 weeks" --yes
  gumroad workflows update-email <workflow-id> <email-id> --body ./email.html`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newViewCmd())
	cmd.AddCommand(newAddEmailCmd())
	cmd.AddCommand(newUpdateEmailCmd())

	return cmd
}

func workflowAudienceLabel(audienceType string) string {
	switch audienceType {
	case workflowAPIAudienceAll:
		return workflowAudienceAll
	case workflowAPIAudienceCustomers:
		return workflowAudienceCustomers
	case workflowAPIAudienceFollowers:
		return workflowAudienceFollowers
	default:
		return audienceType
	}
}

func workflowBool(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func workflowDelayLabel(delay workflowDelay) string {
	if delay.Unit == "" {
		return ""
	}
	unit := delay.Unit
	if delay.Amount != 1 {
		unit += "s"
	}
	return fmt.Sprintf("%d %s", delay.Amount, unit)
}

func workflowRateLabel(rate *float64) string {
	if rate == nil {
		return ""
	}
	return fmt.Sprintf("%.1f%%", *rate)
}
