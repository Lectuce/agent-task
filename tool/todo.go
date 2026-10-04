package tool

import (
	"encoding/json"
	"fmt"
)

type Todo struct {
	Content string `json:"content"`
	Status  string `json:"status"`
}

var CurrentTodos = []Todo{}

func runTodoWrite(input map[string]any) (string, error) {
	// todos []Todo
	raw, ok := input["todos"]
	if !ok {
		return "", fmt.Errorf("todos must be an array")
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return "", err
	}
	todos := make([]Todo, 0)
	err = json.Unmarshal(data, &todos)
	if err != nil {
		return "", err
	}
	CurrentTodos = todos
	lines := []string{"\n## Current Tasks"}
	for _, t := range CurrentTodos {
		icon := map[string]string{
			"pending":     " ",
			"in_progress": "▸",
			"completed":   "✓",
		}[t.Status]
		lines = append(lines, fmt.Sprintf("%v %v", icon, t.Content))
	}
	for _, line := range lines {
		fmt.Println(line)
	}
	return fmt.Sprintf("Updated %v tasks", len(CurrentTodos)), nil
}
