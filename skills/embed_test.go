package skills

import (
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
		"gumroad admin purchases view <purchase-id> --with-clusters",
		"gumroad admin purchases search --email",
		"gumroad admin purchases lookup --stripe-fingerprint",
		"gumroad admin products list --email",
		"gumroad admin products view <product-id> --with-fraud-context",
	} {
		if !strings.Contains(content, example) {
			t.Errorf("expected skill to mention admin example %q", example)
		}
	}
}
