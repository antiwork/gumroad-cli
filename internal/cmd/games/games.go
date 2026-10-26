// Package games implements the `gumroad games` command family.
package games

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
	Build build `json:"game_build"`
}

type buildsResponse struct {
	Builds []build `json:"game_builds"`
}

func buildsPath(product string, rest ...string) string {
	return cmdutil.JoinPath(append([]string{"products", product, "games", "builds"}, rest...)...)
}

// NewGamesCmd returns the root command for browser game builds.
func NewGamesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "games",
		Short: "Deploy browser game builds to a product",
		Long: "Deploy browser game builds to a product.\n\n" +
			"`deploy` uploads a build (a .zip, or a directory that is zipped for you) and records it as a new " +
			"immutable build. A deployed build is not live until it is promoted: pass --promote to deploy, or run " +
			"`gumroad games promote`. The live build is attached to the product as its download. Promoting an older " +
			"build rolls back.",
		Example: `  gumroad games deploy ./dist --product abc123
  gumroad games deploy ./game.zip --product abc123 --promote
  gumroad games list --product abc123
  gumroad games promote <build-id> --product abc123`,
	}
	cmd.AddCommand(newDeployCmd())
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newPromoteCmd())
	return cmd
}
