package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/cmd/admin"
	"github.com/antiwork/gumroad-cli/internal/cmd/auth"
	"github.com/antiwork/gumroad-cli/internal/cmd/categories"
	"github.com/antiwork/gumroad-cli/internal/cmd/completion"
	"github.com/antiwork/gumroad-cli/internal/cmd/customfields"
	"github.com/antiwork/gumroad-cli/internal/cmd/files"
	"github.com/antiwork/gumroad-cli/internal/cmd/licenses"
	"github.com/antiwork/gumroad-cli/internal/cmd/offercodes"
	"github.com/antiwork/gumroad-cli/internal/cmd/payouts"
	"github.com/antiwork/gumroad-cli/internal/cmd/products"
	"github.com/antiwork/gumroad-cli/internal/cmd/sales"
	"github.com/antiwork/gumroad-cli/internal/cmd/skill"
	"github.com/antiwork/gumroad-cli/internal/cmd/subscribers"
	"github.com/antiwork/gumroad-cli/internal/cmd/user"
	"github.com/antiwork/gumroad-cli/internal/cmd/variants"
	"github.com/antiwork/gumroad-cli/internal/cmd/webhooks"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

var (
	Version        = "dev"
	newRootCommand = NewRootCmd
	exitProcess    = os.Exit
	getOSArgs      = func() []string { return os.Args }
)

func NewRootCmd() *cobra.Command {
	opts := cmdutil.DefaultOptions()

	cmd := &cobra.Command{
		Use:   "gumroad",
		Short: "CLI for the Gumroad API",
		Long:  "A command-line interface for the Gumroad API.\nDesigned for humans and AI agents alike.\n\nDocumentation: https://github.com/antiwork/gumroad\nMan pages:     available locally as `man gumroad` after `make install`\nReport issues: https://github.com/antiwork/gumroad-cli/issues",
		Example: `  # Log in with your API token
  gumroad auth login

  # View your account
  gumroad user --json --jq '.user.email'

  # List products and sales
  gumroad products list
  gumroad sales list --json --jq '.sales[0].id'

  # Verify a license without incrementing uses
  echo "$LICENSE_KEY" | gumroad licenses verify --product <id> --no-increment`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			ctx := commandContext(cmd)
			opts.Context = ctx
			opts.Stdin = cmd.InOrStdin()
			opts.Version = Version
			opts.Stdout = cmd.OutOrStdout()
			opts.Stderr = cmd.ErrOrStderr()
			cmd.SetContext(cmdutil.WithOptions(ctx, opts))
			if err := validateOutputFlags(cmd, opts); err != nil {
				return err
			}
			if err := cmdutil.RequireNonNegativeDurationFlag(cmd, "page-delay", opts.PageDelay); err != nil {
				return err
			}
			skill.AutoRefresh(Version)
			return nil
		},
		Version: Version,
	}

	cmd.SetVersionTemplate(fmt.Sprintf("gumroad version %s\n", Version))
	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		if err == nil {
			return nil
		}
		return cmdutil.NewUsageError(c, err.Error())
	})

	// Global flags
	cmd.PersistentFlags().BoolVar(&opts.JSONOutput, "json", false, "Output as JSON")
	cmd.PersistentFlags().BoolVar(&opts.PlainOutput, "plain", false, "Output as plain tab-separated text")
	cmd.PersistentFlags().StringVar(&opts.JQExpr, "jq", "", "Filter JSON output with a jq expression")
	cmd.PersistentFlags().BoolVarP(&opts.Quiet, "quiet", "q", false, "Suppress non-essential output")
	cmd.PersistentFlags().BoolVar(&opts.DryRun, "dry-run", false, "Preview mutating requests without executing them")
	cmd.PersistentFlags().BoolVar(&opts.NoColor, "no-color", false, "Disable color output")
	cmd.PersistentFlags().BoolVar(&opts.NoInput, "no-input", false, "Disable interactive prompts")
	cmd.PersistentFlags().BoolVar(&opts.Yes, "yes", false, "Skip confirmation prompts")
	cmd.PersistentFlags().BoolVar(&opts.NoImage, "no-image", false, "Disable image rendering")
	cmd.PersistentFlags().DurationVar(&opts.PageDelay, "page-delay", 0, "Wait between paginated --all requests (e.g. 200ms, 1s)")
	cmd.PersistentFlags().BoolVar(&opts.Debug, "debug", false, "Enable debug logging to stderr")

	// Subcommands
	cmd.AddCommand(admin.NewAdminCmd())
	cmd.AddCommand(auth.NewAuthCmd())
	cmd.AddCommand(user.NewUserCmd())
	cmd.AddCommand(products.NewProductsCmd())
	cmd.AddCommand(sales.NewSalesCmd())
	cmd.AddCommand(payouts.NewPayoutsCmd())
	cmd.AddCommand(subscribers.NewSubscribersCmd())
	cmd.AddCommand(licenses.NewLicensesCmd())
	cmd.AddCommand(offercodes.NewOfferCodesCmd())
	cmd.AddCommand(categories.NewCategoriesCmd())
	cmd.AddCommand(variants.NewVariantsCmd())
	cmd.AddCommand(customfields.NewCustomFieldsCmd())
	cmd.AddCommand(files.NewFilesCmd())
	cmd.AddCommand(webhooks.NewWebhooksCmd())
	cmd.AddCommand(completion.NewCompletionCmd())
	cmd.AddCommand(skill.NewSkillCmd())
	cmdutil.PropagateExamples(cmd)

	return cmd
}

