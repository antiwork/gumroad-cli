---
name: gumroad
description: >
  Use the `gumroad` CLI to look up and manage Gumroad data from the terminal.
  Trigger when the user asks about Gumroad products, files, file uploads,
  attachments, sales, subscribers, licenses, payouts, offer codes, webhooks,
  or any Gumroad data lookup.
  Also trigger on "check my Gumroad", "look up a sale", "verify a license",
  "list my products", "how much have I made", "who bought", "recent sales",
  "refund a sale", "create a product", "upload a file", "attach a file to a product",
  "add a cover image", "set a product thumbnail", "upload product media",
  "attach a file to a variant", "finish a failed upload", "abort an upload", "manage webhooks",
  "check my earnings", "see my revenue", "who subscribed", "manage my store",
  "discount code", "coupon", "shipping status", "payout schedule", or any
  request to query or act on Gumroad data — even if the user doesn't say
  "Gumroad" explicitly but is clearly referring to their creator store or
  digital product sales.
  Do NOT trigger for Gumroad web UI, Rails, or codebase questions.
---

# gumroad CLI

Use `gumroad` (Gumroad CLI) to query and manage Gumroad data.

## Agent invariants

Always follow these rules:

- **Always** pass `--no-input` to prevent interactive prompts from blocking.
- **Always** pass `--json` for programmatic access.
- Use `--json --jq <expr>` together to extract exactly what you need.
- For operations that can prompt for confirmation (delete, refund, mutating admin actions, `files abort`, `files complete` replay, or product updates that remove files), add `--yes` to skip confirmation.
- Pass `--quiet` to suppress spinners and status messages.
- Pass `--dry-run` to preview mutating requests without executing them.
- Use `--page-delay 200ms` with `--all` to avoid rate limits on large datasets.
- Prices are in whole currency units (e.g. `--price 10.00` for $10), not cents. The CLI converts internally. Use `--currency eur` to change currency.
- Products are created as drafts — use `gumroad products publish <id>` to make them live.
- Product cover and thumbnail uploads support JPEG, PNG, and GIF. WebP is not supported by the API and the CLI rejects it before upload.
- If a command fails with a seller auth error, tell the user to run `gumroad auth login` interactively — agents cannot do this step.
- For admin commands in agents/CI, pass `--non-interactive` and set `GUMROAD_ADMIN_TOKEN`; interactive shells can store an admin token with `gumroad auth login`.

## Response shapes

Responses are wrapped in `{"success": true, ...}` with resource-specific keys:

- `user` → `.user`
- `products list` → `.products[]`
- `products view` → `.product`
- `sales list` → `.sales[]`
- `sales view` → `.sale`
- `sales export` → `.status`, `.recipient_email`
- `sales summary` → `.gross_cents`, `.net_cents`, `.breakdown[]`
- `payouts list` → `.payouts[]`, `payouts view/upcoming` → `.payout`
- `subscribers list` → `.subscribers[]`, `subscribers view` → `.subscriber`
- `licenses verify` → `.purchase`
- `offer-codes list` → `.offer_codes[]`
- `variant-categories list` → `.variant_categories[]`
- `variants list` → `.variants[]`
- `files upload` / `files complete` → `.file_url`
- `products create` with media flags → `.product` plus `.media[]`
- `products update` with media flags → mutation envelope with `.result.media[]`
- `products covers add --image` → `.result.covers[]`, `.result.main_cover_id`, plus `.result.media[]`
- `products covers add --url` → `.result.covers[]`, `.result.main_cover_id`
- `products thumbnail set` → `.result.media[].response`
- `webhooks list` → `.resource_subscriptions[]`
- `admin users info` → `.user`
- `admin users affiliates` → `.affiliates[]`
- `admin users comments list` → `.comments[]`
- `admin users comments add` → `.comment`
- `admin users compliance` → `.compliance_info`, `.info_requests[]`
- `admin users radar` → `.radar_stats`, `.recent_efws[]`
- `admin users purchases` → `.purchases[]`
- `admin users related` → `.related_users[]`, `.truncated`, `.per_signal_limit`
- `admin users mark-compliant`, `admin users suspend`, `admin users suspend-for-tos-violation` → `.status`, `.message`, `.user_id`
- `admin products flag-for-tos-violation` → `.status`, `.message`, `.user_id`, `.product_id`
- `admin payouts scheduled create` → `.message`, `.user_id`, `.scheduled_payout`
- `admin users refund-balance` → `.status`, `.message`, `.user_id`, `.count`, `.total_amount_cents`, `.currency`
- `admin purchases view` → `.purchase`
- `admin purchases search` → `.purchases[]`, `.has_more`, `.limit`
- `admin purchases lookup` → `.purchases[]`
- `admin products list` → `.products[]`, `admin products view` → `.product`

