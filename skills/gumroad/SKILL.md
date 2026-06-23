---
name: gumroad
description: >
  Use the `gumroad` CLI to look up and manage Gumroad data from the terminal.
  Trigger when the user asks about Gumroad products, files, file uploads,
  attachments, sales, subscribers, licenses, payouts, audience emails, broadcasts, offer codes, webhooks,
  refund policies,
  or any Gumroad data lookup.
  Also trigger on "check my Gumroad", "look up a sale", "verify a license",
  "list my products", "how much have I made", "who bought", "recent sales",
  "refund a sale", "create a product", "upload a file", "attach a file to a product",
  "add a cover image", "set a product thumbnail", "get product content", "set product content", "upload product media",
  "publish a product landing page", "publish custom HTML", "clear custom HTML",
  "attach a file to a variant", "finish a failed upload", "abort an upload", "manage webhooks",
  "draft an email", "preview a broadcast", "send an audience email", "list drafts",
  "set refund policy", "check my refund policy", "check my earnings", "see my revenue", "who subscribed", "manage my store",
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
- For operations that can prompt for confirmation (delete, refund, mutating admin actions, `files abort`, `files complete` replay, product updates that remove files, or `products content set` when omitted page IDs will be deleted), add `--yes` to skip confirmation.
- Pass `--quiet` to suppress spinners and status messages.
- Pass `--dry-run` to preview mutating requests without executing them.
- Use `--page-delay 200ms` with `--all` to avoid rate limits on large datasets.
- Prices are in whole currency units (e.g. `--price 10.00` for $10), not cents. The CLI converts internally. Use `--currency eur` to change currency.
- Products are created as drafts — use `gumroad products publish <id>` to make them live.
- Product cover and thumbnail uploads support JPEG, PNG, and GIF. WebP is not supported by the API and the CLI rejects it before upload.
- Product custom HTML landing pages use `gumroad products page preview <id> ./landing.html` to run the backend sanitizer without writing, `gumroad products page publish <id> ./landing.html` to store the page, `gumroad products page clear <id> --yes` to remove it, and `gumroad products page url <id>` to print the live URL. `--dry-run` only previews the CLI request body; it does not call the backend sanitizer. Inspect `.sanitization_report` in `preview` and `publish` JSON output for server-side changes.
- Product rich content uses `gumroad products content list <id> --json --no-input` to inspect page IDs, `gumroad products content get <id> --json --no-input` to dump the shared `rich_content` page array, and `gumroad products content set <id> content.json --dry-run --json --no-input` to preview a whole-document replacement. Without an explicit path, whole-document `set` reads `./content.json`; `set --page` reads `./page.json`. Use `--page <page_id>` with `get`/`set` to edit one matching page object; `set --page` still sends a merged whole-document PUT. For per-variant content, pass both `--variant <variant_id>` and `--category <cat_id>`. Whole-document `set` deletes existing pages omitted from the JSON.
- Custom HTML pages can use `data-gumroad-field="name"`, `data-gumroad-field="price"`, `data-gumroad-field="description"`, and `data-gumroad-action="buy"`. To preselect checkout state, add `data-gumroad-option="<variant name>"`, `data-gumroad-quantity="<integer>"`, `data-gumroad-price="<decimal>"`, or `data-gumroad-recurrence="monthly|quarterly|biannually|yearly|every_two_years"`. Production validates these values and falls back to product defaults when invalid. Prefer anchors for buy CTAs so production can add a checkout href; non-anchor buy elements also post to checkout.
- Audience emails are created as drafts by default. Use `gumroad emails send-preview <id> --json --no-input` and inspect `.preview_url` before `gumroad emails send <id> --yes --json --no-input`. Creating with `--send` publishes and blasts immediately, so use `--dry-run` first and require explicit human approval.
- If a command fails with a seller auth error, run `gumroad auth status --json --no-input` first. Agents can start seller auth with `gumroad auth login --no-input` and hand the printed approval URL to a human, or use an existing seller token via `GUMROAD_ACCESS_TOKEN` or `gumroad auth login --with-token`.
- For admin commands in agents/CI, pass `--non-interactive` and set `GUMROAD_ADMIN_TOKEN`; interactive shells can store an admin token with `gumroad auth login --web`.

