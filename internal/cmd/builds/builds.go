// Package builds implements the `gumroad builds` command family.
package builds

import (
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

type build struct {
	ID         string  `json:"id"`
	Number     int     `json:"number"`
	Status     string  `json:"status"`
	ByteSize   int64   `json:"byte_size"`
	PromotedAt *string `json:"promoted_at"`
}

type buildResponse struct {
	Build build `json:"build"`
}

type buildsResponse struct {
	Builds []build `json:"builds"`
}

func buildsPath(product string, rest ...string) string {
	return cmdutil.JoinPath(append([]string{"products", product, "builds"}, rest...)...)
}

// NewBuildsCmd returns the root command for hosted web builds (games, interactive courses).
func NewBuildsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "builds",
		Aliases: []string{"games"},
		Short:   "Deploy web builds (games, interactive courses) to a product",
		Long: "Deploy web builds, such as browser games and interactive courses, to a product.\n\n" +
			"`deploy` uploads a build (a .zip, or a directory that is zipped for you) and records it as a new " +
			"immutable build. A deployed build is not live until it is promoted: pass --promote to deploy, or run " +
			"`gumroad builds promote`. The live build is attached to the product as its download. Promoting an older " +
			"build rolls back.",
		Example: `  gumroad builds init ./my-game --product abc123
  gumroad builds deploy ./dist --product abc123
  gumroad builds deploy ./game.zip --product abc123 --promote
  gumroad builds list --product abc123
  gumroad builds promote <build-id> --product abc123`,
	}
	cmd.AddCommand(newInitCmd())
	cmd.AddCommand(newDeployCmd())
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newPromoteCmd())
	return cmd
}