Admin pagination models differ by command:

- Cursor-paginated: `admin users affiliates`, `admin users comments list`, `admin users radar`, `admin users purchases`, and `admin purchases lookup` return `.pagination.next` as a cursor string. Pass it back with `--cursor`.
- Page-paginated: `admin products list` returns `.pagination.next` as an integer page number. Pass it back with `--page`; use `--per-page` for page size.
- Capped, not continuable: `admin users related` returns at most 50 related users per signal. Always inspect `.truncated`; when any signal is `true`, the result hit the cap and there is no cursor/page to fetch the rest.
- Capped, not continuable: `admin purchases search` returns `.has_more` when the server capped results. `--limit` is server-capped at 25 and there is no continuation token.

## Bulk operations

When creating or updating many products:

- Check existing products and permalinks first, then skip duplicates on re-runs.
- Derive custom permalinks deterministically from source data so retries are idempotent.
- Use `--dry-run --json` to preview generated requests, and ask the user to confirm before mutating more than 5 products.
- Continue past per-product errors, collect each failure with its product/permalink, and summarize successes and failures at the end.
- For product media failures after creation, retry with the command printed in the error, such as `gumroad products covers add <id> --image ./cover.jpg`.

## Commands

### auth — Manage authentication

```sh
# Check auth (do this first if unsure)
gumroad auth status --no-input

# Login requires interactive input — tell the user to run it themselves
# gumroad auth login

# Logout
gumroad auth logout --yes --no-input
```

### user — Account info

```sh
gumroad user --json --no-input
gumroad user --json --jq '.user.email' --no-input
```

### admin — Internal admin API

```sh
# Admin commands need internal admin auth.
# In agents/CI, set GUMROAD_ADMIN_TOKEN and pass --non-interactive.

# Inspect user identity, sign-in, social, risk, payout, and watchlist state
gumroad admin users info --email seller@example.com --json --non-interactive --no-input

# Review affiliate relationships
gumroad admin users affiliates --user-id 2245593582708 --direction granted --limit 50 --json --non-interactive --no-input
gumroad admin users affiliates --email seller@example.com --direction received --cursor cur-next --json --non-interactive --no-input

# Read and add admin comments
gumroad admin users comments list --user-id 2245593582708 --type note --limit 50 --json --non-interactive --no-input
gumroad admin users comments add --user-id 2245593582708 --content "VAT exempt confirmed" --yes --json --non-interactive --no-input

# Inspect compliance, Radar risk, and buyer history
gumroad admin users compliance --user-id 2245593582708 --json --non-interactive --no-input
gumroad admin users radar --user-id 2245593582708 --limit 50 --json --non-interactive --no-input
gumroad admin users purchases --user-id 2245593582708 --status successful --has-early-fraud-warning=false --limit 50 --json --non-interactive --no-input

# Find related accounts by risk signals
gumroad admin users related --email seller@example.com --signal ip --signal payment_address --json --non-interactive --no-input
gumroad admin users related --email seller@example.com --json --jq '{related_users, truncated, per_signal_limit}' --non-interactive --no-input

# Mutate user compliance and suspension state
gumroad admin users mark-compliant --user-id 2245593582708 --expected-email seller@example.com --note "Cleared after review" --yes --json --non-interactive --no-input
gumroad admin users suspend --user-id 2245593582708 --expected-email seller@example.com --note "Chargeback risk confirmed" --yes --json --non-interactive --no-input
gumroad admin users suspend-for-tos-violation --user-id 2245593582708 --expected-email seller@example.com --note "DMCA takedown notice confirmed" --yes --json --non-interactive --no-input
gumroad admin products flag-for-tos-violation <product-id> --user-id 2245593582708 --expected-email seller@example.com --yes --json --non-interactive --no-input
gumroad admin payouts scheduled create --user-id 2245593582708 --expected-email seller@example.com --processor stripe --payout-date 2026-06-15 --note "Appeal window closes before payout." --yes --json --non-interactive --no-input
gumroad admin payouts scheduled list --status pending --user-id 2245593582708 --json --non-interactive --no-input

# Refund-balance dry-run still calls the preview GET, but skips the guarded POST.
gumroad admin users refund-balance --user-id 2245593582708 --expected-email seller@example.com --dry-run --json --non-interactive --no-input
gumroad admin users refund-balance --user-id 2245593582708 --expected-email seller@example.com --yes --json --non-interactive --no-input

# Inspect purchase and product fraud context
gumroad admin purchases view <purchase-id> --with-clusters --json --non-interactive --no-input
gumroad admin purchases search --email buyer@example.com --json --jq '{purchases, has_more, limit}' --non-interactive --no-input
gumroad admin purchases lookup --stripe-fingerprint fp_abc --limit 25 --json --non-interactive --no-input
gumroad admin products list --email seller@example.com --page 2 --per-page 25 --json --non-interactive --no-input
gumroad admin products view <product-id> --with-fraud-context --json --non-interactive --no-input

# Watchlist state does not pause payouts or change user risk state
gumroad admin users watch --user-id 2245593582708 --expected-email seller@example.com --revenue-threshold 200 --note "Review next buyers" --yes --json --non-interactive --no-input
gumroad admin users update-watch --user-id 2245593582708 --expected-email seller@example.com --revenue-threshold 500 --yes --json --non-interactive --no-input
gumroad admin users update-watch --user-id 2245593582708 --expected-email seller@example.com --revenue-threshold 500 --clear-note --yes --json --non-interactive --no-input
gumroad admin users unwatch --user-id 2245593582708 --expected-email seller@example.com --yes --json --non-interactive --no-input
```

