package products

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/upload"
	"github.com/antiwork/gumroad-cli/internal/uploadui"
	"github.com/spf13/cobra"
)

func sortedKeys(m map[string]bool) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

type createProductResponse struct {
	Product struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		FormattedPrice string `json:"formatted_price"`
	} `json:"product"`
}

var validProductTypes = map[string]bool{
	"digital": true, "course": true, "ebook": true,
	"membership": true, "bundle": true, "coffee": true,
	"call": true, "commission": true,
}

var validSubscriptionDurations = map[string]bool{
	"monthly": true, "quarterly": true, "biannually": true,
	"yearly": true, "every_two_years": true,
}

// s3HTTPClientForTesting redirects multipart PUTs at a test TLS server. Tests
// in this package must not use t.Parallel while mutating this hook.
var s3HTTPClientForTesting *http.Client

type createUploadInput struct {
	Path        string
	DisplayName string
	Description string
	Plan        upload.Plan
}

type dryRunCreateUpload struct {
	Action    string `json:"action"`
	Path      string `json:"path"`
	Filename  string `json:"filename"`
	Size      int64  `json:"size"`
	PartSize  int64  `json:"part_size"`
	PartCount int    `json:"part_count"`
}

type dryRunCreateRequest struct {
	Method string     `json:"method"`
	Path   string     `json:"path"`
	Params url.Values `json:"params,omitempty"`
}

type dryRunCreatePayload struct {
	DryRun  bool                 `json:"dry_run"`
	Uploads []dryRunCreateUpload `json:"uploads"`
	Request dryRunCreateRequest  `json:"request"`
}

