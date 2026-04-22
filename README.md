# <img src="assets/gumroad-badge.svg" height="28" alt="Gumroad"> Gumroad CLI

CLI for the [Gumroad API](https://app.gumroad.com/api). Designed for humans and AI agents alike.


## Install

```sh
brew install antiwork/cli/gumroad
```

If you previously installed the cask, switch once with:

```sh
brew uninstall --cask antiwork/cli/gumroad
brew install antiwork/cli/gumroad
```

<details>
<summary>Other installation methods</summary>

**Shell script** (macOS, Linux, Windows via Git Bash):

```sh
curl -fsSL https://gumroad.com/install-cli.sh | bash
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
gumroad files         upload, complete, abort
gumroad webhooks      list, create, delete
gumroad skill         Install or refresh the Claude Code skill
gumroad completion    bash, zsh, fish, powershell
```

Run `gumroad <command> --help` for usage details and examples.

## File attachments

```sh
# Upload a file and print the canonical Gumroad URL
gumroad files upload ./pack.zip

# Add a new file to a product while keeping its current attachments
gumroad products update <product_id> --file ./pack.zip

# Replace the current file set, preserving only the IDs you keep explicitly
gumroad products update <product_id> --replace-files --keep-file <file_id> --file ./new-pack.zip
```

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

`gumroad` is built to work with AI agents. The `--json`, `--jq`, and `--no-input` flags make it easy to query Gumroad data programmatically, and `GUMROAD_ACCESS_TOKEN` gives agents a no-persistence auth path.

A [Claude Code skill](skills/gumroad/SKILL.md) is included. Run `gumroad skill` to install or refresh it.

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