### products — Manage products

```sh
# List all products
gumroad products list --json --no-input

# View a product
gumroad products view <id> --json --no-input

# Create a product (created as draft)
gumroad products create --name "Art Pack" --price 10.00 --json --no-input
gumroad products create --name "Art Pack" --price 10.00 --file ./pack.zip --file-name "Art Pack.zip" --json --no-input
gumroad products create --name "Art Pack" --price 10.00 --cover-image ./cover.jpg --thumbnail ./thumb.jpg --json --no-input
gumroad products create --name "Newsletter" --type membership --subscription-duration monthly --json --no-input
gumroad products create --name "E-Book" --type ebook --price 5 --tag art --tag digital --json --no-input

# Update a product
gumroad products update <id> --name "New Name" --json --no-input
gumroad products update <id> --price 15.00 --currency eur --json --no-input
gumroad products update <id> --file ./pack.zip --json --no-input
gumroad products update <id> --cover-image ./cover.jpg --json --no-input
gumroad products update <id> --preview-image ./gallery-1.jpg --preview-image ./gallery-2.jpg --json --no-input
gumroad products update <id> --thumbnail ./thumb.jpg --json --no-input
gumroad products update <id> --replace-files --keep-file file_123 --file ./new-pack.zip --yes --json --no-input
gumroad products update <id> --remove-file file_456 --yes --json --no-input

# Product covers and thumbnail
gumroad products covers add <id> --image ./cover.jpg --json --no-input
gumroad products covers add <id> --url https://www.youtube.com/watch?v=qKebcV1jv3A --json --no-input
gumroad products covers reorder <id> <cover_id> <cover_id> --json --no-input
gumroad products covers remove <id> <cover_id> --yes --json --no-input
gumroad products thumbnail set <id> --image ./thumb.jpg --json --no-input
gumroad products thumbnail remove <id> --yes --json --no-input

# Publish / unpublish
gumroad products publish <id> --json --no-input
gumroad products unpublish <id> --json --no-input

# Delete (destructive — needs --yes)
gumroad products delete <id> --yes --json --no-input

# List SKUs for a product
gumroad products skus <id> --json --no-input
```

**Create flags:** `--name` (required), `--price`, `--type` (digital|course|ebook|membership|bundle|coffee|call|commission), `--currency`, `--pay-what-you-want`, `--suggested-price`, `--description`, `--custom-summary`, `--custom-permalink`, `--custom-receipt`, `--max-purchase-count`, `--taxonomy-id`, `--tag` (repeatable), `--file` (repeatable), `--file-name` (repeatable, aligned to `--file`), `--file-description` (repeatable, aligned to `--file`), `--cover-image`, `--preview-image` (repeatable), `--thumbnail`.

