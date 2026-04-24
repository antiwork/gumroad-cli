package licenses

import (
	"fmt"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/prompt"
	"github.com/spf13/cobra"
)

const (
	licenseKeyPrompt = "license key"
	licenseKeyHint   = "Pipe it via stdin or pass --key."
)

type licenseResponse struct {
	License  license     `json:"license"`
	Purchase purchase    `json:"purchase"`
	Uses     api.JSONInt `json:"uses"`
}

type license struct {
	Email       string      `json:"email"`
	ProductID   string      `json:"product_id"`
	ProductName string      `json:"product_name"`
	PurchaseID  string      `json:"purchase_id"`
	Uses        api.JSONInt `json:"uses"`
	Enabled     *bool       `json:"enabled"`
	Disabled    *bool       `json:"disabled"`
	CreatedAt   string      `json:"created_at"`
}

type purchase struct {
	ID           string      `json:"id"`
	Email        string      `json:"email"`
	ProductID    string      `json:"product_id"`
	ProductName  string      `json:"product_name"`
	ProductAlias string      `json:"link_name"`
	Uses         api.JSONInt `json:"uses"`
}

type lookupRequest struct {
	LicenseKey string `json:"license_key"`
}

func newLookupCmd() *cobra.Command {
	var key string

	cmd := &cobra.Command{
		Use:   "lookup",
		Short: "Look up a license key",
		Args:  cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			resolvedKey, err := resolveLicenseKey(c, opts, key)
			if err != nil {
				return err
			}

			return admincmd.RunPostJSONDecoded[licenseResponse](opts, "Fetching license...", "/licenses/lookup", lookupRequest{LicenseKey: resolvedKey}, func(resp licenseResponse) error {
				return renderLicense(opts, resp)
			})
		},
	}

	cmd.Flags().StringVar(&key, "key", "", "License key")

	return cmd
}

func resolveLicenseKey(cmd *cobra.Command, opts cmdutil.Options, key string) (string, error) {
	if cmd != nil && cmd.Flags().Changed("key") {
		key = strings.TrimSpace(key)
		if key == "" {
			return "", cmdutil.UsageErrorf(cmd, "--key cannot be empty")
		}
		return key, nil
	}

	key, err := prompt.SecretInput(licenseKeyPrompt, licenseKeyPrompt, opts.In(), opts.Err(), opts.NoInput, licenseKeyHint)
	if err != nil {
		return "", err
	}
	if key == "" {
		return "", cmdutil.UsageErrorf(cmd, "license key cannot be empty. %s", licenseKeyHint)
	}
	return key, nil
}

func renderLicense(opts cmdutil.Options, resp licenseResponse) error {
	email := firstNonEmpty(resp.License.Email, resp.Purchase.Email)
	product := firstNonEmpty(resp.License.ProductName, resp.Purchase.ProductName, resp.Purchase.ProductAlias, resp.License.ProductID, resp.Purchase.ProductID)
	purchaseID := firstNonEmpty(resp.License.PurchaseID, resp.Purchase.ID)
	uses := resp.License.Uses
	if uses == 0 {
		uses = resp.Purchase.Uses
	}
	if uses == 0 {
		uses = resp.Uses
	}
	status := licenseStatus(resp.License)

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{
			{purchaseID, email, product, fmt.Sprintf("%d", uses), status},
		})
	}

	style := opts.Style()
	headlineFromPurchaseID := false
	headline := product
	if headline == "" && purchaseID != "" {
		headline = purchaseID
		headlineFromPurchaseID = true
	}
	if headline == "" {
		headline = "License"
	}
	if err := output.Writeln(opts.Out(), style.Bold(headline)); err != nil {
		return err
	}
	if purchaseID != "" && !headlineFromPurchaseID {
		if err := output.Writef(opts.Out(), "Purchase ID: %s\n", purchaseID); err != nil {
			return err
		}
	}
	if email != "" {
		if err := output.Writef(opts.Out(), "Buyer: %s\n", email); err != nil {
			return err
		}
	}
	if err := output.Writef(opts.Out(), "Uses: %d\n", uses); err != nil {
		return err
	}
	if status != "" {
		return output.Writef(opts.Out(), "Status: %s\n", status)
	}
	return nil
}

func licenseStatus(l license) string {
	switch {
	case l.Enabled != nil && *l.Enabled:
		return "enabled"
	case l.Disabled != nil && *l.Disabled:
		return "disabled"
	case l.Enabled != nil:
		return "disabled"
	case l.Disabled != nil:
		return "enabled"
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
