package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/prompt"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type commandErrorEnvelope struct {
	Success bool               `json:"success"`
	Error   commandErrorDetail `json:"error"`
}

type commandErrorDetail struct {
	Type       string `json:"type"`
	Code       string `json:"code,omitempty"`
	Message    string `json:"message"`
	Hint       string `json:"hint,omitempty"`
	StatusCode int    `json:"status_code,omitempty"`
}

func printStructuredCommandError(w io.Writer, err error) error {
	payload := commandErrorEnvelope{
		Success: false,
		Error:   classifyCommandError(err),
	}

	data, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return fmt.Errorf("could not encode error output: %w", marshalErr)
	}

	return output.PrintJSON(w, data, "")
}

func classifyCommandError(err error) commandErrorDetail {
	if err == nil {
		return commandErrorDetail{
			Type:    "internal_error",
			Code:    "unknown_error",
			Message: "unknown error",
		}
	}

	var usageErr *cmdutil.UsageError
	var apiErr *api.APIError
	switch {
	case errors.As(err, &usageErr):
		return invalidInputErrorDetail(usageErr.Error())
	case errors.As(err, &apiErr):
		return commandErrorDetail{
			Type:       "api_error",
			Code:       apiErrorCode(apiErr.StatusCode),
			Message:    apiErr.Error(),
			Hint:       apiErr.GetHint(),
			StatusCode: apiErr.StatusCode,
		}
	case errors.Is(err, config.ErrNotAuthenticated), errors.Is(err, api.ErrNotAuthenticated):
		hint := api.HintRunAuthLogin
		if strings.Contains(err.Error(), "gumroad auth login") {
			hint = ""
		}
		return commandErrorDetail{
			Type:    "auth_error",
			Code:    "not_authenticated",
			Message: err.Error(),
			Hint:    hint,
		}
	case errors.Is(err, prompt.ErrConfirmationNoInput), errors.Is(err, prompt.ErrConfirmationNonInteractive):
		return invalidInputErrorDetail(err.Error())
	case isLikelyJQError(err):
		return commandErrorDetail{
			Type:    "usage_error",
			Code:    "invalid_jq",
			Message: err.Error(),
		}
	case isLikelyUsageError(err):
		return invalidInputErrorDetail(err.Error())
	default:
		return commandErrorDetail{
			Type:    "internal_error",
			Code:    "internal_error",
			Message: err.Error(),
		}
	}
}

func invalidInputErrorDetail(message string) commandErrorDetail {
	return commandErrorDetail{
		Type:    "usage_error",
		Code:    "invalid_input",
		Message: message,
	}
}

func apiErrorCode(statusCode int) string {
	switch statusCode {
	case 401:
		return "not_authenticated"
	case 403:
		return "access_denied"
	case 404:
		return "not_found"
	case 429:
		return "rate_limited"
	default:
		return "api_error"
	}
}

func structuredOutputRequested(cmd *cobra.Command) bool {
	if structuredOutputRequestedFromCommand(cmd) {
		return true
	}
	return structuredOutputRequestedInArgs(os.Args[1:])
}

func structuredOutputRequestedFromCommand(cmd *cobra.Command) bool {
	opts := cmdutil.OptionsFrom(cmd)
	if opts.UsesJSONOutput() {
		return true
	}
	if cmd == nil {
		return false
	}

	return structuredOutputRequestedInFlagSet(cmd.Flags()) ||
		structuredOutputRequestedInFlagSet(cmd.PersistentFlags())
}

func structuredOutputRequestedInFlagSet(flags *pflag.FlagSet) bool {
	if flags == nil {
		return false
	}

	jsonOutput, err := flags.GetBool("json")
	if err == nil && jsonOutput {
		return true
	}

	jqExpr, err := flags.GetString("jq")
	return err == nil && jqExpr != ""
}

func structuredOutputRequestedInArgs(args []string) bool {
	for _, arg := range args {
		switch {
		case arg == "--json":
			return true
		case strings.HasPrefix(arg, "--json="):
			value, err := strconv.ParseBool(strings.TrimPrefix(arg, "--json="))
			if err == nil && value {
				return true
			}
		case arg == "--jq":
			return true
		case strings.HasPrefix(arg, "--jq="):
			return true
		}
	}

	return false
}

func isLikelyUsageError(err error) bool {
	if err == nil {
		return false
	}

	message := err.Error()
	return strings.HasPrefix(message, "unknown command ") ||
		strings.HasPrefix(message, "unknown flag: ") ||
		strings.HasPrefix(message, "unknown shorthand flag: ") ||
		strings.Contains(message, " requires at least") ||
		strings.Contains(message, "flag needs an argument")
}

func isLikelyJQError(err error) bool {
	if err == nil {
		return false
	}

	message := err.Error()
	return strings.HasPrefix(message, "invalid jq expression:") ||
		strings.HasPrefix(message, "jq error:")
}
