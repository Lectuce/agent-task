package background

import (
	"fmt"
	"strings"
	"sync"
)

type Task struct {
	ID        string
	ToolUseID string
	Command   string
	Status    string
}

type Manager struct {
	mu      sync.Mutex
	counter int
	tasks   map[string]*Task
	results map[string]string
}

func NewManager() *Manager {
	return &Manager{
		tasks:   make(map[string]*Task, 0),
		results: make(map[string]string, 0),
	}
}

var BG = NewManager()

func IsSlowOperation(toolName string, input map[string]any) bool {
	if toolName != "bash" {
		return false
	}

	command, _ := input["command"].(string)
	command = strings.ToLower(command)

	slowKeywords := []string{
		"install",
		"build",
		"test",
		"deploy",
		"compile",
		"docker build",
		"pip install",
		"npm install",
		"cargo build",
		"pytest",
		"make",
	}
	for _, kw := range slowKeywords {
		if strings.Contains(command, kw) {
			return true
		}
	}
	return false
}

func ShouldRunBackground(toolName string, input map[string]any) bool {

	value, ok := input["run_in_background"].(bool)
	if ok && value {
		return true
	}

	return IsSlowOperation(toolName, input)
}

func (m *Manager) Start(toolUseID string, toolName string, input map[string]any, handler func(map[string]any) (string, error)) string {

	m.mu.Lock()

	m.counter++
	bgID := fmt.Sprintf("bg_%04d", m.counter)

	command := toolName
	cmd, ok := input["command"].(string)
	if ok {
		command = cmd
	}

	m.tasks[bgID] = &Task{
		ID:        bgID,
		ToolUseID: toolUseID,
		Command:   command,
		Status:    "running",
	}

	m.mu.Unlock()

	go func() {
		output, err := handler(input)

		m.mu.Lock()
		defer m.mu.Unlock()

		task, ok := m.tasks[bgID]
		if !ok {
			return
		}
		if err != nil {
			task.Status = "failed"
			m.results[bgID] = fmt.Sprintf("Error: %v", err)
			return
		}
		task.Status = "completed"
		m.results[bgID] = output
	}()

	fmt.Printf("[background] dispatched %s: %s\n", bgID, command)

	return bgID
}

func (m *Manager) CollectResults() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	var notifications []string

	for id, task := range m.tasks {
		if task.Status != "completed" {
			continue
		}

		output := m.results[id]

		summary := output
		if len(summary) > 200 {
			summary = summary[:200]
		}

		notification := fmt.Sprintf(
			"<task_notification>\n"+
				"  <task_id>%s</task_id>\n"+
				"  <status>completed</status>\n"+
				"  <command>%s</command>\n"+
				"  <summary>%s</summary>\n"+
				"</task_notification>",
			id,
			task.Command,
			summary,
		)

		notifications = append(notifications, notification)

		fmt.Printf("[background done] %s: %s\n", id, task.Command)

		delete(m.tasks, id)
		delete(m.results, id)
	}

	return notifications
}
