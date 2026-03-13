# CLAUDE.md

## Build & test

```sh
make build        # Compile to ./gumroad
make test         # Run all tests
make test-cover   # Tests + coverage gates (85% cmd, 90% infra)
make lint         # golangci-lint
```

## Project structure

- `cmd/gumroad/main.go` — entry point, calls `cmd.Execute()`
- `internal/cmd/` — one package per noun (products, sales, etc.), each with cobra commands
- `internal/api/` — HTTP client (`client.go`) and error rewriting (`errors.go`)
- `internal/config/` — XDG config at `~/.config/gumroad/config.json`, `0600` perms
- `internal/output/` — table, JSON, plain, color, spinner, pager, image rendering
- `internal/prompt/` — interactive token input and Y/N confirmations
- `internal/cmdutil/` — per-command `Options` struct (JSON, quiet, dry-run, etc.) and request runners
- `internal/testutil/` — mock HTTP server and test helpers

## Adding a new command

1. Create package under `internal/cmd/<noun>/`
2. Add `New<Noun>Cmd()` returning `*cobra.Command`, register in `internal/cmd/root.go`
3. Each subcommand: parse flags → call `api.Client` → format via `output` package
4. Always use `RunE` (not `Run`) to propagate errors
5. Add tests — coverage must meet gates

### Output pipeline

Commands use `cmdutil.RunRequestDecoded[T]()` (or `RunRequest`, `RunRequestWithSuccess` for mutations). These runners handle auth, spinners, dry-run, and JSON/JQ output automatically — the render callback only needs to handle the plain and table cases:

```go
return cmdutil.RunRequestDecoded[productsListResponse](opts, "Fetching products...", "GET", "/products", url.Values{}, func(resp productsListResponse) error {
    if opts.PlainOutput {
        return output.PrintPlain(opts.Out(), rows)
    }
    // Default: colored table with pager
    return output.WithPager(opts.Out(), func(w io.Writer) error {
        return tbl.Render(w)
    })
})
```

### Destructive operations

Delete/refund require `prompt.Confirm()`. `--yes` skips it, `--no-input` fails if confirmation is needed.

### Test pattern

Tests use `testutil.Setup` to create a mock HTTP server and temp config. `testutil.Command` wraps a cobra command with test options. Example:

```go
testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
    testutil.JSON(t, w, map[string]any{
        "products": []map[string]any{
            {"id": "p1", "name": "Art Pack", "published": true, "formatted_price": "$10"},
        },
    })
})

cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{}) })
```

## Gumroad API quirks

These are non-obvious behaviors in the Gumroad API v2 that directly affect how you write code:

- **200 OK with `success: false`** — the API returns HTTP 200 for many errors. Always check the `success` field in the response body, not just the status code. `internal/api/errors.go` handles this.
- **Inconsistent numeric types** — some fields like `sales_usd_cents` arrive as `0` (int) or `0.0` (float) depending on state. Use `json.Number` or handle both when parsing.
- **Null vs missing fields** — optional fields may be `null`, empty string, or omitted entirely.
- **Products create/update return 404** — the routes exist but the actions are stubbed out. Only list, view, delete, enable, disable work.
- **Deprecated `page` param on sales** — use cursor-based `page_key` instead.
