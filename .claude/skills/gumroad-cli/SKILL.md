---
name: gumroad-cli
description: Use the `gumroad` CLI to look up and manage Gumroad data from the terminal. Trigger when the user asks about Gumroad products, sales, subscribers, licenses, payouts, offer codes, webhooks, or any Gumroad data lookup. Also trigger on "check my Gumroad", "look up a sale", "verify a license", "list my products", or any request to query or act on Gumroad data. Do NOT trigger for Gumroad web UI, Rails, or codebase questions.
---

# gumroad CLI

Use `gumroad` (Gumroad CLI) to query and manage Gumroad data. Always use `--json` for programmatic access and `--no-input` to prevent interactive prompts from hanging.

## Key flags

```
--json          Structured JSON output (always use this)
--jq <expr>     Filter JSON (avoids parsing full response)
--no-input      Fail instead of prompting (prevents hangs)
--yes           Auto-confirm destructive ops (delete, refund)
--quiet         Suppress spinners and status messages
```

## Common patterns

```sh
# Check auth status
gumroad auth status --no-input

# List all products
gumroad products list --json --no-input

# Create a product (created as draft)
gumroad products create --name "Art Pack" --price 10.00 --json --no-input

# Get a specific product
gumroad products view <id> --json --no-input

# Find a sale by email
gumroad sales list --json --jq '.sales[] | select(.email == "user@example.com")' --no-input

# Get recent sales for a product
gumroad sales list --product <id> --after 2026-01-01 --json --no-input

# Extract a single field
gumroad user --json --jq '.user.email' --no-input

# Verify a license key
gumroad licenses verify --product <id> --key <key> --no-increment --json --no-input

# List subscribers
gumroad subscribers list --product <id> --json --no-input

# Check upcoming payouts
gumroad payouts upcoming --json --no-input
```

## Available commands

```
auth           login, status, logout
user           Account info
products       create, list, view, delete, publish, unpublish, skus
sales          list, view, refund, ship, resend-receipt
payouts        list, view, upcoming
subscribers    list, view
licenses       verify, enable, disable, decrement, rotate
offer-codes    list, view, create, update, delete
variant-categories  list, view, create, update, delete
variants       list, view, create, update, delete
custom-fields  list, create, update, delete
webhooks       list, create, delete
```

## Important notes

- Always include `--no-input` to prevent the CLI from blocking on interactive prompts.
- Use `--json --jq` together to extract exactly what you need without parsing.
- Products are created as drafts — use `gumroad products publish <id>` to publish.
- If `gumroad auth status` fails, the user needs to run `gumroad auth login` interactively.
- For destructive operations (delete, refund), add `--yes` to skip confirmation.
- Run `gumroad <command> --help` for flag details on any specific command.
