package products

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	var name, currency, description, customPermalink string
	var customSummary, customReceipt, taxonomyID string
	var price, suggestedPrice string
	var maxPurchaseCount int
	var payWhatYouWant bool
	var tags []string
	var files []string
	var fileNames []string
	var fileDescriptions []string
	var keepFileIDs []string
	var removeFileIDs []string
	var replaceFiles bool

	cmd := &cobra.Command{
		Use:   "update <product_id>",
		Short: "Update a product",
		Example: `  gumroad products update <id> --name "New Name"
  gumroad products update <id> --price 15.00 --currency eur
  gumroad products update <id> --tag art --tag digital
  gumroad products update <id> --file ./pack.zip
  gumroad products update <id> --replace-files --keep-file file_123 --file ./new-pack.zip`,
		Args: cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			flags := c.Flags()

			if err := cmdutil.RequireAnyFlagChanged(c,
				"name", "price", "currency", "description",
				"custom-permalink", "custom-summary", "custom-receipt",
				"pay-what-you-want", "suggested-price", "max-purchase-count",
				"taxonomy-id", "tag",
				"file", "file-name", "file-description",
				"keep-file", "remove-file", "replace-files",
			); err != nil {
				return err
			}

			if flags.Changed("name") && name == "" {
				return cmdutil.UsageErrorf(c, "--name cannot be empty")
			}
			if err := cmdutil.RequireNonNegativeIntFlag(c, "max-purchase-count", maxPurchaseCount); err != nil {
				return err
			}
			requestedUploads, err := collectRequestedProductUploads(c, files, fileNames, fileDescriptions)
			if err != nil {
				return err
			}

			currency = strings.ToLower(currency)
			params := url.Values{}
			if flags.Changed("name") {
				params.Set("name", name)
			}
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
			for _, t := range tags {
				params.Add("tags[]", t)
			}

			path := cmdutil.JoinPath("products", args[0])
			fileFlagsChanged := flags.Changed("file") ||
				flags.Changed("file-name") ||
				flags.Changed("file-description") ||
				flags.Changed("keep-file") ||
				flags.Changed("remove-file") ||
				flags.Changed("replace-files")
			if !fileFlagsChanged {
				return cmdutil.RunRequestWithSuccess(opts,
					"Updating product...", "PUT", path, params,
					args[0], "Product "+args[0]+" updated.")
			}

			token, err := config.Token()
			if err != nil {
				return err
			}
			client := cmdutil.NewAPIClient(opts, token)
			existingFiles, err := fetchExistingProductFiles(client, args[0])
			if err != nil {
				return err
			}

			filePlan, err := planProductFileUpdate(c, existingFiles, requestedUploads, keepFileIDs, removeFileIDs, replaceFiles)
			if err != nil {
				return err
			}

			ok, err := confirmProductFileRemoval(opts, args[0], filePlan.Removed)
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.PrintCancelledAction(opts, "update product "+args[0], args[0])
			}

			if fileUpdateNeedsJSONBody(filePlan) {
				if opts.DryRun {
					return renderClearAllFilesDryRun(opts, path, params)
				}
				payload := buildProductUpdateJSONBody(params, []map[string]any{})
				return runProductUpdateJSON(opts, client, path, args[0], payload)
			}

			plannedUploads, err := describeProductUploads(filePlan.Uploads)
			if err != nil {
				return err
			}
			if opts.DryRun {
				dryRunParams := cmdutil.CloneValues(params)
				appendProductFilesParams(dryRunParams, filePlan, placeholderUploadURLs(len(plannedUploads)))
				return cmdutil.PrintDryRunRequest(opts, "PUT", path, dryRunParams)
			}

			uploadedURLs := make([]string, len(plannedUploads))
			for i, planned := range plannedUploads {
				fileURL, err := uploadProductFile(opts, client, planned)
				if err != nil {
					return err
				}
				uploadedURLs[i] = fileURL
			}
			appendProductFilesParams(params, filePlan, uploadedURLs)
			return cmdutil.RunRequestWithSuccess(opts,
				"Updating product...", "PUT", path, params,
				args[0], "Product "+args[0]+" updated.")
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "New product name")
	cmd.Flags().StringVar(&price, "price", "", "New price (e.g. 10, 10.00, 9.99)")
	cmd.Flags().StringVar(&currency, "currency", "", "New price currency (e.g. usd, eur)")
	cmd.Flags().StringVar(&description, "description", "", "New HTML description")
	cmd.Flags().StringVar(&customPermalink, "custom-permalink", "", "New custom URL slug")
	cmd.Flags().StringVar(&customSummary, "custom-summary", "", "New short summary")
	cmd.Flags().StringVar(&customReceipt, "custom-receipt", "", "New custom receipt text")
	cmd.Flags().BoolVar(&payWhatYouWant, "pay-what-you-want", false, "Enable pay-what-you-want pricing")
	cmd.Flags().StringVar(&suggestedPrice, "suggested-price", "", "New suggested price for pay-what-you-want (e.g. 5, 5.00)")
	cmd.Flags().IntVar(&maxPurchaseCount, "max-purchase-count", 0, "New maximum number of purchases")
	cmd.Flags().StringVar(&taxonomyID, "taxonomy-id", "", "New taxonomy/category ID")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "Tag (repeatable, replaces all existing tags)")
	cmd.Flags().StringArrayVar(&files, "file", nil, "Attach a new local file (repeatable)")
	cmd.Flags().StringArrayVar(&fileNames, "file-name", nil, "Display name for the matching --file (repeatable)")
	cmd.Flags().StringArrayVar(&fileDescriptions, "file-description", nil, "Description for the matching --file (repeatable)")
	cmd.Flags().StringArrayVar(&keepFileIDs, "keep-file", nil, "Existing file ID to preserve when using --replace-files (repeatable)")
	cmd.Flags().StringArrayVar(&removeFileIDs, "remove-file", nil, "Existing file ID to remove (repeatable)")
	cmd.Flags().BoolVar(&replaceFiles, "replace-files", false, "Replace the current file set instead of preserving existing files by default")

	return cmd
}