## Response shapes

Most responses are wrapped in `{"success": true, ...}` with resource-specific keys:

- `user` → `.user`
- `refund-policy view/set` → `.refund_policy`
- `products list` → `.products[]`
- `products view` → `.product`
- `products content get` → rich content page array directly, or one page object with `--page`
- `products content list` → rich content page summary array directly
- `products content set` → mutation envelope with `.result`
- `sales list` → `.sales[]`
- `sales buyers` → `.buyers[]` (`email`, `name`, `purchase_count`, `last_purchase_date`, `utm_source`, `utm_medium`, `utm_campaign`, `utm_term`, `utm_content`)
- `sales view` → `.sale`
- `sales export` → `.status`, `.recipient_email`
- `sales summary` → `.gross_cents`, `.net_cents`, `.breakdown[]`
- `emails list` → `.emails[]`, `emails view/create/send` → `.email`, `emails send-preview` → `.preview_url`, `emails delete` → `.message`
- `payouts list` → `.payouts[]`, `payouts view/upcoming` → `.payout`
- `subscribers list` → `.subscribers[]`, `subscribers view` → `.subscriber`
- `licenses verify` → `.purchase`
- `offer-codes list` → `.offer_codes[]`
- `variant-categories list` → `.variant_categories[]`
- `variants list` → `.variants[]`
- `files upload` / `files complete` → `.file_url`
- `products create` with media flags → `.product` plus `.media[]`
- `products update` with media flags → `.product` plus `.media[]`
- `products covers add --image` → `.result.covers[]`, `.result.main_cover_id`, plus `.result.media[]`
- `products covers add --url` → `.result.covers[]`, `.result.main_cover_id`
- `products thumbnail set --image` → `.result.thumbnail`, plus `.result.media[]`
- `products thumbnail set --url` → `.result.thumbnail`
- `products page preview` → `.custom_html`, `.sanitization_report`
- `products page publish` / `products page clear` → `.product.custom_html`, `.product.landing_url`, `.previous_custom_html`, `.sanitization_report`
- `products update --custom-html` → `.product.custom_html`, `.product.landing_url`, `.previous_custom_html`, `.sanitization_report`
- Not every `products` write verb is flat: `create`, `update`, `unpublish`, and `delete` return top-level fields, but `covers add`, `thumbnail set`, and `content set` still wrap their payload in the `{success, …, result}` envelope — read those under `.result`
- `webhooks list` → `.resource_subscriptions[]`
- `admin users info` → `.user` (includes `.user.stripe` Stripe Connect state — `connected`, and when connected `stripe_connect_account_id`, `stripe_dashboard_url`, and a `verification` block of flags/counts or an `error` subfield when the live Stripe lookup failed — and `.user.admin_links` impersonate/user/purchases/stripe-dashboard URLs)
- `admin users affiliates` → `.affiliates[]`
- `admin users comments list` → `.comments[]`
- `admin users comments add` → `.comment`
- `admin users compliance` → `.compliance_info`, `.info_requests[]`
- `admin users credits add` → `.user_id`, `.credit.id`, `.credit.amount_cents`, `.credit.reason`, `.credit.crediting_user_id`, `.credit.created_at`
- `admin users credits list` → `.credits[]`, `.pagination.next`
- `admin users radar` → `.radar_stats`, `.recent_efws[]`
- `admin users purchases` → `.purchases[]`
- `admin users related` → `.related_users[]`, `.truncated`, `.per_signal_limit`
- `admin users mark-compliant`, `admin users suspend`, `admin users suspend-for-tos-violation` → `.status`, `.message`, `.user_id`
- `admin products flag-for-tos-violation` → `.status`, `.message`, `.user_id`, `.product_id`
- `admin payouts list` → `.recent_payouts[]`, `.pagination.next`. Each payout carries `stripe_transfer_id` (a `po_…` payout or `py_…` destination payment, or null), `bank_account` (null for PayPal and debit-card payouts; otherwise `bank_number` routing/BIC, `account_holder_full_name`, `account_type`, `currency`), and `trace_id` (currently always null).
- `admin payouts scheduled create` → `.message`, `.user_id`, `.scheduled_payout`
- `admin users refund-balance` → `.status`, `.message`, `.user_id`, `.count`, `.total_amount_cents`, `.currency`
- `admin purchases view` → `.purchase`
- `admin purchases search` → `.purchases[]`, `.has_more`, `.limit`
- `admin purchases lookup` → `.purchases[]`
- `admin products list` → `.products[]`, `admin products view` → `.product`

