package builds

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

const (
	starterIndexFile  = "index.html"
	starterDeployFile = "deploy.sh"
	// productEnvVar is read by a deploy.sh written without --product, so the
	// starter is usable before the seller knows the product it belongs to.
	productEnvVar = "GUMROAD_PRODUCT"
)

// A playable page, not a placeholder: it proves the deploy round trip on the
// first run and shows where a real build's index.html goes.
const starterIndexHTML = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>My web build</title>
    <style>
      html, body { margin: 0; height: 100%; overflow: hidden; background: #111; }
      canvas { display: block; width: 100%; height: 100%; }
    </style>
  </head>
  <body>
    <canvas id="game"></canvas>
    <script>
      // Replace this file with your build's entry point; index.html must stay at the top level.
      const MOVEMENT_SPEED = 6;
      const SQUARE_SIZE = 48;
      const canvas = document.getElementById("game");
      const ctx = canvas.getContext("2d");
      const SPEED = 6;
      const SQUARE_SIZE = 48;
      const keys = new Set();
      let x = 0, y = 0;
      addEventListener("keydown", (event) => keys.add(event.key));
      addEventListener("keyup", (event) => keys.delete(event.key));
      function frame() {
        const width = (canvas.width = innerWidth), height = (canvas.height = innerHeight);
        if (keys.has("ArrowLeft")) x -= MOVEMENT_SPEED;
        if (keys.has("ArrowRight")) x += MOVEMENT_SPEED;
        if (keys.has("ArrowUp")) y -= MOVEMENT_SPEED;
        if (keys.has("ArrowDown")) y += MOVEMENT_SPEED;
        x = Math.max(0, Math.min(width - SQUARE_SIZE, x));
        y = Math.max(0, Math.min(height - SQUARE_SIZE, y));
        ctx.fillStyle = "#111";
        ctx.fillRect(0, 0, width, height);
        ctx.fillStyle = "#f4c430";
        ctx.fillRect(x, y, SQUARE_SIZE, SQUARE_SIZE);
        requestAnimationFrame(frame);
      }
      frame();
    </script>
  </body>
</html>
`

type starterFile struct {
	name    string
	content string
	mode    os.FileMode
}

func newInitCmd() *cobra.Command {
	var product string
	var force bool
	c := &cobra.Command{
		Use:   "init [dir]",
		Short: "Create a starter build you can deploy",
		Long: "Create a directory holding a playable index.html and a deploy.sh that uploads this folder as a new build and promotes it.\n\n" +
			"The starter runs with no other setup so the first deploy is a real one: `./deploy.sh` (or `gumroad builds deploy . --product <id> --promote`). " +
			"Replace index.html and add your assets next to it — index.html must stay at the top level, and files or folders starting with a dot are skipped on deploy.\n\n" +
			"Files are never overwritten: init fails if either already exists unless --force is given.",
		Args: cobra.MaximumNArgs(1),
		Example: `  gumroad builds init ./my-game --product abc123
  gumroad builds init ./my-course
  gumroad builds deploy ./my-game --product abc123 --promote`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			files := []starterFile{
				{name: starterIndexFile, content: starterIndexHTML, mode: 0o644},
				{name: starterDeployFile, content: deployScript(product), mode: 0o755},
			}
			paths := make([]string, len(files))
			for i, file := range files {
				paths[i] = filepath.Join(dir, file.name)
			}
			if opts.DryRun {
				return cmdutil.PrintDryRunAction(opts, "create "+strings.Join(paths, " and "))
			}
			if err := writeStarter(dir, files, force); err != nil {
				return err
			}
			return printStarterResult(opts, dir, paths, product)
		},
	}
	c.Flags().StringVar(&product, "product", "", "Product ID or permalink to deploy to (written into deploy.sh)")
	c.Flags().BoolVar(&force, "force", false, "Overwrite index.html and deploy.sh if they already exist")
	return c
}

// deployScript writes a script that deploys the folder it sits in, so the file
// keeps working when the folder is moved or the product is renamed.
func deployScript(product string) string {
	resolve := `product="${` + productEnvVar + `:?set ` + productEnvVar + ` to the product id, or pass --product to gumroad builds init}"`
	if product != "" {
		resolve = "product=" + shellQuote(product)
	}
	return `#!/usr/bin/env bash
# Deploys this folder as a new build and makes it the live one. Requires ` + "`gumroad login`" + `.
set -euo pipefail
cd "$(dirname "$0")"
` + resolve + `
gumroad builds deploy . --product "$product" --promote "$@"
`
}

// shellQuote single-quotes a value for the generated script; the product is a
// bare id or permalink, but the script is the seller's to edit afterwards.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// writeStarter checks every destination before writing any of them, so a
// refusal never leaves a half-created starter behind. Destinations are read
// with Lstat and created with O_EXCL: writing through a link would land the
// file outside the folder the seller named.
func writeStarter(dir string, files []starterFile, force bool) error {
	for _, file := range files {
		path := filepath.Join(dir, file.name)
		info, err := os.Lstat(path)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return cmdutil.InvalidInputErrorf("%s is a symbolic link (remove it, or init a different directory)", path)
		}
		if !force {
			return cmdutil.InvalidInputErrorf("%s already exists (use --force to overwrite, or init an empty directory)", path)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, file := range files {
		flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
		if force {
			flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		}
		path := filepath.Join(dir, file.name)
		handle, err := os.OpenFile(path, flags, file.mode)
		if err != nil {
			return err
		}
		_, writeErr := handle.WriteString(file.content)
		closeErr := handle.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
		// The create mode is filtered by the umask, so set it explicitly:
		// deploy.sh has to be executable for the starter to run.
		if err := os.Chmod(path, file.mode); err != nil {
			return err
		}
	}
	return nil
}

func printStarterResult(opts cmdutil.Options, dir string, paths []string, product string) error {
	productArg := "<product-id>"
	if product != "" {
		productArg = shellQuote(product)
	}
	next := strings.Join([]string{"gumroad builds deploy", shellQuote(dir), "--product", productArg, "--promote"}, " ")
	if opts.UsesJSONOutput() {
		payload, err := json.Marshal(map[string]any{"success": true, "files": paths, "next": next})
		if err != nil {
			return err
		}
		return cmdutil.PrintJSONResponse(opts, payload)
	}
	if opts.PlainOutput {
		rows := make([][]string, len(paths))
		for i, path := range paths {
			rows[i] = []string{path}
		}
		return output.PrintPlain(opts.Out(), rows)
	}
	if err := cmdutil.PrintSuccess(opts, "Created "+strings.Join(paths, " and ")+"."); err != nil {
		return err
	}
	return cmdutil.PrintInfo(opts, "Next: "+next)
}