func commandContext(cmd *cobra.Command) context.Context {
	if cmd == nil {
		return context.Background()
	}
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

func validateOutputFlags(cmd *cobra.Command, opts cmdutil.Options) error {
	if !opts.PlainOutput {
		return nil
	}

	message := plainOutputConflictMessage(opts)
	if message == "" {
		return nil
	}

	return cmdutil.NewUsageError(cmd, message)
}

func plainOutputConflictMessage(opts cmdutil.Options) string {
	switch {
	case opts.JSONOutput && opts.JQExpr != "":
		return "--plain cannot be combined with --json or --jq"
	case opts.JSONOutput:
		return "--plain cannot be combined with --json"
	case opts.JQExpr != "":
		return "--plain cannot be combined with --jq"
	default:
		return ""
	}
}

func Execute() {
	exitProcess(executeRootCommand(os.Stdout, os.Stderr))
}

func executeRootCommand(stdout, stderr io.Writer) int {
	cmd := newRootCommand()
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	err := cmd.Execute()
	if err == nil {
		return 0
	}

	if isUnknownShorthandError(err) {
		if newArgs := insertDoubleDashBeforeArg(getOSArgs()[1:], err); newArgs != nil {
			retryCmd := newRootCommand()
			retryCmd.SetArgs(newArgs)
			retryCmd.SetOut(stdout)
			retryCmd.SetErr(stderr)
			if retryErr := retryCmd.Execute(); retryErr == nil {
				return 0
			} else {
				return exitCodeForCommandError(retryCmd, retryErr)
			}
		}
	}

	return exitCodeForCommandError(cmd, err)
}

func executeCommand(cmd *cobra.Command, stdout, stderr io.Writer) int {
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}

	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	if err := cmd.Execute(); err != nil {
		return exitCodeForCommandError(cmd, err)
	}

	return 0
}

// isUnknownShorthandError checks if the error is from cobra/pflag rejecting
// a dash-prefixed argument as an unknown shorthand flag. This relies on
// pflag's error format: "unknown shorthand flag: 'X' in -XYZ".
func isUnknownShorthandError(err error) bool {
	return strings.Contains(err.Error(), "unknown shorthand flag")
}

