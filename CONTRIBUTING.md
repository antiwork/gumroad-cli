# Contributing to Gumroad CLI

## Development setup

```bash
git clone https://github.com/antiwork/gumroad-cli
cd gumroad-cli
make build    # Compile to ./gumroad
make test     # Run all tests
```

### Requirements

- Go 1.25+
- [golangci-lint](https://golangci-lint.run/) for linting

## Making changes

1. Run the full check suite before pushing:
   ```bash
   make test-cover   # Tests with coverage gates (85% cmd, 90% infra)
   make lint         # golangci-lint
   ```
2. Add tests for new functionality — coverage must meet the gates
3. Update documentation if adding commands or changing behavior
4. Keep commits focused — one logical change per commit

## Pull requests

### PR description structure

- **What** — what this PR does. Concrete changes, not a list of files.
- **Why** — why this change exists and why this approach over alternatives.

End with an AI disclosure after a `---` separator. Name the specific model (e.g., "Claude Opus 4.6") and list the prompts used.

### Code style

- Run `gofmt` before committing (the linter enforces this)
- Follow [Effective Go](https://go.dev/doc/effective_go) conventions
- Use native-sounding English in all communication — no excessive capitalization, question marks, or typos
- Explain the reasoning behind changes, not just the change itself

### Testing guidelines

- Write descriptive test names that explain the behavior being tested
- Use `testutil.Setup` for mock HTTP servers and `testutil.Command` for wrapping cobra commands
- Tests must fail when the fix is reverted
- Use `@example.com` for emails and `example.com` for domains in tests

## Adding a new command

1. Create a package under `internal/cmd/<noun>/`
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
    return output.WithPager(opts.Out(), func(w io.Writer) error {
        return tbl.Render(w)
    })
})
```

### Destructive operations

Delete/refund require `prompt.Confirm()`. `--yes` skips it, `--no-input` fails if confirmation is needed.

### Test pattern

Tests use `testutil.Setup` to create a mock HTTP server and temp config. `testutil.Command` wraps a cobra command with test options:

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

Non-obvious behaviors that directly affect how you write code:

- **200 OK with `success: false`** — the API returns HTTP 200 for many errors. Always check the `success` field, not just the status code. `internal/api/errors.go` handles this.
- **Inconsistent numeric types** — some fields like `sales_usd_cents` arrive as `0` (int) or `0.0` (float) depending on state. Use `json.Number` or handle both when parsing.
- **Null vs missing fields** — optional fields may be `null`, empty string, or omitted entirely.
- **Deprecated `page` param on sales** — use cursor-based `page_key` instead.
