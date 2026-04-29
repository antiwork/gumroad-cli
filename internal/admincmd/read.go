package admincmd

import (
	"encoding/json"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/adminapi"
	"github.com/antiwork/gumroad-cli/internal/adminconfig"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
)

type ClientRunner func(*adminapi.Client) (json.RawMessage, error)

func NewAPIClient(opts cmdutil.Options, token string) *adminapi.Client {
	client := adminapi.NewClientWithContext(opts.Context, token, opts.Version, opts.DebugEnabled())
	client.SetDebugWriter(opts.Err())
	return client
}

func Run(opts cmdutil.Options, spinnerMessage string, run ClientRunner, render func(json.RawMessage) error) error {
	data, err := runAuthenticatedData(opts, spinnerMessage, run)
	if err != nil {
		return err
	}
	if opts.UsesJSONOutput() {
		return cmdutil.PrintJSONResponse(opts, data)
	}
	return render(data)
}

func RunDecoded[T any](opts cmdutil.Options, spinnerMessage string, run ClientRunner, render func(T) error) error {
	data, err := runAuthenticatedData(opts, spinnerMessage, run)
	if err != nil {
		return err
	}
	if opts.UsesJSONOutput() {
		return cmdutil.PrintJSONResponse(opts, data)
	}

	decoded, err := cmdutil.DecodeJSON[T](data)
	if err != nil {
		return err
	}
	return render(decoded)
}

func RunGet(opts cmdutil.Options, spinnerMessage, path string, params url.Values, render func(json.RawMessage) error) error {
	return Run(opts, spinnerMessage, func(client *adminapi.Client) (json.RawMessage, error) {
		return client.Get(path, params)
	}, render)
}

func RunGetDecoded[T any](opts cmdutil.Options, spinnerMessage, path string, params url.Values, render func(T) error) error {
	return RunDecoded[T](opts, spinnerMessage, func(client *adminapi.Client) (json.RawMessage, error) {
		return client.Get(path, params)
	}, render)
}

// FetchGetDecoded fetches a GET endpoint and decodes the response without
// rendering it. Use it when chaining requests where only the data is needed.
func FetchGetDecoded[T any](opts cmdutil.Options, spinnerMessage, path string, params url.Values) (T, error) {
	var zero T
	data, err := runAuthenticatedData(opts, spinnerMessage, func(client *adminapi.Client) (json.RawMessage, error) {
		return client.Get(path, params)
	})
	if err != nil {
		return zero, err
	}
	return cmdutil.DecodeJSON[T](data)
}

func RunPostJSONDecoded[T any](opts cmdutil.Options, spinnerMessage, path string, payload any, render func(T) error) error {
	return RunDecoded[T](opts, spinnerMessage, func(client *adminapi.Client) (json.RawMessage, error) {
		return client.PostJSON(path, payload)
	}, render)
}

// FetchPostJSON posts a JSON payload and returns the raw response without
// rendering it. Use this when the caller needs to keep transport-level
// failures distinguishable from post-success render/decode errors (e.g. so
// only the POST failure gets a "request failed" wrapper).
func FetchPostJSON(opts cmdutil.Options, spinnerMessage, path string, payload any) (json.RawMessage, error) {
	return runAuthenticatedData(opts, spinnerMessage, func(client *adminapi.Client) (json.RawMessage, error) {
		return client.PostJSON(path, payload)
	})
}

func runAuthenticatedData(opts cmdutil.Options, spinnerMessage string, run ClientRunner) (json.RawMessage, error) {
	token, err := adminconfig.Token()
	if err != nil {
		return nil, err
	}
	return runWithTokenData(opts, token, spinnerMessage, run)
}

func runWithTokenData(opts cmdutil.Options, token, spinnerMessage string, run ClientRunner) (json.RawMessage, error) {
	if cmdutil.ShouldShowSpinner(opts) {
		sp := output.NewSpinnerTo(spinnerMessage, opts.Err())
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
