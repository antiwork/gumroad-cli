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
gumroad admin         Internal admin API commands
gumroad discover      search
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

Admin commands use a separate internal token. For non-interactive use, set `GUMROAD_ADMIN_ACCESS_TOKEN`; for local testing, set `GUMROAD_ADMIN_API_BASE_URL`.

## File attachments

```sh
# Upload a file and print the canonical Gumroad URL
gumroad files upload ./pack.zip

# Recover an upload after a state-unknown complete failure
gumroad files upload ./pack.zip --json > err.json
jq '.error.recovery' err.json | gumroad files complete --recovery - --yes

# Abort an orphaned multipart upload from saved recovery fields
gumroad files abort --upload-id up-123 --key attachments/u/k/original/pack.zip

# Create a product with an attached file
gumroad products create --name "Art Pack" --price 10.00 --file ./pack.zip --file-name "Art Pack.zip"

# Add a new file to a product while keeping its current attachments
gumroad products update <product_id> --file ./pack.zip

# Replace the current file set, preserving only the IDs you keep explicitly
gumroad products update <product_id> --replace-files --keep-file <file_id> --file ./new-pack.zip
```

`gumroad files upload` and `gumroad files complete` both print the canonical `file_url`. Product create/update accept repeatable `--file` flags, with matching `--file-name` and `--file-description` values when you need custom attachment metadata. `gumroad products update` also supports `--remove-file`, and `--replace-files` with `--keep-file`, when you need to remove existing attachments.

## Discover

`gumroad discover search` queries the public catalog that powers gumroad.com/discover. The endpoint is unauthenticated — nothing in `~/.config/gumroad/config.json` is read, and no `Authorization` header is sent even when `GUMROAD_ACCESS_TOKEN` is set.

```sh
# Bare keyword search
gumroad discover search "machine learning"

# Filter by tag, taxonomy, and file type
gumroad discover search --tag productivity --taxonomy design/illustration --filetypes pdf,epub

# Price range with a minimum rating
gumroad discover search "icons" --min-price 5 --max-price 30 --rating 4

# Tri-state booleans: omit for "no filter", --flag (or =true) to require, =false to exclude
gumroad discover search --subscription           # only subscriptions
gumroad discover search --bundle=false           # exclude bundles
gumroad discover search --call=true --rating 5   # only top-rated calls

# Sort and limit
gumroad discover search "design" --sort best_sellers --limit 50

# JSON + jq for scripting
gumroad discover search "writing" --json --jq '.products[] | {name, url, price_cents}'
```

| Flag | Description |
|------|-------------|
| `--tag` | Filter by tag(s); comma-separated for multiple (e.g. `design,productivity`). |
| `--taxonomy` | Filter by category slug path (e.g. `3d/games`, `design/illustration`). |
| `--filetypes` | Filter by file type(s); comma-separated (e.g. `pdf,epub,zip`). |
| `--min-price` | Minimum price in dollars. Must be non-negative. |
| `--max-price` | Maximum price in dollars. Must be non-negative and >= `--min-price`. |
| `--rating` | Minimum average rating. Must be between 1 and 5. |
| `--min-reviews` | Minimum number of reviews. Must be non-negative. |
| `--staff-picked` | Only staff-picked products. |
| `--subscription` | Tri-state: unset = mixed, `--subscription` / `=true` = only, `=false` = exclude. |
| `--bundle` | Tri-state: unset = mixed, `--bundle` / `=true` = only, `=false` = exclude. |
| `--call` | Tri-state: unset = mixed, `--call` / `=true` = only, `=false` = exclude. |
| `--exclude-ids` | Exclude product IDs; comma-separated. |
| `--sort` | One of `default`, `best_sellers`, `curated`, `hot_and_new`, `newest`, `price_asc`, `price_desc`, `most_reviewed`, `highest_rated`, `recently_updated`, `staff_picked`. |
| `--limit` | Number of results to return. Must be 1–500. Defaults to 30. |
| `--from` | Offset for pagination. Must be non-negative. |

JSON response shape:

```json
{
  "total": 1284,
  "products": [
    {
      "id": "abc123",
      "permalink": "art-pack",
      "name": "Art Pack",
      "seller": {
        "id": "u_98765",
        "name": "Jane Doe",
        "avatar_url": "https://public-files.gumroad.com/avatar.jpg",
        "profile_url": "https://janedoe.gumroad.com",
        "is_verified": true
      },
      "ratings": {
        "count": 142,
        "average": 4.7
      },
      "native_type": "digital",
      "price_cents": 1500,
      "currency_code": "usd",
      "is_pay_what_you_want": false,
      "url": "https://janedoe.gumroad.com/l/art-pack",
      "thumbnail_url": "https://public-files.gumroad.com/thumb.jpg",
      "recurrence": "",
      "duration_in_months": null,
      "quantity_remaining": null,
      "is_sales_limited": false,
      "description": "A curated set of brushes, palettes, and reference sheets."
    }
  ]
}
```

If the public endpoint rate-limits the request, the CLI surfaces the `429` response with a `Wait a moment and retry` hint.

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
