package products

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

// One row of the product editor's Additional details.
type additionalDetail struct {
	Name  string
	Value string
}

// The keys setAdditionalDetailParams writes, read back when a JSON body has to carry the rows.
var additionalDetailParamPattern = regexp.MustCompile(`^additional_details\[(\d+)\]\[(name|value)\]$`)

func registerAdditionalDetailFlags(cmd *cobra.Command, additionalDetails *[]string) {
	cmd.Flags().StringArrayVar(additionalDetails, "additional-detail", nil, "Additional detail as \"Name: value\" (repeatable; replaces the whole list, and \"\" clears it)")
}

// parseAdditionalDetailFlags reads the repeatable --additional-detail values ("Pages: 40").
// The flag replaces the whole list, so a single empty value is the clear signal. The editor keeps a
// row by its name, so a value with no name is refused here as the API refuses it.
func parseAdditionalDetailFlags(c *cobra.Command, values []string) ([]additionalDetail, error) {
	rows := make([]additionalDetail, 0, len(values))
	for _, value := range values {
		name, val, _ := strings.Cut(value, ":")
		name, val = strings.TrimSpace(name), strings.TrimSpace(val)
		if name == "" {
			if val != "" {
				return nil, cmdutil.UsageErrorf(c, "--additional-detail %q needs a name, e.g. \"Pages: 40\"", value)
			}
			continue
		}
		rows = append(rows, additionalDetail{Name: name, Value: val})
	}
	return rows, nil
}

// setAdditionalDetailParams writes each row under its own index, Rails' collection encoding
// (additional_details[0][name]). The index is in the key, so the rows arrive in order however the
// client emits them. An empty list still sends one empty row, because the param replaces the whole
// list and that row is what drops the existing ones.
func setAdditionalDetailParams(params url.Values, rows []additionalDetail) {
	if len(rows) == 0 {
		rows = []additionalDetail{{}}
	}
	for i, row := range rows {
		params.Set(fmt.Sprintf("additional_details[%d][name]", i), row.Name)
		params.Set(fmt.Sprintf("additional_details[%d][value]", i), row.Value)
	}
}
