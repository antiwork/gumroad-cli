package cmdutil

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/prompt"
)

const dryRunLabel = "Dry run"

type dryRunRequest struct {
	DryRun bool       `json:"dry_run"`
	Method string     `json:"method"`
	Path   string     `json:"path"`
	Params url.Values `json:"params,omitempty"`
}

type dryRunAction struct {
	DryRun bool   `json:"dry_run"`
	Action string `json:"action"`
}

const cancelledLabel = "Cancelled."

func ConfirmAction(opts Options, message string) (bool, error) {
	if opts.DryRun {
		return true, nil
	}
	return prompt.Confirm(message, opts.In(), opts.Err(), opts.Yes, opts.NoInput)
}

func PrintDryRunRequest(opts Options, method, path string, params url.Values) error {
	if method == http.MethodGet {
		return nil
	}

	cloned := redactDryRunParams(params)
	payload := dryRunRequest{
		DryRun: true,
		Method: method,
		Path:   path,
		Params: cloned,
	}

	if opts.UsesJSONOutput() {
		return printDryRunJSON(opts, payload)
	}

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{
			method,
			path,
			formatDryRunParams(cloned),
		}})
	}

	style := opts.Style()
	if err := output.Writeln(opts.Out(), style.Yellow(dryRunLabel)+": "+method+" "+path); err != nil {
		return err
	}
	for _, line := range dryRunParamLines(cloned) {
		if err := output.Writeln(opts.Out(), line); err != nil {
			return err
		}
	}
	return nil
}

func PrintDryRunAction(opts Options, action string) error {
	payload := dryRunAction{
		DryRun: true,
		Action: action,
	}

	if opts.UsesJSONOutput() {
		return printDryRunJSON(opts, payload)
	}

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{action}})
	}

	style := opts.Style()
	return output.Writeln(opts.Out(), style.Yellow(dryRunLabel)+": "+action)
}

func PrintCancelledAction(opts Options, action string) error {
	trimmedAction := strings.TrimSpace(action)
	message := cancelledActionMessage(trimmedAction)

	if opts.UsesJSONOutput() {
		return printMutationPayload(opts, mutationOutput{
			Success:   false,
			Message:   message,
			Cancelled: true,
			Action:    trimmedAction,
		})
	}

	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{"false", message}})
	}

	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	return output.Writeln(opts.Out(), style.Yellow(message))
}

func printDryRunJSON(opts Options, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("could not encode dry-run output: %w", err)
	}
	return output.PrintJSON(opts.Out(), data, opts.JQExpr)
}

func cancelledActionMessage(action string) string {
	action = strings.TrimSuffix(action, ".")
	if action == "" {
		return cancelledLabel
	}
	return "Cancelled: " + action + "."
}

func formatDryRunParams(params url.Values) string {
	if len(params) == 0 {
		return ""
	}

	keys := sortedDryRunKeys(params)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+strings.Join(params[key], ","))
	}
	return strings.Join(parts, "&")
}

func dryRunParamLines(params url.Values) []string {
	if len(params) == 0 {
		return nil
	}

	keys := sortedDryRunKeys(params)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, fmt.Sprintf("%s: %s", key, strings.Join(params[key], ", ")))
	}
	return lines
}

func sortedDryRunKeys(params url.Values) []string {
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func redactDryRunParams(params url.Values) url.Values {
	cloned := CloneValues(params)
	for key, current := range cloned {
		if !shouldRedactDryRunParam(key) {
			continue
		}

		redacted := make([]string, len(current))
		for i := range current {
			redacted[i] = "REDACTED"
		}
		cloned[key] = redacted
	}
	return cloned
}

func shouldRedactDryRunParam(key string) bool {
	normalizedKey := strings.ToLower(strings.TrimSpace(key))

	switch normalizedKey {
	case "access_token", "api_key", "license_key", "password", "secret", "token":
		return true
	default:
		return false
	}
}
