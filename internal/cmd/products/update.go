package products

import (
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	var name, currency, description, customPermalink string
	var customSummary, customReceipt, category, taxonomyID string
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
	var coverImage, thumbnail string
	var previewImages []string
	var customHTML string

	cmd := &cobra.Command{
		Use:   "update <product_id>",
		Short: "Update a product",
		Example: `  gumroad products update <id> --name "New Name"
  gumroad products update <id> --price 15.00 --currency eur
  gumroad products update <id> --category design/ui-and-web/figma
  gumroad products update <id> --tag art --tag digital
  gumroad products update <id> --cover-image ./cover.jpg
  gumroad products update <id> --preview-image ./gallery-1.jpg --preview-image ./gallery-2.jpg
  gumroad products update <id> --file ./pack.zip
  gumroad products update <id> --custom-html ./landing.html
  gumroad products update <id> --custom-html ''
  gumroad products update <id> --replace-files --keep-file file_123 --file ./new-pack.zip`,
		Args: cmdutil.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			flags := c.Flags()

			if err := cmdutil.RequireAnyFlagChanged(c,
				"name", "price", "currency", "description",
				"custom-permalink", "custom-summary", "custom-receipt",
				"pay-what-you-want", "suggested-price", "max-purchase-count",
				"category", "taxonomy-id", "tag",
				"file", "file-name", "file-description",
				"keep-file", "remove-file", "replace-files",
				"cover-image", "preview-image", "thumbnail",
				"custom-html",
			); err != nil {
				return err
			}

			if flags.Changed("name") && name == "" {
				return cmdutil.UsageErrorf(c, "--name cannot be empty")
			}
			if err := cmdutil.RequireNonNegativeIntFlag(c, "max-purchase-count", maxPurchaseCount); err != nil {
				return err
			}
			if err := validateProductCategoryFlags(c); err != nil {
				return err
			}
			requestedUploads, err := collectRequestedProductUploads(c, files, fileNames, fileDescriptions)
			if err != nil {
				return err
			}
			if err := validateProductMediaFlagPaths(c, coverImage, previewImages, thumbnail); err != nil {
				return err
			}
			media, err := describeProductMedia(collectProductMedia(coverImage, previewImages, thumbnail))
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
			if flags.Changed("category") {
				params.Set("category", category)
			}
			if flags.Changed("taxonomy-id") {
				params.Set("taxonomy_id", taxonomyID)
			}
			if flags.Changed("custom-html") {
				if customHTML == "" {
					params.Set("custom_html", "")
				} else {
					html, err := os.ReadFile(customHTML)
					if err != nil {
						return cmdutil.UsageErrorf(c, "--custom-html: cannot read %s: %v", customHTML, err)
					}
					params.Set("custom_html", string(html))
				}
			}
			for _, t := range tags {
				params.Add("tags[]", t)
			}

			path := cmdutil.JoinPath("products", args[0])
			productFieldsChanged := productUpdateFieldFlagsChanged(c)
			fileFlagsChanged := flags.Changed("file") ||
				flags.Changed("file-name") ||
				flags.Changed("file-description") ||
				flags.Changed("keep-file") ||
				flags.Changed("remove-file") ||
				flags.Changed("replace-files")
			if !fileFlagsChanged {
				if len(media) > 0 {
					if opts.DryRun {
						requests := productMediaDryRunRequests(args[0], media)
						if !productFieldsChanged {
							if opts.UsesJSONOutput() {
								return renderProductUpdateMediaOnlyDryRunJSON(opts, media, requests)
							}
							return renderStandaloneProductMediaDryRun(opts, media, requests)
						}
						return renderProductUpdateDryRunWithMedia(opts, path, productFileUpdatePlan{}, nil, buildProductJSONBody(params, nil), media, requests)
					}
					token, err := config.Token()
					if err != nil {
						return err
					}
					client := cmdutil.NewAPIClient(opts, token)
					var data []byte
					completedAction := ""
					if productFieldsChanged {
						data, err = runProductUpdateFormData(opts, client, path, params)
						if err != nil {
							return err
						}
						completedAction = "product update"
					}
					mediaResults, err := uploadAndAttachProductMedia(opts, client, args[0], media, completedAction)
					if err != nil {
						return err
					}
					if !productFieldsChanged {
						data, err = productMediaOnlyUpdateResult(args[0])
						if err != nil {
							return err
						}
					}
					data, err = mergeProductMediaResult(data, mediaResults)
					if err != nil {
						return err
					}
					return cmdutil.PrintResourceSuccess(opts, data, "", "Product "+args[0]+" updated.")
				}
				if opts.DryRun && opts.UsesJSONOutput() {
					return renderProductUpdateDryRun(opts, path, productFileUpdatePlan{}, nil, buildProductJSONBody(params, nil))
				}
				return cmdutil.RunRequestWithResource(opts,
					"Updating product...", "PUT", path, params,
					"", "Product "+args[0]+" updated.")
			}

			selections, err := validateProductFileSelections(c, keepFileIDs, removeFileIDs, replaceFiles)
			if err != nil {
				return err
			}
			plannedUploads, err := describeProductUploads(requestedUploads)
			if err != nil {
				return err
			}

			token, err := config.Token()
			if err != nil {
				return err
			}
			client := cmdutil.NewAPIClient(opts, token)
			existingState, err := fetchExistingProductFileState(client, args[0])
			if err != nil {
				return err
			}

			filePlan, err := planProductFileUpdate(c, existingState.Files, requestedUploads, selections, replaceFiles)
			if err != nil {
				return err
			}

			fileRefs, err := newRichContentFileRefs(len(plannedUploads))
			if err != nil {
				return err
			}
			if len(fileRefs) > 0 && productUsesPerVariantRichContent(existingState) {
				return cmdutil.UsageErrorf(c,
					"product %s uses per-variant content, so product-level --file cannot update rich_content; use gumroad variants update <variant_id> --product %s --category <cat_id> --file <path>",
					args[0], args[0])
			}

			richContent, includeRichContent, err := buildProductUpdateRichContent(c, existingState.RichContent, filePlan, fileRefs)
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

			if opts.DryRun {
				payload := buildProductUpdateJSONBody(params, filePlan, placeholderUploadURLs(len(plannedUploads)), fileRefs, richContent, includeRichContent)
				return renderProductUpdateDryRunWithMedia(opts, path, filePlan, plannedUploads, payload, media, productMediaDryRunRequests(args[0], media))
			}

			uploadedURLs, err := uploadBatch(opts, client, productBatchUploadInputs(plannedUploads))
			if err != nil {
				return err
			}
			payload := buildProductUpdateJSONBody(params, filePlan, uploadedURLs, fileRefs, richContent, includeRichContent)
			data, err := runProductUpdateJSONData(opts, client, path, payload, uploadedURLs)
			if err != nil {
				return err
			}
			mediaResults, err := uploadAndAttachProductMedia(opts, client, args[0], media, "product update")
			if err != nil {
				return err
			}
			if len(mediaResults) > 0 {
				data, err = mergeProductMediaResult(data, mediaResults)
				if err != nil {
					return err
				}
			}
			return cmdutil.PrintResourceSuccess(opts, data, "", "Product "+args[0]+" updated.")
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
	cmd.Flags().StringVar(&category, "category", "", "New product category path (for example: design/ui-and-web/figma)")
	cmd.Flags().StringVar(&taxonomyID, "taxonomy-id", "", "New numeric taxonomy/category ID")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "Tag (repeatable, replaces all existing tags)")
	cmd.Flags().StringArrayVar(&files, "file", nil, "Attach a new local file (repeatable)")
	cmd.Flags().StringArrayVar(&fileNames, "file-name", nil, "Display name for the matching --file (repeatable)")
	cmd.Flags().StringArrayVar(&fileDescriptions, "file-description", nil, "Description for the matching --file (repeatable)")
	cmd.Flags().StringArrayVar(&keepFileIDs, "keep-file", nil, "Existing file ID to preserve when using --replace-files (repeatable)")
	cmd.Flags().StringArrayVar(&removeFileIDs, "remove-file", nil, "Existing file ID to remove (repeatable)")
	cmd.Flags().BoolVar(&replaceFiles, "replace-files", false, "Replace the current file set instead of preserving existing files by default")
	cmd.Flags().StringVar(&coverImage, "cover-image", "", "Local JPEG, PNG, or GIF cover image to upload")
	cmd.Flags().StringArrayVar(&previewImages, "preview-image", nil, "Additional local JPEG, PNG, or GIF preview image to upload as a product cover (repeatable)")
	cmd.Flags().StringVar(&thumbnail, "thumbnail", "", "Local JPEG, PNG, or GIF thumbnail image to upload")
	cmd.Flags().StringVar(&customHTML, "custom-html", "", "Path to an HTML file for the product's custom landing page (empty string clears it)")

	return cmd
}

func productUpdateFieldFlagsChanged(cmd *cobra.Command) bool {
	for _, flag := range []string{
		"name", "price", "currency", "description",
		"custom-permalink", "custom-summary", "custom-receipt",
		"pay-what-you-want", "suggested-price", "max-purchase-count",
		"category", "taxonomy-id", "tag", "custom-html",
	} {
		if cmd.Flags().Changed(flag) {
			return true
		}
	}
	return false
}
