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

## Refund policy

```sh
# Read the store-wide refund policy shown to buyers
gumroad refund-policy view

# Set the refund period and fine print
gumroad refund-policy set --period 30 --fine-print "Refund requests are reviewed within 2 business days."

# Clear fine print and allow no refunds
gumroad refund-policy set --period none --fine-print ""
```

Refund policy is account-level, not per-product. Allowed periods are `none`, `7`, `14`, `30`, and `183`; `--fine-print` is optional and capped by the API at 3000 characters.

## Product categories

```sh
# Find the category path to use on create/update
gumroad products categories --search figma

# Set a product category by path
gumroad products create --name "Figma Kit" --category design/ui-and-web/figma
gumroad products update <product_id> --category design/ui-and-web/figma
```

Use the `path` from `gumroad products categories` with `--category`. The older numeric `--taxonomy-id` flag still works when you already have an API taxonomy ID, but it cannot be combined with `--category`.

## File attachments

```sh
# Upload a file and print the canonical Gumroad URL
gumroad files upload ./pack.zip

# Create a product with an attached file
gumroad products create --name "Art Pack" --price 10.00 --file ./pack.zip --file-name "Art Pack.zip"

# Roll the file embeds in a shared product content document
gumroad products update <product_id> --file ./pack.zip
```

`gumroad files upload` prints the canonical `file_url`; product create/update commands accept repeatable `--file` flags. On update, `--file` replaces existing rich content file embeds in place when they exist, or creates file embeds when the document has none. Pass one `--file` per existing file embed; use `products content get/set` for structural content edits. For per-variant content, use `variants update ... --file` on the target variant.

## Product content

```sh
# Dump the shared rich content document for editing
gumroad products content get <product_id> > content.json

# Dump a per-variant rich content document for editing
gumroad products content get <product_id> --variant <variant_id> --category <cat_id> > content.json

# List page IDs, titles, positions, and top-level block counts
gumroad products content list <product_id>

# Dump one page object for editing
gumroad products content get <product_id> --page <page_id> > page.json

# Preview the whole-document replacement request
gumroad products content set <product_id> content.json --dry-run

# Preview a single-page replacement after merging it into the existing document
gumroad products content set <product_id> --page <page_id> --dry-run
gumroad products content set <product_id> page.json --page <page_id> --dry-run

# Preview a per-variant whole-document replacement request
gumroad products content set <product_id> content.json --variant <variant_id> --category <cat_id> --dry-run

# Replace the shared rich content document
gumroad products content set <product_id> content.json --yes
```

`products content set` sends the whole `rich_content` page array back to Gumroad. Existing pages omitted from the JSON are deleted, so the command prompts before replacing a document that drops existing page IDs; use `--dry-run` to inspect the exact `PUT` body first. Without a path, whole-document `set` reads `./content.json`. `--page <page_id>` reads one page object from `./page.json` by default, requires its `id` to match, merges it into the fetched document, and then sends the merged whole-document `PUT`. Omit `--variant` for shared product content. For per-variant content, pass both `--variant` and `--category` because the Gumroad API addresses variants through their category path.

## Product media

```sh
# Create a draft product, then attach cover and thumbnail images
gumroad products create --name "Art Pack" --price 10.00 --cover-image ./cover.jpg --thumbnail ./thumb.jpg

# Add preview/gallery images to an existing product
gumroad products update <product_id> --preview-image ./gallery-1.jpg --preview-image ./gallery-2.jpg

# Set a thumbnail from a public image URL
gumroad products thumbnail set <product_id> --url https://example.com/thumb.png
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
