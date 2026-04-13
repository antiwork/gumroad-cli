package skills

import "embed"

//go:embed gumroad
var content embed.FS

func SkillMarkdown() ([]byte, error) {
	return content.ReadFile("gumroad/SKILL.md")
}
