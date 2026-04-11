# gumroad-cli

CLI for the Gumroad API. Designed for humans and AI agents alike.

[![CI](https://github.com/antiwork/gumroad-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/antiwork/gumroad-cli/actions/workflows/ci.yml)
[![Go](https://img.shields.io/github/go-mod/go-version/antiwork/gumroad-cli)](https://github.com/antiwork/gumroad-cli)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://github.com/antiwork/gumroad-cli/blob/main/LICENSE)

```
$ gumroad products list
ID            NAME              STATUS     PRICE
abc123        Design Templates  published  $25.00
def456        Icon Pack         published  $10.00

$ gumroad sales list --json --jq '.sales[0].email'
"customer@example.com"
```

## Install

**Homebrew**:

```sh
brew install --cask antiwork/cli/gumroad
```

On macOS, this installs the binary, man pages, and shell completions. Linux Homebrew support for casks depends on the Homebrew version and cask support available on your system; if `brew install --cask` is unavailable, use the shell script below.

**Shell script** (macOS, Linux, and Windows via Git Bash):

```sh
curl -fsSL https://gumroad.com/install-cli | bash
```

> Homebrew and the install script require a published release. Until the first release is tagged, use `go install` or build from source.

**Go**:

```sh
go install github.com/antiwork/gumroad-cli/cmd/gumroad@latest
```

**From source** with man pages and completions:

```sh
make install

# Or install into a custom prefix
make install PREFIX="$HOME/.local"
```

Under the selected `PREFIX`, `make install` places the binary in `bin/`, man pages in `share/man/man1/`, and shell completions under `share/`.

## Quick start

```sh
# Authenticate with your Gumroad API token
gumroad auth login

# Or use an ephemeral token for this shell / CI job
export GUMROAD_ACCESS_TOKEN=your-token

# View your account
gumroad user

# List your products
gumroad products list

# Fetch every page of sales
gumroad sales list --all

# Get a sale as JSON, filter with jq
gumroad sales view abc123 --json --jq '.sale.email'

# Preview a refund without executing it
gumroad sales refund abc123 --amount 5.00 --dry-run
```

## Commands

```
gumroad auth          login, status, logout
gumroad user          View your account info
gumroad products      create, update, list, view, delete, publish, unpublish, skus
gumroad sales         list, view, refund, ship, resend-receipt
gumroad payouts       list, view, upcoming
gumroad subscribers   list, view
gumroad licenses      verify, enable, disable, decrement, rotate
gumroad offer-codes   list, view, create, update, delete
gumroad variant-categories list, view, create, update, delete
gumroad variants      list, view, create, update, delete
gumroad custom-fields list, create, update, delete
gumroad webhooks      list, create, delete
gumroad completion    bash, zsh, fish, powershell
```

Every command has built-in help with examples: `gumroad <command> --help`

Man pages are available locally after `make install`. You can also generate them directly with `make man`.

## Shell completion

Generate and install shell completions directly from the command:

```sh
# Bash
source <(gumroad completion bash)

# Zsh
gumroad completion zsh > "${fpath[1]}/_gumroad"

# Fish
gumroad completion fish | source

# PowerShell
gumroad completion powershell | Out-String | Invoke-Expression
```

## Output modes

| Flag | Output | Use case |
|------|--------|----------|
| *(default)* | Colored tables | Human reading |
| `--json` | JSON | Programmatic access |
| `--jq <expr>` | Filtered JSON | Extract specific fields |
| `--plain` | Tab-separated, control chars escaped | Piping to `grep`/`awk` |
| `--quiet` | Minimal | Scripts |

Paginated list commands such as `sales list`, `payouts list`, and `subscribers list` accept `--all` to fetch every page automatically. Use `--page-delay 200ms` to pace large fetches when you want to be gentler on the API.

Mutation commands keep their human success messages by default, and with `--json` they emit a stable envelope:

```json
{
  "success": true,
  "message": "Product prod_123 deleted.",
  "result": { "...": "raw API response" }
}
```

If you decline a confirmation prompt, mutating commands still emit JSON in machine-readable modes with `success: false`, `cancelled: true`, and `result: null`.

## API coverage

`gumroad` maps 1:1 to the [Gumroad API v2](https://app.gumroad.com/api). The CLI exposes everything the API supports today — but the API has some gaps worth knowing about:

- **No rich content or file upload** — product create/update supports basic fields but not rich content pages or file attachments. Use the web UI for those.
- **No analytics or audience data** — no API endpoints exist for dashboard stats, traffic, or email lists.
- **No bulk operations** — all actions are one resource at a time.
- **Limited filtering** — `gumroad sales list` supports date/email/product filters, but `gumroad products list` returns everything with no filtering.
- **Non-standard errors** — Gumroad sometimes returns `200 OK` with `success: false` in the body instead of a 4xx/5xx.
- **Loose schemas** — some numeric fields arrive as `0` or `0.0`, and optional fields may be `null` or omitted.

`gumroad` normalizes these quirks where it can.

As the Gumroad API expands, the CLI will grow to match. The command structure is designed to accommodate new endpoints without breaking existing usage.

## Design principles

Built following [clig.dev](https://clig.dev/) guidelines and [`gh`](https://github.com/cli/cli) conventions:

- **Human-first, machine-readable on demand** — tables by default, `--json`/`--plain` for machines
- **Secrets never in args** — `gumroad auth login` prompts or reads stdin, token stored with `0600` permissions
- **Headless-friendly auth** — `GUMROAD_ACCESS_TOKEN` overrides stored config for shells, agents, and CI
- **Confirm destructive ops** — interactive confirmation for delete/refund, `--yes` to skip
- **Support safe previews** — `--dry-run` shows mutating requests without executing them
- **Rewrite errors for humans** — no raw API JSON, actionable suggestions instead
- **Respect the terminal** — colors off when not TTY or `NO_COLOR` set, pager for long output

## Architecture

```
cmd/gumroad/main.go    Entry point
internal/
  cmd/                 Command implementations (cobra)
    auth/              Authentication (login, status, logout)
    products/          gumroad products create|update|list|view|delete|publish|unpublish
    sales/             gumroad sales list|view|refund|ship|resend-receipt
    ...                One package per noun
  api/                 HTTP client for Gumroad API v2
  config/              XDG-compliant config (~/.config/gumroad/config.json)
  output/              Table, JSON, plain, color, spinner, pager, image rendering
  prompt/              Interactive input (token, confirmations)
  cmdutil/             Global flag state
  testutil/            Shared test helpers
```

Each command follows the same pattern: parse flags, call `api.Client`, format output via `output` package. Tests use a shared HTTP mock server from `testutil`.

## Developer Notes

- Human-facing tables should be built with a command-scoped styler (`opts.Style()` via `output.NewStyledTable`) so explicit flags like `--no-color` do not get lost behind terminal auto-detection.
- User-visible `--all` JSON/JQ output is staged before being copied to stdout. This is intentional: the CLI prefers atomic, valid output over true first-byte streaming so late failures do not leave partial JSON behind. Small responses stay in memory; larger ones spill to a temp file only when needed.

## AI agents

`gumroad` is designed to be used by AI coding agents. The `--json`, `--jq`, and `--no-input` flags make it easy to query Gumroad data programmatically without interactive prompts, and `GUMROAD_ACCESS_TOKEN` gives agents a no-persistence auth path.

A [Claude Code skill](.claude/skills/gumroad-cli/SKILL.md) is included in this repo. It teaches Claude when and how to use `gumroad` for Gumroad lookups — install it to let Claude automatically reach for `gumroad` when you ask about your products, sales, or subscribers.

## Development

```sh
make build        # Compile to ./gumroad
make install      # Install binary, man pages, and shell completions
make test         # Run all tests
make test-cover   # Run tests with per-package coverage gates
make test-smoke   # Run opt-in live API smoke test for read-only auth/list/view/output paths
make lint         # Run golangci-lint
make man          # Generate man pages
make snapshot     # Build release snapshot via goreleaser
```

Live smoke test:

```sh
GUMROAD_ACCESS_TOKEN=your-token make test-smoke
```

`make test-smoke` runs a small live, read-only sanity check against the real API only when `GUMROAD_SMOKE=1`; the make target sets that flag for you. It covers auth, representative list/view commands, and machine-readable output modes. Destructive flows still rely on mocked integration tests. You can optionally point it at another base URL with `GUMROAD_API_BASE_URL`.


Built with Go, [cobra](https://github.com/spf13/cobra), and [gojq](https://github.com/itchyny/gojq).

## License

[MIT](LICENSE)
