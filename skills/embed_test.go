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