Admin pagination models differ by command:

- Cursor-paginated: `admin users affiliates`, `admin users comments list`, `admin users credits list`, `admin users radar`, `admin users purchases`, and `admin purchases lookup` return `.pagination.next` as a cursor string. Pass it back with `--cursor`.
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
gumroad auth status --json --no-input

# Start device authorization and wait for human approval
gumroad auth login --no-input

# Use an existing seller token without browser approval
gumroad auth login --with-token --json --no-input < token.txt
printf '%s\n' "$GUMROAD_ACCESS_TOKEN" | gumroad auth login --with-token --json --no-input

# Print the active resolved seller token for another tool
gumroad auth token --no-input

# Force the local browser OAuth flow
gumroad auth login --web

# Logout
gumroad auth logout --yes --no-input
```

### user — Account info

```sh
gumroad user --json --no-input
gumroad user --json --jq '.user.email' --no-input
```

### refund-policy — Store-wide refund policy

```sh
# View the current account-level refund policy
gumroad refund-policy view --json --no-input
gumroad refund-policy view --json --jq '.refund_policy.in_effect' --no-input

# Set the refund period. Allowed values: none, 7, 14, 30, 183.
gumroad refund-policy set --period 30 --fine-print "Refund requests are reviewed within 2 business days." --json --no-input

# Clear fine print. This is account-level, not per-product.
gumroad refund-policy set --period none --fine-print "" --json --no-input
```

### admin — Internal admin API

```sh
# Admin commands need internal admin auth.
# In agents/CI, set GUMROAD_ADMIN_TOKEN and pass --non-interactive.

# Inspect user identity, sign-in, social, risk, payout, and watchlist state
# Look up by --email, --user-id, or --username (resolves user_id > email > username)
gumroad admin users info --email seller@example.com --json --non-interactive --no-input
gumroad admin users info --username sellerone --json --non-interactive --no-input

# Review affiliate relationships
gumroad admin users affiliates --user-id 2245593582708 --direction granted --limit 50 --json --non-interactive --no-input
gumroad admin users affiliates --email seller@example.com --direction received --cursor cur-next --json --non-interactive --no-input

# Read and add admin comments
gumroad admin users comments list --user-id 2245593582708 --type note --limit 50 --json --non-interactive --no-input
gumroad admin users comments add --user-id 2245593582708 --content "VAT exempt confirmed" --yes --json --non-interactive --no-input

# Account credits. credits add is a high-stakes write: dry-run first, then issue with explicit --yes.
# Amounts are cents, positive only, capped at $1,000 unless --allow-large-amount is explicitly passed.
gumroad admin users credits list --user-id 2245593582708 --limit 50 --json --non-interactive --no-input
gumroad admin users credits add --user-id 2245593582708 --expected-email seller@example.com --amount-cents 1000 --reason "Goodwill for checkout bug" --dry-run --json --non-interactive --no-input
gumroad admin users credits add --user-id 2245593582708 --expected-email seller@example.com --amount-cents 1000 --reason "Goodwill for checkout bug" --yes --json --non-interactive --no-input

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

# Recent payouts, with Stripe transfer id and destination bank account per row
gumroad admin payouts list --user-id 2245593582708 --limit 25 --json --jq '.recent_payouts[] | {external_id, stripe_transfer_id, bank_account}' --non-interactive --no-input

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

# Find product categories
gumroad products categories --search figma --json --no-input

# Create a product (created as draft)
gumroad products create --name "Art Pack" --price 10.00 --json --no-input
gumroad products create --name "Figma Kit" --category design/ui-and-web/figma --json --no-input
gumroad products create --name "Art Pack" --price 10.00 --file ./pack.zip --file-name "Art Pack.zip" --json --no-input
gumroad products create --name "Art Pack" --price 10.00 --cover-image ./cover.jpg --thumbnail ./thumb.jpg --json --no-input
gumroad products create --name "Newsletter" --type membership --subscription-duration monthly --json --no-input
gumroad products create --name "E-Book" --type ebook --price 5 --tag art --tag digital --json --no-input

