# gumroad-cli

CLI for the [Gumroad API](https://app.gumroad.com/api). Designed for humans and AI agents alike.

[![CI](https://github.com/antiwork/gumroad-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/antiwork/gumroad-cli/actions/workflows/ci.yml)
[![Go](https://img.shields.io/github/go-mod/go-version/antiwork/gumroad-cli)](https://github.com/antiwork/gumroad-cli)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://github.com/antiwork/gumroad-cli/blob/main/LICENSE)

## Install

```sh
brew install --cask antiwork/cli/gumroad
```

<details>
<summary>Other installation methods</summary>

**Shell script** (macOS, Linux, Windows via Git Bash):

```sh
curl -fsSL https://gumroad.com/install-cli | bash
```

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

</details>

## Quick start

```sh
# Authenticate (opens browser for OAuth)
gumroad auth login

# Or use an environment variable for CI / agents
export GUMROAD_ACCESS_TOKEN=your-token

# View your account
gumroad user

# List products, then inspect one
gumroad products list
gumroad products view abc123

# Fetch all sales, filter with jq
gumroad sales list --all --json --jq '.sales[] | {email, formatted_total_price}'

# Preview a refund without executing it
gumroad sales refund abc123 --amount 5.00 --dry-run
```

## Authentication

`gumroad auth login` opens your browser for OAuth authorization. After you approve, the CLI stores the token locally and you're done.

```sh
gumroad auth login          # Browser-based OAuth (default)
gumroad auth login --web    # Force browser OAuth, no fallback
gumroad auth status         # Check who you're logged in as
gumroad auth logout         # Remove stored token
```

When a browser isn't available (SSH, containers), the CLI falls back to a manual flow: it prints the authorize URL and you paste the redirect URL back.

For CI and agents, set `GUMROAD_ACCESS_TOKEN` instead — it takes precedence over stored config and needs no interactive login. Piped stdin also works: `echo $TOKEN | gumroad auth login`.

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

Run `gumroad <command> --help` for usage details and examples.

## Output modes

| Flag | Output | Use case |
|------|--------|----------|
| *(default)* | Colored, formatted output | Human reading |
| `--json` | JSON | Programmatic access |
| `--jq <expr>` | Filtered JSON | Extract specific fields |
| `--plain` | Tab-separated, control chars escaped | Piping to `grep`/`awk` |
| `--quiet` | Minimal | Scripts |

Paginated commands (`sales list`, `payouts list`, `subscribers list`) accept `--all` to fetch every page. Use `--page-delay 200ms` to pace large fetches.

Mutation commands (create, update, delete, refund, ship) emit a JSON envelope with `--json`:

```json
{
  "success": true,
  "message": "Product prod_123 deleted.",
  "result": { "...": "raw API response" }
}
```

If you decline a confirmation prompt, the envelope includes `success: false`, `cancelled: true`, and `result: null`.

## Shell completion

```sh
source <(gumroad completion bash)                        # Bash
gumroad completion zsh > "${fpath[1]}/_gumroad"          # Zsh
gumroad completion fish | source                         # Fish
gumroad completion powershell | Out-String | Invoke-Expression  # PowerShell
```

## AI agents

`gumroad` is built to work with AI agents. The `--json`, `--jq`, and `--no-input` flags make it easy to query Gumroad data programmatically, and `GUMROAD_ACCESS_TOKEN` gives agents a no-persistence auth path.

A [Claude Code skill](.claude/skills/gumroad-cli/SKILL.md) is included in this repo. Install it to let Claude automatically use `gumroad` when you ask about your products, sales, or subscribers.

## API coverage

`gumroad` covers the [Gumroad API v2](https://app.gumroad.com/api) — products, sales, payouts, subscribers, licenses, offer codes, variant categories, variants, custom fields, and webhooks are all implemented.

**Not yet in the CLI:**

- **File uploads** — the API supports a presign → S3 upload → complete workflow for product files, covers, and thumbnails. The CLI does not yet wrap this. Use the web UI for file management.
- **Rich content pages** — Gumroad's multi-section page editor (the "Content" tab) has no public API. Product descriptions (`--description`, HTML) are fully supported.

**Worth knowing:**

- **Webhook deletion** requires the token's OAuth app to match the app that created the subscription. Webhooks created through the web UI cannot be deleted via the API.


## Design principles

Built following [clig.dev](https://clig.dev/) guidelines and [`gh`](https://github.com/cli/cli) conventions:

- **Human-first, machine-readable on demand** — formatted output by default, `--json`/`--plain` for machines
- **Secrets stay off the command line** — login uses browser OAuth or reads from stdin; token stored with `0600` permissions
- **Headless-friendly auth** — `GUMROAD_ACCESS_TOKEN` overrides stored config for shells, agents, and CI
- **Confirm destructive ops** — interactive confirmation for delete/refund, `--yes` to skip
- **Safe previews** — `--dry-run` shows mutating requests without executing them
- **Rewrite errors for humans** — no raw API JSON, actionable suggestions instead
- **Respect the terminal** — colors off when not TTY or `NO_COLOR` set, pager for long output

## Configuration

```
~/.config/gumroad/config.json    # macOS/Linux (0600 permissions)
%APPDATA%\gumroad\config.json    # Windows
```

On Unix, `XDG_CONFIG_HOME` is respected if set. `GUMROAD_ACCESS_TOKEN` takes precedence over the stored config.

## Development

```sh
make build        # Compile to ./gumroad
make test         # Run all tests
make test-cover   # Tests with per-package coverage gates (85% cmd, 90% infra)
make test-smoke   # Live read-only smoke test against real API
make lint         # golangci-lint
make man          # Generate man pages
make snapshot     # Build release snapshot via goreleaser
```

Live smoke test:

```sh
GUMROAD_ACCESS_TOKEN=your-token make test-smoke
```

### Architecture

```
cmd/gumroad/main.go    Entry point
internal/
  cmd/                 Command packages (one per noun, cobra)
  api/                 HTTP client for Gumroad API v2
  oauth/               OAuth browser login with PKCE
  config/              XDG-compliant config
  output/              Table, JSON, plain, color, spinner, pager
  prompt/              Interactive input and confirmations
  cmdutil/             Shared command utilities
  testutil/            Mock HTTP server and test helpers
```

Built with Go, [cobra](https://github.com/spf13/cobra), and [gojq](https://github.com/itchyny/gojq).

## License

[MIT](LICENSE)
