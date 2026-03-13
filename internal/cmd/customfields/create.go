package customfields

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

var customFieldTypePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

func newCreateCmd() *cobra.Command {
	var product, name, fieldType string
	var required bool

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a custom field",
		Args:  cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			if product == "" {
				return cmdutil.MissingFlagError(c, "--product")
			}
			if name == "" {
				return cmdutil.MissingFlagError(c, "--name")
			}
			normalizedType, err := normalizeCustomFieldType(c, fieldType)
			if err != nil {
				return err
			}

			params := url.Values{}
			params.Set("name", name)
			if required {
				params.Set("required", "true")
			}
			params.Set("type", normalizedType)

			return cmdutil.RunRequestWithSuccess(opts, "Creating custom field...", "POST", cmdutil.JoinPath("products", product, "custom_fields"), params, "Custom field created.")
		},
	}

	cmd.Flags().StringVar(&product, "product", "", "Product ID (required)")
	cmd.Flags().StringVar(&name, "name", "", "Field name (required)")
	cmd.Flags().BoolVar(&required, "required", false, "Make field required")
	cmd.Flags().StringVar(&fieldType, "type", "text", "Field type (default: text)")

	return cmd
}

func normalizeCustomFieldType(cmd *cobra.Command, value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return "", cmdutil.UsageErrorf(cmd, "--type cannot be empty")
	}
	if !customFieldTypePattern.MatchString(normalized) {
		return "", cmdutil.UsageErrorf(cmd, "--type must use lowercase letters, numbers, hyphens, or underscores")
	}
	return normalized, nil
}
