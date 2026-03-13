package licenses

import (
	"strings"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/prompt"
	"github.com/spf13/cobra"
)

const (
	licenseKeyPrompt = "license key"
	licenseKeyHint   = "Pipe it via stdin or pass --key (deprecated)."
)

func addLicenseKeyFlag(cmd *cobra.Command, key *string) {
	cmd.Flags().StringVar(key, "key", "", "License key (deprecated; prefer stdin or interactive prompt)")
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