# Update a product
gumroad products update <id> --name "New Name" --json --no-input
gumroad products update <id> --price 15.00 --currency eur --json --no-input
gumroad products update <id> --category design/ui-and-web/figma --json --no-input
gumroad products update <id> --file ./pack.zip --json --no-input
gumroad products update <id> --cover-image ./cover.jpg --json --no-input
gumroad products update <id> --preview-image ./gallery-1.jpg --preview-image ./gallery-2.jpg --json --no-input
gumroad products update <id> --thumbnail ./thumb.jpg --json --no-input
gumroad products page preview <id> ./landing.html --json --no-input
gumroad products page publish <id> ./landing.html --json --no-input
gumroad products page publish <id> - --json --no-input < landing.html
gumroad products page clear <id> --yes --json --no-input
gumroad products page url <id> --no-input
gumroad products page url <id> --json --jq '.product.landing_url' --no-input

# Product covers and thumbnail
gumroad products covers add <id> --image ./cover.jpg --json --no-input
gumroad products covers add <id> --url https://www.youtube.com/watch?v=qKebcV1jv3A --json --no-input
gumroad products covers reorder <id> <cover_id> <cover_id> --json --no-input
gumroad products covers remove <id> <cover_id> --yes --json --no-input
gumroad products thumbnail set <id> --image ./thumb.jpg --json --no-input
gumroad products thumbnail set <id> --url https://example.com/thumb.png --json --no-input
gumroad products thumbnail remove <id> --yes --json --no-input

# Shared product rich content. `set` replaces the whole page array; dry-run first.
gumroad products content list <id> --json --no-input
gumroad products content get <id> --json --no-input > content.json
gumroad products content set <id> content.json --dry-run --json --no-input
gumroad products content set <id> content.json --yes --json --no-input
gumroad products content get <id> --page <page_id> --json --no-input > page.json
gumroad products content set <id> --page <page_id> --dry-run --json --no-input
gumroad products content set <id> page.json --page <page_id> --dry-run --json --no-input

# Per-variant rich content. `--category` is required because the variant endpoint is category-scoped.
gumroad products content list <id> --variant <variant_id> --category <cat_id> --json --no-input
gumroad products content get <id> --variant <variant_id> --category <cat_id> --json --no-input > content.json
gumroad products content set <id> content.json --variant <variant_id> --category <cat_id> --dry-run --json --no-input
gumroad products content set <id> content.json --variant <variant_id> --category <cat_id> --yes --json --no-input

# Publish / unpublish
gumroad products publish <id> --json --no-input
gumroad products unpublish <id> --json --no-input

# Delete (destructive — needs --yes)
gumroad products delete <id> --yes --json --no-input

# List SKUs for a product
gumroad products skus <id> --json --no-input