func newCreateCmd() *cobra.Command {
	var name, nativeType, currency, description, customPermalink string
	var customSummary, customReceipt, subscriptionDuration, taxonomyID string
	var price, suggestedPrice string
	var maxPurchaseCount int
	var payWhatYouWant bool
	var tags []string
	var files, fileNames, fileDescriptions []string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new product (as draft)",
		Example: `  gumroad products create --name "Art Pack" --price 10.00
  gumroad products create --name "Art Pack" --file ./pack.zip --file-name "Art Pack.zip"
  gumroad products create --name "Newsletter" --type membership --subscription-duration monthly
  gumroad products create --name "E-Book" --type ebook --price 5 --tag art --tag digital`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			flags := c.Flags()

			if name == "" {
				return cmdutil.MissingFlagError(c, "--name")
			}

			if !validProductTypes[nativeType] {
				return cmdutil.UsageErrorf(c, "invalid --type %q; must be one of: %s", nativeType, sortedKeys(validProductTypes))
			}

			if flags.Changed("subscription-duration") {
				if nativeType != "membership" {
					return cmdutil.UsageErrorf(c, "--subscription-duration can only be used with --type membership")
				}
				if !validSubscriptionDurations[subscriptionDuration] {
					return cmdutil.UsageErrorf(c, "invalid --subscription-duration %q; must be one of: %s", subscriptionDuration, sortedKeys(validSubscriptionDurations))
				}
			}

			if err := cmdutil.RequireNonNegativeIntFlag(c, "max-purchase-count", maxPurchaseCount); err != nil {
				return err
			}

			plannedUploads, err := planCreateUploads(c, files, fileNames, fileDescriptions)
			if err != nil {
				return err
			}

			params := url.Values{}
			params.Set("name", name)
			params.Set("native_type", nativeType)
			currency = strings.ToLower(currency)
			if flags.Changed("price") {
				cents, err := cmdutil.ParseMoney("price", price, "price", currency)
				if err != nil {
					return cmdutil.UsageErrorf(c, "%s", err.Error())
				}
				params.Set("price", strconv.Itoa(cents))
			}
			if flags.Changed("currency") {
				params.Set("price_currency_type", currency)
			}
			if flags.Changed("description") {
				params.Set("description", description)
			}
			if flags.Changed("custom-permalink") {
				params.Set("custom_permalink", customPermalink)
			}
			if flags.Changed("custom-summary") {
				params.Set("custom_summary", customSummary)
			}
			if flags.Changed("custom-receipt") {
				params.Set("custom_receipt", customReceipt)
			}
			if flags.Changed("pay-what-you-want") {
				params.Set("customizable_price", strconv.FormatBool(payWhatYouWant))
			}
			if flags.Changed("suggested-price") {
				cents, err := cmdutil.ParseMoney("suggested-price", suggestedPrice, "suggested price", currency)
				if err != nil {
					return cmdutil.UsageErrorf(c, "%s", err.Error())
				}
				params.Set("suggested_price_cents", strconv.Itoa(cents))
			}
			if flags.Changed("max-purchase-count") {
				params.Set("max_purchase_count", strconv.Itoa(maxPurchaseCount))
			}
			if flags.Changed("taxonomy-id") {
				params.Set("taxonomy_id", taxonomyID)
			}
			if flags.Changed("subscription-duration") {
				params.Set("subscription_duration", subscriptionDuration)
			}
			for _, t := range tags {
				params.Add("tags[]", t)
			}

			if len(plannedUploads) > 0 {
				fileURLs := make([]string, len(plannedUploads))
				if opts.DryRun {
					for i := range plannedUploads {
						fileURLs[i] = dryRunFilePlaceholder(i)
					}
					appendCreateUploadParams(params, plannedUploads, fileURLs)
					return renderCreateDryRun(opts, plannedUploads, params)
				}

				token, err := config.Token()
				if err != nil {
					return err
				}
				client := cmdutil.NewAPIClient(opts, token)
				for i, planned := range plannedUploads {
					statusLabel := planned.Plan.Filename
					if len(plannedUploads) > 1 {
						statusLabel = fmt.Sprintf("%s (%d/%d)", planned.Plan.Filename, i+1, len(plannedUploads))
					}
					fileURLs[i], err = uploadui.UploadFile(opts, client, planned.Path, planned.Plan, s3HTTPClientForTesting, statusLabel)
					if err != nil {
						return err
					}
				}
				appendCreateUploadParams(params, plannedUploads, fileURLs)
			}

			return cmdutil.RunRequestDecoded[createProductResponse](opts,
				"Creating product...", "POST", "/products", params,
				func(resp createProductResponse) error {
					p := resp.Product
					if opts.PlainOutput {
						return output.PrintPlain(opts.Out(), [][]string{
							{p.ID, p.Name, p.FormattedPrice},
						})
					}
					if opts.Quiet {
						return nil
					}
					s := opts.Style()
					if err := output.Writef(opts.Out(), "%s %s (%s)\n",
						s.Bold("Created draft product:"), p.Name, s.Dim(p.ID)); err != nil {
						return err
					}
					return output.Writef(opts.Out(), "\n%s gumroad products publish %s\n",
						s.Dim("Publish with:"), p.ID)
				})
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Product name (required)")
	cmd.Flags().StringVar(&nativeType, "type", "digital", "Product type (digital, course, ebook, membership, bundle, coffee, call, commission)")
	cmd.Flags().StringVar(&price, "price", "", "Price (e.g. 10, 10.00, 9.99)")
	cmd.Flags().StringVar(&currency, "currency", "", "Price currency (e.g. usd, eur)")
	cmd.Flags().StringVar(&description, "description", "", "HTML description")
	cmd.Flags().StringVar(&customPermalink, "custom-permalink", "", "Custom URL slug")
	cmd.Flags().StringVar(&customSummary, "custom-summary", "", "Short summary")
	cmd.Flags().StringVar(&customReceipt, "custom-receipt", "", "Custom receipt text")
	cmd.Flags().BoolVar(&payWhatYouWant, "pay-what-you-want", false, "Enable pay-what-you-want pricing")
	cmd.Flags().StringVar(&suggestedPrice, "suggested-price", "", "Suggested price for pay-what-you-want (e.g. 5, 5.00)")
	cmd.Flags().IntVar(&maxPurchaseCount, "max-purchase-count", 0, "Maximum number of purchases (inventory limit)")
	cmd.Flags().StringVar(&taxonomyID, "taxonomy-id", "", "Taxonomy/category ID")
	cmd.Flags().StringVar(&subscriptionDuration, "subscription-duration", "", "Subscription duration (membership only: monthly, quarterly, biannually, yearly, every_two_years)")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "Tag (repeatable)")
	cmd.Flags().StringArrayVar(&files, "file", nil, "Attach a local file to the new product (repeatable)")
	cmd.Flags().StringArrayVar(&fileNames, "file-name", nil, "Display filename for the matching --file upload (repeatable; use an empty string to skip a slot)")
	cmd.Flags().StringArrayVar(&fileDescriptions, "file-description", nil, "Description for the matching --file upload (repeatable; use an empty string to skip a slot)")

	return cmd
}

