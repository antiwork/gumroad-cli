package skills

import (
	"os"
	"strings"
	"testing"
)

func TestSkillMarkdown_ReturnsContent(t *testing.T) {
	data, err := SkillMarkdown()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty skill content")
	}

	content := string(data)
	if !strings.Contains(content, "name: gumroad") {
		t.Error("expected frontmatter with name")
	}
	if !strings.Contains(content, "gumroad products list") {
		t.Error("expected command examples")
	}
}

func TestEmbeddedSkillsDirectoryContainsOnlyGumroad(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read embedded skills directory: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != "gumroad" {
			t.Errorf("unexpected embedded skill directory %q", entry.Name())
		}
	}
}

func TestSkillMarkdown_ContainsAllCommands(t *testing.T) {
	data, err := SkillMarkdown()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content := string(data)
	for _, cmd := range []string{"auth", "user", "products", "sales", "payouts", "subscribers", "licenses", "offer-codes", "variants", "custom-fields", "webhooks"} {
		if !strings.Contains(content, cmd) {
			t.Errorf("expected skill to mention command %q", cmd)
		}
	}
}

func TestSkillMarkdown_ContainsSalesExamples(t *testing.T) {
	data, err := SkillMarkdown()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content := string(data)
	for _, example := range []string{
		"gumroad sales list --after 2024-01-01 --before 2024-01-31 --csv --no-input",
		"gumroad sales summary --group-by product --json --no-input",
		"gumroad sales summary --group-by month --from 2026-01-01 --to 2026-05-21 --json --no-input",
		"gumroad sales export --from 2026-01-01 --to 2026-05-21 --no-input",
		"gumroad sales export --after 2026-01-01 --before 2026-05-21 --no-input",
		"gumroad sales export --product <id> --json --no-input",
		"`sales summary` → `.gross_cents`, `.net_cents`, `.breakdown[]`",
		"`sales export` → `.status`, `.recipient_email`",
		"`--product`, `--order`, `--email`, `--after` (YYYY-MM-DD), `--before` (YYYY-MM-DD), `--all`, `--page-key`, `--csv`",
		"`--from` (YYYY-MM-DD), `--to` (YYYY-MM-DD), `--group-by` (product|day|week|month)",
		"`--from`/`--after` (YYYY-MM-DD), `--to`/`--before` (YYYY-MM-DD), `--product`",
	} {
		if !strings.Contains(content, example) {
			t.Errorf("expected skill to mention sales example %q", example)
		}
	}
}

func TestSkillMarkdown_ContainsVariantFileAttachExamples(t *testing.T) {
	data, err := SkillMarkdown()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content := string(data)
	for _, example := range []string{
		"gumroad variants update <var_id> --product <id> --category <cat_id> --file ./license.pdf --json --no-input",
		"`products update --file` for shared product Content",
		"`variants update --file` only for products with per-variant Content",
	} {
		if !strings.Contains(content, example) {
			t.Errorf("expected skill to mention variant file attach example %q", example)
		}
	}
}

func TestSkillMarkdown_ContainsProductMediaAndBulkGuidance(t *testing.T) {
	data, err := SkillMarkdown()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content := string(data)
	for _, example := range []string{
		"gumroad products create --name \"Art Pack\" --price 10.00 --cover-image ./cover.jpg --thumbnail ./thumb.jpg --json --no-input",
		"gumroad products update <id> --preview-image ./gallery-1.jpg --preview-image ./gallery-2.jpg --json --no-input",
		"gumroad products covers add <id> --image ./cover.jpg --json --no-input",
		"gumroad products thumbnail set <id> --image ./thumb.jpg --json --no-input",
		"gumroad products thumbnail set <id> --url https://example.com/thumb.png --json --no-input",
		"WebP is not supported by the API",
		"`products update` with media flags → `.product` plus `.media[]`",
		"`products covers add --url` → `.result.covers[]`, `.result.main_cover_id`",
		"`products thumbnail set --url` → `.result.thumbnail`",
		"Check existing products and permalinks first",
		"Continue past per-product errors",
	} {
		if !strings.Contains(content, example) {
			t.Errorf("expected skill to mention product media or bulk guidance %q", example)
		}
	}
}

func TestSkillMarkdown_ContainsAdminRolloutCommands(t *testing.T) {
	data, err := SkillMarkdown()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content := string(data)
	for _, example := range []string{
		"gumroad admin users info --email",
		"gumroad admin users affiliates --user-id",
		"gumroad admin users comments list --user-id",
		"gumroad admin users comments add --user-id",
		"gumroad admin users compliance --user-id",
		"gumroad admin users purchases --user-id",
		"gumroad admin users related --email",
		"gumroad admin users suspend-for-tos-violation --user-id",
		"`admin users mark-compliant`, `admin users suspend`, `admin users suspend-for-tos-violation` → `.status`, `.message`, `.user_id`",
		"gumroad admin products flag-for-tos-violation <product-id> --user-id",
		"`admin products flag-for-tos-violation` → `.status`, `.message`, `.user_id`, `.product_id`",
		"gumroad admin payouts scheduled create --user-id",
		"`admin payouts scheduled create` → `.message`, `.user_id`, `.scheduled_payout`",
		"gumroad admin users refund-balance --user-id",
		"`admin users refund-balance` → `.status`, `.message`, `.user_id`, `.count`, `.total_amount_cents`, `.currency`",
		"gumroad admin purchases view <purchase-id> --with-clusters",
		"gumroad admin purchases search --email",
		"gumroad admin purchases lookup --stripe-fingerprint",
		"gumroad admin products list --email",
		"gumroad admin products view <product-id> --with-fraud-context",
		"Cursor-paginated: `admin users affiliates`",
		"Page-paginated: `admin products list`",
		"Capped, not continuable: `admin users related`",
		"Capped, not continuable: `admin purchases search`",
		".truncated",
		".has_more",
	} {
		if !strings.Contains(content, example) {
			t.Errorf("expected skill to mention admin example %q", example)
		}
	}
}