// insertDoubleDashBeforeArg finds the arg that caused an "unknown shorthand flag"
// error and inserts "--" before it so cobra treats it as a positional arg.
// Returns nil if the offending arg cannot be identified or doesn't look like
// an encoded ID (to avoid retrying real flag typos like "-z").
func insertDoubleDashBeforeArg(args []string, err error) []string {
	// Error format: "unknown shorthand flag: 'c' in -cGksPcArAUU8j_XTYsrnQ=="
	// The error may include usage text after newlines, so only look at the first line.
	firstLine := err.Error()
	if nl := strings.IndexByte(firstLine, '\n'); nl >= 0 {
		firstLine = firstLine[:nl]
	}
	inIdx := strings.LastIndex(firstLine, " in ")
	if inIdx < 0 {
		return nil
	}
	offending := firstLine[inIdx+4:]

	// Only retry if the offending arg looks like an encoded ID, not a flag typo.
	// Gumroad IDs are base64-encoded and contain digits, '=', '_', '+', or '/'.
	// Flag typos like "-z" or "-json" are purely alphabetic (plus the leading dash).
	if !looksLikeEncodedID(offending) {
		return nil
	}

	// Move the offending arg to the end, preceded by "--", so all flags
	// remain before "--" and are parsed normally.
	result := make([]string, 0, len(args)+1)
	found := false
	for _, arg := range args {
		if !found && arg == offending {
			found = true
			continue // skip it; we'll append it after "--"
		}
		result = append(result, arg)
	}
	if !found {
		return nil
	}
	result = append(result, "--", offending)
	return result
}

// looksLikeEncodedID returns true if s looks like a base64-encoded Gumroad ID
// rather than a mistyped shorthand flag. Encoded IDs typically contain digits,
// '=', '_', '+', or '/' — characters that never appear in flag names. As a
// fallback, any arg longer than 10 characters is treated as an ID since flag
// typos are short.
func looksLikeEncodedID(s string) bool {
	if len(s) > 10 {
		return true
	}
	for _, c := range s {
		if (c >= '0' && c <= '9') || c == '=' || c == '_' || c == '+' || c == '/' {
			return true
		}
	}
	return false
}

func exitCodeForCommandError(cmd *cobra.Command, err error) int {
	if output.IsBrokenPipeError(err) {
		return 0
	}

	if structuredOutputRequested(cmd) {
		writeErr := printStructuredCommandError(cmd.OutOrStdout(), err)
		switch {
		case writeErr == nil:
			return 1
		case output.IsBrokenPipeError(writeErr):
			return 0
		default:
			printHumanCommandError(cmd, writeErr)
			return 1
		}
	}

	printHumanCommandError(cmd, err)
	return 1
}

func printHumanCommandError(cmd *cobra.Command, err error) {
	w := cmd.ErrOrStderr()
	style := output.NewStylerForWriter(w, noColorRequested(cmd))
	fmt.Fprintln(w, style.Red("Error: "+err.Error()))

	// classifyCommandError is the single source of truth for error hints
	// so human and JSON modes stay in sync — particularly for upload
	// errors, whose recovery guidance is not carried by the
	// api.HintedError interface.
	if hint := classifyCommandError(err).Hint; hint != "" {
		fmt.Fprintln(w, style.Dim("Hint: "+hint))
	}
	printUploadRecovery(w, style, err)
}

func noColorRequested(cmd *cobra.Command) bool {
	if noColorRequestedFromCommand(cmd) {
		return true
	}
	return noColorRequestedInArgs(getOSArgs()[1:])
}

func noColorRequestedFromCommand(cmd *cobra.Command) bool {
	opts := cmdutil.OptionsFrom(cmd)
	if opts.NoColor {
		return true
	}
	if cmd == nil {
		return false
	}

	noColor, err := cmd.PersistentFlags().GetBool("no-color")
	if err == nil && noColor {
		return true
	}

	noColor, err = cmd.Flags().GetBool("no-color")
	return err == nil && noColor
}

func noColorRequestedInArgs(args []string) bool {
	for _, arg := range args {
		if arg == "--no-color" {
			return true
		}
		if !strings.HasPrefix(arg, "--no-color=") {
			continue
		}
		value, err := strconv.ParseBool(strings.TrimPrefix(arg, "--no-color="))
		if err == nil && value {
			return true
		}
	}

	return false
}