```

In custom HTML, use Gumroad data attributes for live product values and checkout:

```html
<h1 data-gumroad-field="name">Product name</h1>
<span data-gumroad-field="price">$0</span>
<p data-gumroad-field="description">Product description</p>
<a data-gumroad-action="buy">Buy now</a>
<a data-gumroad-action="buy" data-gumroad-option="Pro" data-gumroad-recurrence="yearly">Buy Pro - $99/year</a>
<button data-gumroad-action="buy" data-gumroad-quantity="2">Buy 2 seats</button>
<button data-gumroad-action="buy" data-gumroad-price="19.99">Pay $19.99</button>
```

**Categories:** `products categories [--search <term>]` returns label, path, and numeric ID. Prefer `--category <path>` for product create/update. `--taxonomy-id` remains supported when you already have the numeric ID, but it cannot be combined with `--category`.

**Create flags:** `--name` (required), `--price`, `--type` (digital|course|ebook|membership|bundle|coffee|call|commission), `--currency`, `--pay-what-you-want`, `--suggested-price`, `--description`, `--custom-summary`, `--custom-permalink`, `--custom-receipt`, `--max-purchase-count`, `--category`, `--taxonomy-id`, `--tag` (repeatable), `--file` (repeatable), `--file-name` (repeatable, aligned to `--file`), `--file-description` (repeatable, aligned to `--file`), `--cover-image`, `--preview-image` (repeatable), `--thumbnail`.

**Update flags:** `--name`, `--price`, `--currency`, `--description`, `--custom-summary`, `--custom-permalink`, `--custom-receipt`, `--max-purchase-count`, `--category`, `--taxonomy-id`, `--tag` (repeatable), `--custom-html`, `--file` (repeatable), `--file-name`, `--file-description`, `--cover-image`, `--preview-image` (repeatable), `--thumbnail`. Prefer `products page preview/publish/clear/url` for custom HTML page workflows; `products update --custom-html` remains supported as a low-level product update flag.

Use `products update --file` for shared product Content. It replaces existing rich content file embeds in place when they exist, or creates file embeds when the document has none; pass one `--file` per existing file embed and use `products content get/set` for structural content edits. For products with per-variant Content, use `variants update ... --file` for the specific variant you want to change.

Use `--cover-image` for the primary cover, repeat `--preview-image` for additional gallery/preview images, and `--thumbnail` for the card/library thumbnail. These media flags run the required two-step API flow: direct upload first, then attach by signed blob ID. For an existing product, `products thumbnail set --url` asks Gumroad to download and attach a public HTTP(S) image directly.

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

### emails — Manage audience emails

```sh
# Create a draft from an HTML body file (or - for stdin). Draft is the default safety behavior.
gumroad emails create --subject "New release" --body ./email.html --json --no-input
gumroad emails create --subject "Product update" --body ./email.html --audience product --product <id> --json --no-input

# Preview before sending; use `.preview_url` for human review.
gumroad emails send-preview <id> --json --jq '.preview_url' --no-input
gumroad emails view <id> --json --no-input

# List drafts, scheduled emails, or sent emails.
gumroad emails list --state draft --json --no-input
gumroad emails list --state published --all --json --no-input

# Send or delete. Both are confirmation-gated; agents must pass --yes.
gumroad emails send <id> --yes --json --no-input
gumroad emails delete <id> --yes --json --no-input
```

**Create flags:** `--subject` (required), `--body` (required HTML file path, or `-` for stdin), `--audience` (all|customers|followers|product, default all), `--product` (required for product audience), `--send` (publish and send immediately).
**List flags:** `--state` (published|scheduled|draft), `--all`, `--page-key`.

Use `--dry-run --json --no-input` to inspect create params without calling the API. Passing `--send` blasts the audience immediately; prefer the draft → `send-preview` URL → `send` workflow. `send-preview` emails a copy to the seller. Scheduled emails can only be created in the web UI; the CLI can list and view them (`--state scheduled`) but not create them.

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

# Deduplicated buyer list for one or more products.
# JSON/CSV/plain include buyer-level last-touch UTM fields; same-timestamp ties keep the first API result.
# Use sales export for the full per-sale web CSV.
gumroad sales buyers --product <id> --json --no-input
gumroad sales buyers --product <old-id> --product <new-id> --json --no-input
gumroad sales buyers --product <id> --after 2024-01-01 --csv --no-input
gumroad sales buyers --product <id> --json --jq '.buyers[].email' --no-input

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

# Conditional discount: require a minimum order total (e.g. spend $100 -> 10% off)
gumroad offer-codes create --product <id> --name SUMMER --percent-off 10 --minimum-amount 100.00 --json --no-input

# View / update / delete
gumroad offer-codes view <code_id> --product <id> --json --no-input
gumroad offer-codes update <code_id> --product <id> --max-purchase-count 100 --json --no-input
gumroad offer-codes delete <code_id> --product <id> --yes --json --no-input
```

**Create flags:** `--product` (required), `--name` (required), `--percent-off` OR `--amount`, `--minimum-amount` (minimum order total before the discount applies, e.g. `100.00`), `--max-purchase-count`, `--universal`.

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

**Update flags:** `--name`, `--description`, `--price-difference`, `--max-purchase-count`, `--file` (repeatable), `--file-name`, `--file-description`. Use `variants update --file` only for products with per-variant Content; for shared Content, roll files at the product level with `products update --file`.

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