func planCreateUploads(c *cobra.Command, files, fileNames, fileDescriptions []string) ([]createUploadInput, error) {
	if len(files) == 0 {
		if len(fileNames) > 0 {
			return nil, cmdutil.UsageErrorf(c, "--file-name requires at least one --file")
		}
		if len(fileDescriptions) > 0 {
			return nil, cmdutil.UsageErrorf(c, "--file-description requires at least one --file")
		}
		return nil, nil
	}

	alignedNames, err := alignCreateUploadValues(c, "--file-name", fileNames, len(files))
	if err != nil {
		return nil, err
	}
	alignedDescriptions, err := alignCreateUploadValues(c, "--file-description", fileDescriptions, len(files))
	if err != nil {
		return nil, err
	}

	uploads := make([]createUploadInput, 0, len(files))
	for i, path := range files {
		displayName := strings.TrimSpace(alignedNames[i])
		plan, err := upload.Describe(path, upload.Options{Filename: displayName})
		if err != nil {
			return nil, err
		}
		uploads = append(uploads, createUploadInput{
			Path:        path,
			DisplayName: displayName,
			Description: alignedDescriptions[i],
			Plan:        plan,
		})
	}
	return uploads, nil
}

func alignCreateUploadValues(c *cobra.Command, flagName string, values []string, count int) ([]string, error) {
	switch len(values) {
	case 0:
		return make([]string, count), nil
	case count:
		aligned := make([]string, count)
		copy(aligned, values)
		return aligned, nil
	default:
		return nil, cmdutil.UsageErrorf(c, "%s must be provided zero times or exactly once per --file (got %d values for %d files)", flagName, len(values), count)
	}
}

func appendCreateUploadParams(params url.Values, uploads []createUploadInput, fileURLs []string) {
	for i, planned := range uploads {
		params.Set(fmt.Sprintf("files[%d][url]", i), fileURLs[i])
		if planned.DisplayName != "" {
			params.Set(fmt.Sprintf("files[%d][display_name]", i), planned.DisplayName)
		}
		if planned.Description != "" {
			params.Set(fmt.Sprintf("files[%d][description]", i), planned.Description)
		}
	}
}

func dryRunFilePlaceholder(i int) string {
	return fmt.Sprintf("<uploaded:file:%d>", i)
}

func renderCreateDryRun(opts cmdutil.Options, uploads []createUploadInput, params url.Values) error {
	if opts.UsesJSONOutput() {
		payload := dryRunCreatePayload{
			DryRun:  true,
			Uploads: make([]dryRunCreateUpload, 0, len(uploads)),
			Request: dryRunCreateRequest{
				Method: "POST",
				Path:   "/products",
				Params: cmdutil.CloneValues(params),
			},
		}
		for _, planned := range uploads {
			payload.Uploads = append(payload.Uploads, dryRunCreateUpload{
				Action:    "upload",
				Path:      planned.Plan.Path,
				Filename:  planned.Plan.Filename,
				Size:      planned.Plan.Size,
				PartSize:  planned.Plan.PartSize,
				PartCount: planned.Plan.PartCount,
			})
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("could not encode dry-run output: %w", err)
		}
		return output.PrintJSON(opts.Out(), data, opts.JQExpr)
	}

	if opts.PlainOutput {
		for _, planned := range uploads {
			if err := output.PrintPlain(opts.Out(), [][]string{{
				"upload",
				planned.Plan.Path,
				planned.Plan.Filename,
				strconv.FormatInt(planned.Plan.Size, 10),
				strconv.Itoa(planned.Plan.PartCount),
			}}); err != nil {
				return err
			}
		}
		return cmdutil.PrintDryRunRequest(opts, "POST", "/products", params)
	}

	for _, planned := range uploads {
		if err := renderCreateUploadDryRun(opts, planned.Plan); err != nil {
			return err
		}
	}
	return cmdutil.PrintDryRunRequest(opts, "POST", "/products", params)
}

func renderCreateUploadDryRun(opts cmdutil.Options, plan upload.Plan) error {
	style := opts.Style()
	if err := output.Writeln(opts.Out(), style.Yellow("Dry run")+": upload "+plan.Path); err != nil {
		return err
	}
	if err := output.Writef(opts.Out(), "Filename: %s\n", plan.Filename); err != nil {
		return err
	}
	parts := "1 part"
	if plan.PartCount != 1 {
		parts = fmt.Sprintf("%d parts", plan.PartCount)
	}
	return output.Writef(opts.Out(), "Size: %s (%s)\n", uploadui.HumanBytes(plan.Size), parts)
}
