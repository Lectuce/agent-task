package teams

import (
	"agent/config"
	"agent/task"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

type toolHandler func(map[string]any) (string, error)

var BUS *MessageBus

func SetMessageBus(bus *MessageBus) {
	BUS = bus
}

func runBash(input map[string]any) (string, error) {
	// command string
	command := input["command"].(string)
	dangerous := []string{"rm -rf /", "sudo", "shutdown", "reboot", "> /dev/"}
	for _, danger := range dangerous {
		if strings.Contains(command, danger) {
			return "", fmt.Errorf("Dangerous command blocked")
		}
	}
	ctx, cancel := context.WithTimeout(
		context.Background(),
		120*time.Second,
	)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = config.WORKDIR
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("Timeout (120s)")
	}
	result := strings.TrimSpace(string(out))
	if result == "" {
		if err != nil {
			return "", err
		}
		return "(no output)", nil
	}
	if len(result) > 50000 {
		result = result[:50000]
	}
	return result, nil

}

func runRead(input map[string]any) (string, error) {
	// path string, limit int
	path, err := safePath(input)
	if err != nil {
		return "", err
	}
	limit := 0

	if v, exists := input["limit"]; exists {
		switch n := v.(type) {
		case float64:
			limit = int(n)
		case int:
			limit = n
		default:
			return "", fmt.Errorf("limit must be a number")
		}
	}

	file, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	text := string(file)
	if limit > 0 && len(file) > limit {
		text = string(text)[:limit]
	}
	return text, nil
}

func runWrite(input map[string]any) (string, error) {
	// path string, content string
	path, err := safePath(input)
	if err != nil {
		return "", err
	}

	content := input["content"].(string)
	err = os.WriteFile(path, []byte(content), 0644)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("wrote %v bytes to %v\n", len([]byte(content)), path), nil
}

func safePath(input map[string]any) (string, error) {
	path, ok := input["path"].(string)
	if !ok {
		return "", fmt.Errorf("path is required")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(config.WORKDIR, path)
	}
	path = filepath.Clean(path)

	rel, err := filepath.Rel(config.WORKDIR, path)
	if err != nil {
		return "", err
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes workspace: %s", path)
	}

	return path, nil
}

func runSendMessage(input map[string]any) (string, error) {
	// to string, content string

	to, ok := input["to"].(string)
	if !ok || to == "" {
		return "", fmt.Errorf("to is required.")
	}
	content, ok := input["content"].(string)
	if !ok || content == "" {
		return "", fmt.Errorf("content is required.")
	}
	err := BUS.Send("lead", to, content, "message", map[string]any{})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("send to %v", to), nil
}

func runListTasks(input map[string]any) (string, error) {
	tasks, err := task.ListTasks()
	if err != nil {
		return "", err
	}

	if len(tasks) == 0 {
		return "No tasks.", nil
	}

	result := ""
	for _, t := range tasks {
		result += "\n" + fmt.Sprintf("  %v: %v [%v]", t.ID, t.Subject, t.Status)
	}

	return result, nil

}

func runClaimTask(input map[string]any) (string, error) {

	taskID, ok := input["task_id"].(string)
	if !ok || taskID == "" {
		return "", fmt.Errorf("task_id is required.")
	}
	owner, ok := input["owner"].(string)
	if !ok || owner == "" {
		return "", fmt.Errorf("owner is required.")
	}

	return task.ClaimTask(taskID, owner)
}

func runCompleteTask(input map[string]any) (string, error) {
	taskID, ok := input["task_id"].(string)
	if !ok || taskID == "" {
		return "", fmt.Errorf("task_id is required.")
	}

	return task.CompleteTask(taskID)

}

func buildTools(subTools []anthropic.ToolParam) []anthropic.ToolUnionParam {
	tools := make([]anthropic.ToolUnionParam, 0, len(subTools))

	for i := range subTools {
		toolParam := subTools[i]

		tools = append(tools,
			anthropic.ToolUnionParam{
				OfTool: &toolParam,
			},
		)
	}

	return tools
}
