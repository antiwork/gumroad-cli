# <img src="assets/gumroad-badge.svg" height="28" alt="Gumroad"> Gumroad CLI

CLI for the [Gumroad API](https://app.gumroad.com/api). Designed for humans and AI agents alike.

## Install

```sh
brew install antiwork/cli/gumroad
```

<details>
<summary>Other installation methods</summary>

```sh
# Shell script
curl -fsSL https://gumroad.com/install-cli.sh | bash

# Go
go install github.com/antiwork/gumroad-cli/cmd/gumroad@latest

# From source (default prefix)
make install
# From source (custom prefix)
make install PREFIX="$HOME/.local"
```

</details>

## Quick start

```sh
# Authenticate with device approval
gumroad auth login

# Or use an environment variable for CI / agents
export GUMROAD_ACCESS_TOKEN=your-token

# View your account
gumroad user

# View or set your account refund policy
gumroad refund-policy view
gumroad refund-policy set --period 30 --fine-print "Refund requests are reviewed within 2 business days."

# List products, then inspect one
gumroad products list
gumroad products view abc123

# Fetch all sales, filter with jq
gumroad sales list --all --json --jq '.sales[] | {email, formatted_total_price}'

# Show sales totals
gumroad sales summary --group-by month --from 2026-01-01 --to 2026-05-21

# Preview a refund without executing it
gumroad sales refund abc123 --amount 5.00 --dry-run
```

## Authentication

`gumroad auth login` starts OAuth device authorization. It prints a Gumroad approval URL, waits while you approve in the browser, then stores the seller token locally.

```sh
gumroad auth login          # Device authorization (default)
gumroad auth login --web    # Local browser OAuth
gumroad auth login --with-token < token.txt
gumroad auth status         # Check seller auth and stored admin auth
gumroad auth token          # Print the active seller token
gumroad auth logout         # Revoke/delete stored tokens
```

For CI and agents with an existing token, set `GUMROAD_ACCESS_TOKEN` or pipe the token into `gumroad auth login --with-token`; `GUMROAD_ACCESS_TOKEN` takes precedence over stored seller config and needs no interactive login. Use `--web` only when you specifically want the local browser PKCE flow.

## Commands

Run `gumroad --help` and `gumroad <command> --help` for subcommands, usage details, and examples.

Admin commands use a separate internal token. Run `gumroad auth login --web` and check the admin box to store one locally, or use `GUMROAD_ADMIN_TOKEN` with `--non-interactive` in CI and agent runs. For local testing, set `GUMROAD_ADMIN_API_BASE_URL`.

### Which token does a command use?

| Command group | Token | Env var |
|---|---|---|
| `user`, `products`, `sales`, `files`, `subscribers`, `payouts`, `licenses`, `offercodes`, `webhooks`, … | Seller access token | `GUMROAD_ACCESS_TOKEN` |
| `gumroad admin …` | Admin token (separate credential) | `GUMROAD_ADMIN_TOKEN` |

The two are not interchangeable. If a seller command fails with `not_authenticated` while `GUMROAD_ADMIN_TOKEN` is set — either alone, or copied into `GUMROAD_ACCESS_TOKEN` — you need a seller token: set `GUMROAD_ACCESS_TOKEN` to a seller token or run `gumroad auth login`.

## Output modes

| Flag | Output | Use case |
|------|--------|----------|
| *(default)* | Colored, formatted output | Human reading |
| `--json` | JSON | Programmatic access |
| `--jq <expr>` | Filtered JSON | Extract specific fields |
| `--plain` | Tab-separated, control chars escaped | Piping to `grep`/`awk` |
| `--quiet` | Minimal | Scripts |

Paginated commands (`sales list`, `payouts list`, `subscribers list`) accept `--all` to fetch every page. Use `--page-delay 200ms` to pace large fetches.

## AI agents

`gumroad` is built to work with AI agents. The `--json`, `--jq`, `--no-input`, and `--non-interactive` flags make it easy to query Gumroad data programmatically. Agents can start fresh seller auth with `gumroad auth login` and hand the printed approval URL to a human, or use `GUMROAD_ACCESS_TOKEN` for a no-persistence auth path when a token already exists.

An [agent skill](skills/gumroad/SKILL.md) is included. Run `gumroad skill` to install or refresh it.

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

## License

[MIT](LICENSE)
