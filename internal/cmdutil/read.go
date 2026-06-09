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
// {success, message, id, result} envelope used across most of the CLI. Prefer
// PrintResourceSuccess when the API already returns the resource at the top
// level and the write verb should match the read verbs' shape.
func PrintMutationSuccess(opts Options, data json.RawMessage, id, successMessage string) error {
	return renderMutationSuccess(opts, data, id, successMessage)
}

// PrintResourceSuccess renders a successful mutation by passing the API response
// through unchanged in JSON mode, with no result envelope. Write verbs then
// match their read verbs: a resource stays at the same top-level path (e.g.
// product), and a response that carries only a message (e.g. delete) surfaces
// that message verbatim. Human and plain output show successMessage.
//
// When id is non-empty and the JSON response has no top-level "id", id is merged
// in so callers can still correlate the affected resource. This matters for
// irreversible verbs whose response omits the resource entirely (delete returns
// only {success, message}); pass an empty id for verbs that already return the
// resource, leaving the response byte-for-byte unchanged.
func PrintResourceSuccess(opts Options, data json.RawMessage, id, successMessage string) error {
	if opts.UsesJSONOutput() {
		merged, err := withTopLevelID(data, id)
		if err != nil {
			return err
		}
		return PrintJSONResponse(opts, merged)
	}
	if opts.PlainOutput {
		return output.PrintPlain(opts.Out(), [][]string{{"true", successMessage}})
	}
	return PrintSuccess(opts, successMessage)
}

// RunRequestWithResource executes a mutating API request and renders it with
// PrintResourceSuccess, passing the API response through unchanged in JSON mode.
// It is the envelope-free counterpart to RunRequestWithSuccess. See
// PrintResourceSuccess for the id parameter's semantics.
func RunRequestWithResource(opts Options, spinnerMessage, method, path string, params url.Values, id, successMessage string) error {
	if opts.DryRun && method != http.MethodGet {
		return PrintDryRunRequest(opts, method, path, params)
	}

	data, err := runAuthenticatedData(opts, spinnerMessage, requestRunner(method, path, params))
	if err != nil {
		return err
	}
	return PrintResourceSuccess(opts, data, id, successMessage)
}

// withTopLevelID returns data with a top-level "id" field set to id. It is a
// no-op when id is empty or the response already carries a top-level "id", so
// responses that already expose the resource pass through unchanged. The id is
// appended after the existing fields, preserving the API's original key order.
func withTopLevelID(data json.RawMessage, id string) (json.RawMessage, error) {
	if id == "" {
		return data, nil
	}
	normalized := normalizeJSONBody(data)
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(normalized, &fields); err != nil {
		return nil, fmt.Errorf("could not parse response: %w", err)
	}
	if _, ok := fields["id"]; ok {
		return data, nil
	}
	idData, err := json.Marshal(id)
	if err != nil {
		return nil, fmt.Errorf("could not encode id: %w", err)
	}
	object := normalized
	if bytes.Equal(bytes.TrimSpace(object), []byte("null")) {
		object = nil
	}
	return AppendJSONField(object, "id", idData)
}

// AppendJSONField returns object with key:value appended as a top-level field,
// preserving the order of any existing fields. A nil, empty, or whitespace-only
// object yields {"key":value}. It returns an error when object is non-empty but
// not a JSON object. Surrounding whitespace is tolerated.
func AppendJSONField(object json.RawMessage, key string, value json.RawMessage) (json.RawMessage, error) {
	keyData, err := json.Marshal(key)
	if err != nil {
		return nil, fmt.Errorf("could not encode response key: %w", err)
	}
	object = bytes.TrimSpace(object)
	if len(object) == 0 {
		out := make([]byte, 0, len(keyData)+len(value)+4)
		out = append(out, '{')
		out = append(out, keyData...)
		out = append(out, ':')
		out = append(out, value...)
		out = append(out, '}')
		return out, nil
	}
	if !json.Valid(object) || len(object) < 2 || object[0] != '{' || object[len(object)-1] != '}' {
		return nil, fmt.Errorf("could not parse response: expected JSON object")
	}
	inner := bytes.TrimSpace(object[1 : len(object)-1])
	out := make([]byte, 0, len(object)+len(keyData)+len(value)+2)
	out = append(out, '{')
	if len(inner) > 0 {
		out = append(out, inner...)
		out = append(out, ',')
	}
	out = append(out, keyData...)
	out = append(out, ':')
	out = append(out, value...)
	out = append(out, '}')
	return out, nil
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
