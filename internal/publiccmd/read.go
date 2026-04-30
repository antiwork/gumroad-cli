package publiccmd

import (
	"encoding/json"
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/publicapi"
)

type ClientRunner func(*publicapi.Client) (json.RawMessage, error)

func NewAPIClient(opts cmdutil.Options) *publicapi.Client {
	client := publicapi.NewClientWithContext(opts.Context, opts.Version, opts.DebugEnabled())
	client.SetDebugWriter(opts.Err())
	return client
}

func RunDecoded[T any](opts cmdutil.Options, spinnerMessage string, run ClientRunner, render func(T) error) error {
	data, err := runUnauthenticatedData(opts, spinnerMessage, run)
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

func RunGetDecoded[T any](opts cmdutil.Options, spinnerMessage, path string, params url.Values, render func(T) error) error {
	return RunDecoded[T](opts, spinnerMessage, func(client *publicapi.Client) (json.RawMessage, error) {
		return client.Get(path, params)
	}, render)
}

func runUnauthenticatedData(opts cmdutil.Options, spinnerMessage string, run ClientRunner) (json.RawMessage, error) {
	if cmdutil.ShouldShowSpinner(opts) {
		sp := output.NewSpinnerTo(spinnerMessage, opts.Err())
		sp.Start()
		defer sp.Stop()
	}

	client := NewAPIClient(opts)
	return run(client)
}
