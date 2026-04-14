---
name: gumroad
description: >
  Use the `gumroad` CLI to look up and manage Gumroad data from the terminal.
  Trigger when the user asks about Gumroad products, sales, subscribers,
  licenses, payouts, offer codes, webhooks, or any Gumroad data lookup.
  Also trigger on "check my Gumroad", "look up a sale", "verify a license",
  "list my products", "how much have I made", "who bought", "recent sales",
  "refund a sale", "create a product", "manage webhooks", "check my earnings",
  "see my revenue", "who subscribed", "manage my store", "discount code",
  "coupon", "shipping status", "payout schedule", or any request to query
  or act on Gumroad data — even if the user doesn't say "Gumroad" explicitly
  but is clearly referring to their creator store or digital product sales.
  Do NOT trigger for Gumroad web UI, Rails, or codebase questions.
---

# gumroad CLI

Use `gumroad` (Gumroad CLI) to query and manage Gumroad data.

## Agent invariants

Always follow these rules:

- **Always** pass `--no-input` to prevent interactive prompts from blocking.
- **Always** pass `--json` for programmatic access.
- Use `--json --jq <expr>` together to extract exactly what you need.
- For destructive operations (delete, refund), add `--yes` to skip confirmation.
- Pass `--quiet` to suppress spinners and status messages.
- Pass `--dry-run` to preview mutating requests without executing them.
- Use `--page-delay 200ms` with `--all` to avoid rate limits on large datasets.
- Prices are in whole currency units (e.g. `--price 10.00` for $10), not cents. The CLI converts internally. Use `--currency eur` to change currency.
- Products are created as drafts — use `gumroad products publish <id>` to make them live.
- If a command fails with an auth error, tell the user to run `gumroad auth login` interactively — agents cannot do this step.

## Response shapes

Responses are wrapped in `{"success": true, ...}` with resource-specific keys:

- `user` → `.user`
- `products list` → `.products[]`
- `products view` → `.product`
- `sales list` → `.sales[]`
- `sales view` → `.sale`
- `payouts list` → `.payouts[]`, `payouts view/upcoming` → `.payout`
- `subscribers list` → `.subscribers[]`, `subscribers view` → `.subscriber`
- `licenses verify` → `.purchase`
- `offer-codes list` → `.offer_codes[]`
- `variant-categories list` → `.variant_categories[]`
- `variants list` → `.variants[]`
- `webhooks list` → `.resource_subscriptions[]`

## Commands

### auth — Manage authentication

```sh
# Check auth (do this first if unsure)
gumroad auth status --no-input

# Login requires interactive input — tell the user to run it themselves
# gumroad auth login

# Logout
gumroad auth logout --no-input
```

### user — Account info

```sh
gumroad user --json --no-input
gumroad user --json --jq '.user.email' --no-input
```

### products — Manage products

```sh
# List all products
gumroad products list --json --no-input

# View a product
gumroad products view <id> --json --no-input

# Create a product (created as draft)
gumroad products create --name "Art Pack" --price 10.00 --json --no-input
gumroad products create --name "Newsletter" --type membership --subscription-duration monthly --json --no-input
gumroad products create --name "E-Book" --type ebook --price 5 --tag art --tag digital --json --no-input

# Update a product
gumroad products update <id> --name "New Name" --json --no-input
gumroad products update <id> --price 15.00 --currency eur --json --no-input

# Publish / unpublish
gumroad products publish <id> --json --no-input
gumroad products unpublish <id> --json --no-input

# Delete (destructive — needs --yes)
gumroad products delete <id> --yes --json --no-input

# List SKUs for a product
gumroad products skus <id> --json --no-input
```

**Create flags:** `--name` (required), `--price`, `--type` (digital|course|ebook|membership|bundle|coffee|call|commission), `--currency`, `--pay-what-you-want`, `--suggested-price`, `--description`, `--custom-summary`, `--custom-permalink`, `--custom-receipt`, `--max-purchase-count`, `--taxonomy-id`, `--tag` (repeatable).

### sales — Manage sales

```sh
# List sales (paginated)
gumroad sales list --json --no-input
gumroad sales list --product <id> --after 2024-01-01 --json --no-input
gumroad sales list --email user@example.com --json --no-input
gumroad sales list --all --json --no-input

# Find a sale by email
gumroad sales list --json --jq '.sales[] | select(.email == "user@example.com")' --no-input

# View a sale
gumroad sales view <id> --json --no-input

# Refund (destructive — needs --yes)
gumroad sales refund <id> --yes --json --no-input
gumroad sales refund <id> --amount 5.00 --yes --json --no-input

# Resend receipt
gumroad sales resend-receipt <id> --json --no-input
```

**List filters:** `--product`, `--order`, `--email`, `--after` (YYYY-MM-DD), `--before` (YYYY-MM-DD), `--all`, `--page-key`.

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
gumroad variants delete <var_id> --product <id> --category <cat_id> --yes --json --no-input
```

**All subcommands require** `--product` and `--category`.

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
