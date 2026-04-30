package cmdutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/api"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/output"
)

type ClientRunner func(*api.Client) (json.RawMessage, error)

type spinner interface {
	Start()
	Stop()
}

var newSpinner = func(message string, w io.Writer) spinner {
	return output.NewSpinnerTo(message, w)
}

// ShouldShowSpinner reports whether transient spinner output should be shown.
// Debug mode disables the spinner so structured stderr diagnostics stay readable.
func ShouldShowSpinner(opts Options) bool {
	return !opts.Quiet && !opts.DebugEnabled()
}

// NewAPIClient builds an API client that respects the command's context,
// version, debug setting, and stderr writer.
func NewAPIClient(opts Options, token string) *api.Client {
	client := api.NewClientWithContext(opts.Context, token, opts.Version, opts.DebugEnabled())
	client.SetDebugWriter(opts.Err())
	return client
}

// DecodeJSON decodes a Gumroad JSON response into a typed value with the
// shared human-facing parse error wrapper.
func DecodeJSON[T any](data json.RawMessage) (T, error) {
	var decoded T
	if err := json.Unmarshal(data, &decoded); err != nil {
		return decoded, fmt.Errorf("could not parse response: %w", err)
	}
	return decoded, nil
}

// Run executes a caller-provided authenticated client operation and preserves
// the shared JSON/JQ fast-path.
func Run(opts Options, spinnerMessage string, run ClientRunner, render func(json.RawMessage) error) error {
	data, err := runAuthenticatedData(opts, spinnerMessage, run)
	if err != nil {
		return err
	}
	if opts.UsesJSONOutput() {
		return PrintJSONResponse(opts, data)
	}
	return render(data)
}

// RunDecoded executes an authenticated client operation and decodes the
// response for human/plain rendering while preserving the shared JSON/JQ
// fast-path.
func RunDecoded[T any](opts Options, spinnerMessage string, run ClientRunner, render func(T) error) error {
	data, err := runAuthenticatedData(opts, spinnerMessage, run)
	if err != nil {
		return err
	}
	if opts.UsesJSONOutput() {
		return PrintJSONResponse(opts, data)
	}

	decoded, err := DecodeJSON[T](data)
	if err != nil {
		return err
	}
	return render(decoded)
}

// RunRequest executes an authenticated API request and preserves the shared
// JSON/JQ fast-path.
func RunRequest(opts Options, spinnerMessage, method, path string, params url.Values, render func(json.RawMessage) error) error {
	if opts.DryRun && method != http.MethodGet {
		return PrintDryRunRequest(opts, method, path, params)
	}
	return Run(opts, spinnerMessage, requestRunner(method, path, params), render)
}

// RunRequestDecoded executes an authenticated API request and decodes the
// response for human/plain rendering while preserving the shared JSON/JQ
// fast-path.
func RunRequestDecoded[T any](opts Options, spinnerMessage, method, path string, params url.Values, render func(T) error) error {
	if opts.DryRun && method != http.MethodGet {
		return PrintDryRunRequest(opts, method, path, params)
	}
	return RunDecoded[T](opts, spinnerMessage, requestRunner(method, path, params), render)
}

// RunRequestDecodedWithToken executes an API request with a caller-supplied
// token (use "" for unauthenticated public endpoints) and decodes the response
// for human/plain rendering while preserving the shared JSON/JQ fast-path.
func RunRequestDecodedWithToken[T any](opts Options, token, spinnerMessage, method, path string, params url.Values, render func(T) error) error {
	if opts.DryRun && method != http.MethodGet {
		return PrintDryRunRequest(opts, method, path, params)
	}
	data, err := runWithTokenData(opts, token, spinnerMessage, requestRunner(method, path, params))
	if err != nil {
		return err
	}
	if opts.UsesJSONOutput() {
		return PrintJSONResponse(opts, data)
	}
	decoded, err := DecodeJSON[T](data)
	if err != nil {
		return err
	}
	return render(decoded)
}

