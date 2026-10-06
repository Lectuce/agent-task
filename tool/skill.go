package tool

import (
	"agent/skills"
	"fmt"
)

func runloadSkill(input map[string]any) (string, error) {
	name, err := requireString(input, "name", "name is required")
	if err != nil {
		return "", err
	}
	skill, ok := skills.SkillRegistry[name]
	if !ok {
		return "", fmt.Errorf("Skill not found: %v\n", skill)
	}
	return skill.Content, nil

}
