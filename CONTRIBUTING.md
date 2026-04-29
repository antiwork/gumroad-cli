# Contributing to Gumroad CLI

## Overall

Use native-sounding English in all communication with no excessive capitalization (e.g HOW IS THIS GOING), multiple question marks (how's this going???), grammatical errors (how's dis going), or typos (thnx fr update).

- ❌ Before: "is this still open ?? I am happy to work on it ??"
- ✅ After: "Is this actively being worked on? I've started work on it here…"

Explain the reasoning behind your changes, not just the change itself. Describe the architectural decision or the specific problem being solved. For bug fixes, identify the root cause. Don't apply a fix without explaining how the invalid state occurred.

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

## Pull requests

- Include an AI disclosure
- Self-review (comment) on your code
- Break up big 1k+ line PRs into smaller PRs (100 loc)
- **Must**: Include a video for every PR. For user-facing changes (new commands, changed output), show before/after in a terminal recording. For non-user-facing changes, record a short walkthrough of the relevant existing functionality to demonstrate understanding and confirm nothing broke.
- Include updates to any tests!

### PR description structure

Non-trivial PRs should follow this structure:

- **What** — What this PR does. Concrete changes, not a list of files.
- **Why** — Why this change exists and why this approach was chosen over alternatives.
- **Before/After** — Video is required for all PRs. For user-facing changes, show before/after terminal output. For non-user-facing changes, include a short walkthrough video.
- **Test Results** — Screenshot of tests passing locally.

End with an AI disclosure after a `---` separator. Name the specific model (e.g., "Claude Opus 4.6") and list the prompts given to the agent.

## AI models

Use the latest and greatest state-of-the-art models from American AI companies like [Anthropic](https://www.anthropic.com/) and [OpenAI](https://openai.com/). As of this writing, that means Claude Opus 4.7 and GPT-5.5, but always check for the newest releases. Don't settle for last-gen models when better ones are available.

## Before pushing

Always run the full check suite before pushing:

```bash
make test-cover   # Tests with coverage gates (85% cmd, 90% infra)
make lint         # golangci-lint
```

Do not push code with failing tests. CI is not a substitute for local verification.

## Code standards

- Run `gofmt` before committing (the linter enforces this)
- Follow [Effective Go](https://go.dev/doc/effective_go) conventions
- Don't leave comments in the code
- No explanatory comments please
- Don't apologize for errors, fix them
- Assign raw numbers to named constants to clarify their purpose

### Code patterns

- Use `product` instead of `link` in new code
- Use `buyer` and `seller` when naming variables instead of `customer` and `creator`

### Testing guidelines

- Don't use "should" in test descriptions
- Write descriptive test names that explain the behavior being tested
- Group related tests together
- Keep tests independent and isolated
- Tests must fail when the fix is reverted. If the test passes without the application code change, it is invalid.
- Use `testutil.Setup` for mock HTTP servers and `testutil.Command` for wrapping cobra commands
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

## Writing issues

Issues for enhancements, features, or refactors use this structure:

### What

What needs to change. Be concrete:

- Describe the current behavior and the desired behavior
- Who is affected (CLI users, internal team)
- Quantify impact with data when possible
- Use a checkbox task list for multiple deliverables

### Why

Why this change matters:

- What user or business problem does this solve?
- Link to related issues, support tickets, or prior discussions for context

Keep it short. The title should carry most of the weight, the body adds context the title can't.

## Writing bug reports

A great bug report includes:

- A quick summary and/or background
- Steps to reproduce
  - Be specific!
  - Give sample code if you can
- What you expected would happen
- What actually happens
- Notes (possibly including why you think this might be happening, or stuff you tried that didn't work)

## Help

- Any issue with label `help wanted` is open for contributions - [view open issues](https://github.com/antiwork/gumroad-cli/issues?q=state%3Aopen%20label%3A%22help%20wanted%22)

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE.md).