// RunRequestWithSuccess executes a mutating API request and prints a success
// message in human mode. The id identifies the affected resource in JSON output.
func RunRequestWithSuccess(opts Options, spinnerMessage, method, path string, params url.Values, id, successMessage string) error {
	if opts.DryRun && method != http.MethodGet {
		return PrintDryRunRequest(opts, method, path, params)
	}

	data, err := runAuthenticatedData(opts, spinnerMessage, requestRunner(method, path, params))
	if err != nil {
		return err
	}
	return PrintMutationSuccess(opts, data, id, successMessage)
}

// RunWithToken executes a caller-provided client operation with a
// caller-supplied token.
func RunWithToken(opts Options, token, spinnerMessage string, run ClientRunner, render func(json.RawMessage) error) error {
	data, err := RunWithTokenData(opts, token, spinnerMessage, run)
	if err != nil {
		return err
	}
	if opts.UsesJSONOutput() {
		return PrintJSONResponse(opts, data)
	}
	return render(data)
}

// RunWithTokenData executes a caller-provided client operation with a
// caller-supplied token and returns the raw response body without rendering it.
func RunWithTokenData(opts Options, token, spinnerMessage string, run ClientRunner) (json.RawMessage, error) {
	return runWithTokenData(opts, token, spinnerMessage, run)
}

func runAuthenticatedData(opts Options, spinnerMessage string, run ClientRunner) (json.RawMessage, error) {
	token, err := config.Token()
	if err != nil {
		return nil, err
	}
	return runWithTokenData(opts, token, spinnerMessage, run)
}

func runWithTokenData(opts Options, token, spinnerMessage string, run ClientRunner) (json.RawMessage, error) {
	if ShouldShowSpinner(opts) {
		sp := newSpinner(spinnerMessage, opts.Err())
		sp.Start()
		defer sp.Stop()
	}

	client := NewAPIClient(opts, token)
	data, err := run(client)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func normalizeJSONBody(data json.RawMessage) json.RawMessage {
	if len(bytes.TrimSpace(data)) == 0 {
		return json.RawMessage("null")
	}
	return data
}

// PrintJSONResponse renders a raw API response using the command's JSON/JQ
// settings while preserving the shared empty-body normalization.
func PrintJSONResponse(opts Options, data json.RawMessage) error {
	return output.PrintJSON(opts.Out(), normalizeJSONBody(data), opts.JQExpr)
}

type mutationOutput struct {
	Success   bool            `json:"success"`
	Message   string          `json:"message"`
	ID        string          `json:"id,omitempty"`
	Result    json.RawMessage `json:"result"`
	Cancelled bool            `json:"cancelled,omitempty"`
	Action    string          `json:"action,omitempty"`
}

func printMutationJSON(opts Options, data json.RawMessage, id, successMessage string) error {
	return printMutationPayload(opts, mutationOutput{
		Success: true,
		Message: successMessage,
		ID:      id,
		Result:  normalizeJSONBody(data),
	})
}

func printMutationPayload(opts Options, payload mutationOutput) error {
	payload.Result = normalizeJSONBody(payload.Result)

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("could not encode JSON output: %w", err)
	}
	return output.PrintJSON(opts.Out(), data, opts.JQExpr)
}

// PrintMutationSuccess renders a successful mutating command with the shared
// human/plain/JSON envelope used across the CLI.
func PrintMutationSuccess(opts Options, data json.RawMessage, id, successMessage string) error {
	return renderMutationSuccess(opts, data, id, successMessage)
}

func renderMutationSuccess(opts Options, data json.RawMessage, id, successMessage string) error {
	if opts.UsesJSONOutput() {
		return printMutationJSON(opts, data, id, successMessage)
	}
	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{"true", successMessage}})
	}
	return PrintSuccess(opts, successMessage)
}

func requestRunner(method, path string, params url.Values) ClientRunner {
	return func(client *api.Client) (json.RawMessage, error) {
		return runClientRequest(client, method, path, params)
	}
}

func runClientRequest(client *api.Client, method, path string, params url.Values) (json.RawMessage, error) {
	switch method {
	case "GET":
		return client.Get(path, params)
	case "POST":
		return client.Post(path, params)
	case "PUT":
		return client.Put(path, params)
	case "DELETE":
		return client.Delete(path, params)
	default:
		return nil, fmt.Errorf("unsupported HTTP method: %s", method)
	}
}