**Update flags:** `--name`, `--price`, `--currency`, `--description`, `--custom-summary`, `--custom-permalink`, `--custom-receipt`, `--max-purchase-count`, `--taxonomy-id`, `--tag` (repeatable), `--file` (repeatable), `--file-name`, `--file-description`, `--remove-file` (repeatable), `--replace-files`, `--keep-file` (repeatable with `--replace-files`), `--cover-image`, `--preview-image` (repeatable), `--thumbnail`. Updates preserve existing files by default unless `--replace-files` is set.

Use `products update --file` for shared product Content. For products with per-variant Content, use `variants update ... --file` for the specific variant you want to change.

Use `--cover-image` for the primary cover, repeat `--preview-image` for additional gallery/preview images, and `--thumbnail` for the card/library thumbnail. These media flags run the required two-step API flow: direct upload first, then attach by signed blob ID.

### files — Upload and recover file attachments

```sh
# Upload a file and print the canonical Gumroad URL
gumroad files upload ./pack.zip --json --no-input
gumroad files upload ./pack.zip --name "Art Pack.zip" --json --no-input

# Finalize a saved recovery manifest after a state-unknown upload
gumroad files complete --recovery recovery.json --yes --json --no-input
jq '.error.recovery' err.json | gumroad files complete --recovery - --yes --json --no-input

# Abort an orphaned multipart upload
gumroad files abort --upload-id up-123 --key attachments/u/k/original/pack.zip --yes --json --no-input
```

`files upload` and `files complete` both return `.file_url`. When a JSON upload fails with recovery details, reuse `.error.recovery` with `files complete` to finish it or `files abort` to reclaim the orphaned multipart upload.

### sales — Manage sales

```sh
# List sales (paginated)
gumroad sales list --json --no-input
gumroad sales list --product <id> --after 2024-01-01 --json --no-input
gumroad sales list --email user@example.com --json --no-input
gumroad sales list --all --json --no-input
gumroad sales list --after 2024-01-01 --before 2024-01-31 --csv --no-input

# Find a sale by email
gumroad sales list --json --jq '.sales[] | select(.email == "user@example.com")' --no-input

# Show sales totals
gumroad sales summary --json --no-input
gumroad sales summary --from 2026-01-01 --to 2026-05-21 --json --no-input
gumroad sales summary --group-by product --json --no-input
gumroad sales summary --group-by month --from 2026-01-01 --to 2026-05-21 --json --no-input

# Request an emailed CSV export for larger ranges
gumroad sales export --from 2026-01-01 --to 2026-05-21 --no-input
gumroad sales export --after 2026-01-01 --before 2026-05-21 --no-input
gumroad sales export --product <id> --json --no-input

# View a sale
gumroad sales view <id> --json --no-input

# Refund (destructive — needs --yes)
gumroad sales refund <id> --yes --json --no-input
gumroad sales refund <id> --amount 5.00 --yes --json --no-input

# Resend receipt
gumroad sales resend-receipt <id> --json --no-input
```

**List filters/output:** `--product`, `--order`, `--email`, `--after` (YYYY-MM-DD), `--before` (YYYY-MM-DD), `--all`, `--page-key`, `--csv`.
**Summary filters:** `--from` (YYYY-MM-DD), `--to` (YYYY-MM-DD), `--group-by` (product|day|week|month).
**Export filters:** `--from`/`--after` (YYYY-MM-DD), `--to`/`--before` (YYYY-MM-DD), `--product`.

### payouts — View payouts

```sh
# List payouts
gumroad payouts list --json --no-input
gumroad payouts list --after 2024-01-01 --before 2024-12-31 --json --no-input
gumroad payouts list --all --json --no-input

# View a payout
gumroad payouts view <id> --json --no-input
gumroad payouts view <id> --include-transactions --json --no-input

# Upcoming payout
gumroad payouts upcoming --json --no-input
```

**List filters:** `--after`, `--before`, `--all`, `--page-key`, `--no-upcoming`.
**View flags:** `--include-transactions`, `--no-sales`.

### subscribers — View subscribers

