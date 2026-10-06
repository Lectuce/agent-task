package tool

import (
	"agent/teams"
	"fmt"
)

func runSpawnTeammate(input map[string]any) (string, error) {
	name, ok := input["name"].(string)
	if !ok || name == "" {
		return "", fmt.Errorf("name is required.")
	}
	role, ok := input["role"].(string)
	if !ok || role == "" {
		return "", fmt.Errorf("role is required.")
	}
	prompt, ok := input["prompt"].(string)
	if !ok || prompt == "" {
		return "", fmt.Errorf("prompt is required.")
	}
	return teams.SpawnTeammateThread(name, role, prompt)
}

func runSendMessage(input map[string]any) (string, error) {
	// to string, content string

	to, ok := input["to"].(string)
	if !ok || to == "" {
		return "", fmt.Errorf("to is required.")
	}
	content, ok := input["content"]
	if !ok {
		return "", fmt.Errorf("content is required.")
	}
	content, ok = content.(string)
	if !ok {
		return "", fmt.Errorf("content is not string type")
	}
	MessageBus.Send("lead", to, content.(string), "string", map[string]any{})

	return fmt.Sprintf("send to %v", to), nil
}

func runCheckInbox(input map[string]any) (string, error) {
	text, count, err := teams.ConsumeLeadInboxText()
	if err != nil {
		return "", err
	}
	if count == 0 {
		return "", fmt.Errorf("(inbox empty)")
	}

	return text, nil

}
