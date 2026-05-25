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

# Show sales totals
gumroad sales summary --group-by month --from 2026-01-01 --to 2026-05-21

# Preview a refund without executing it
gumroad sales refund abc123 --amount 5.00 --dry-run
```

## Authentication

`gumroad auth login` opens your browser for OAuth authorization. After you approve, the CLI stores the seller token locally.

```sh
gumroad auth login          # Browser-based OAuth (default)
gumroad auth login --web    # Force browser OAuth, no fallback
gumroad auth status         # Check seller auth and stored admin auth
gumroad auth logout         # Revoke/delete stored tokens
```

When a browser isn't available, the CLI prints the authorize URL and you paste the redirect URL back. For CI and agents, set `GUMROAD_ACCESS_TOKEN`; it takes precedence over stored seller config and needs no interactive login.

## Commands

Run `gumroad --help` and `gumroad <command> --help` for subcommands, usage details, and examples.

Admin commands use a separate internal token. Run `gumroad auth login` and check the admin box to store one locally, or use `GUMROAD_ADMIN_TOKEN` with `--non-interactive` in CI and agent runs. For local testing, set `GUMROAD_ADMIN_API_BASE_URL`.

## File attachments

```sh
# Upload a file and print the canonical Gumroad URL
gumroad files upload ./pack.zip

# Create a product with an attached file
gumroad products create --name "Art Pack" --price 10.00 --file ./pack.zip --file-name "Art Pack.zip"

# Add a new file to a product while keeping its current attachments
gumroad products update <product_id> --file ./pack.zip
```

`gumroad files upload` prints the canonical `file_url`; product create/update commands accept repeatable `--file` flags. Run command help for metadata, removal, replacement, and per-variant Content options.

## Product media

```sh
# Create a draft product, then attach cover and thumbnail images
gumroad products create --name "Art Pack" --price 10.00 --cover-image ./cover.jpg --thumbnail ./thumb.jpg

# Add preview/gallery images to an existing product
gumroad products update <product_id> --preview-image ./gallery-1.jpg --preview-image ./gallery-2.jpg
```

Product media upload supports JPEG, PNG, and GIF. Use `gumroad products covers --help` and `gumroad products thumbnail --help` for full-control resource commands.

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

`gumroad` is built to work with AI agents. The `--json`, `--jq`, `--no-input`, and `--non-interactive` flags make it easy to query Gumroad data programmatically, and `GUMROAD_ACCESS_TOKEN` gives agents a no-persistence seller auth path.

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