```sh
gumroad subscribers list --product <id> --json --no-input
gumroad subscribers list --product <id> --email user@example.com --json --no-input
gumroad subscribers list --product <id> --all --json --no-input
gumroad subscribers view <id> --json --no-input
```

**List flags:** `--product` (required), `--email`, `--all`, `--page-key`.

### licenses — Manage license keys

License keys are passed via stdin. Never pass keys as command-line arguments.

```sh
# Verify without incrementing use count
echo "$LICENSE_KEY" | gumroad licenses verify --product <id> --no-increment --json --no-input

# Verify (increments use count)
echo "$LICENSE_KEY" | gumroad licenses verify --product <id> --json --no-input

# Enable / disable
echo "$LICENSE_KEY" | gumroad licenses enable --product <id> --json --no-input
echo "$LICENSE_KEY" | gumroad licenses disable --product <id> --json --no-input

# Decrement use count
echo "$LICENSE_KEY" | gumroad licenses decrement --product <id> --json --no-input

# Rotate (regenerate) key
echo "$LICENSE_KEY" | gumroad licenses rotate --product <id> --json --no-input
```

**All subcommands require** `--product <id>`. Key comes from stdin.

### offer-codes — Manage discount codes

```sh
# List offer codes for a product
gumroad offer-codes list --product <id> --json --no-input

# Create (percent or flat, not both)
gumroad offer-codes create --product <id> --name SAVE10 --percent-off 10 --json --no-input
gumroad offer-codes create --product <id> --name FLAT5 --amount 5.00 --json --no-input

# View / update / delete
gumroad offer-codes view <code_id> --product <id> --json --no-input
gumroad offer-codes update <code_id> --product <id> --max-purchase-count 100 --json --no-input
gumroad offer-codes delete <code_id> --product <id> --yes --json --no-input
```

**Create flags:** `--product` (required), `--name` (required), `--percent-off` OR `--amount`, `--max-purchase-count`, `--universal`.

### variant-categories — Manage variant categories

```sh
gumroad variant-categories list --product <id> --json --no-input
gumroad variant-categories create --product <id> --title "Size" --json --no-input
gumroad variant-categories view <cat_id> --product <id> --json --no-input
gumroad variant-categories update <cat_id> --product <id> --title "Color" --json --no-input
gumroad variant-categories delete <cat_id> --product <id> --yes --json --no-input
```

### variants — Manage variants within a category

```sh
gumroad variants list --product <id> --category <cat_id> --json --no-input
gumroad variants create --product <id> --category <cat_id> --name "Large" --json --no-input
gumroad variants create --product <id> --category <cat_id> --name "XL" --price-difference 5.00 --json --no-input
gumroad variants view <var_id> --product <id> --category <cat_id> --json --no-input
gumroad variants update <var_id> --product <id> --category <cat_id> --name "Medium" --json --no-input
gumroad variants update <var_id> --product <id> --category <cat_id> --file ./license.pdf --json --no-input
gumroad variants delete <var_id> --product <id> --category <cat_id> --yes --json --no-input
```

**All subcommands require** `--product` and `--category`.

**Update flags:** `--name`, `--description`, `--price-difference`, `--max-purchase-count`, `--file` (repeatable), `--file-name`, `--file-description`. Use `variants update --file` only for products with per-variant Content; for shared Content, attach at the product level with `products update --file`.

### custom-fields — Manage custom fields

Custom fields are keyed by name, not ID.

```sh
gumroad custom-fields list --product <id> --json --no-input
gumroad custom-fields create --product <id> --name "Company" --required --json --no-input
gumroad custom-fields update --product <id> --name "Company" --required --json --no-input
gumroad custom-fields delete --product <id> --name "Company" --yes --json --no-input
```

### webhooks — Manage webhooks

```sh
# List (--resource is required)
gumroad webhooks list --resource sale --json --no-input

# Create
gumroad webhooks create --resource sale --url https://example.com/hook --json --no-input

# Delete
gumroad webhooks delete <id> --yes --json --no-input
```

## Tips

- Use `--all` with `sales list`, `subscribers list`, `payouts list` to fetch every page automatically.
- Use `--plain` for tab-separated output suitable for `cut`, `awk`, and other Unix tools.
- Run `gumroad <command> --help` for full flag details on any command.
