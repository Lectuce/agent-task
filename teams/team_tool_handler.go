package teams

import (
	"agent/config"
	"agent/task"
	"bufio"
	"bytes"
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
	// path, err := safePath(input)
	// if err != nil {
	// 	return "", err
	// }
	// limit := 0

	// if v, exists := input["limit"]; exists {
	// 	switch n := v.(type) {
	// 	case float64:
	// 		limit = int(n)
	// 	case int:
	// 		limit = n
	// 	default:
	// 		return "", fmt.Errorf("limit must be a number")
	// 	}
	// }

	// file, err := os.ReadFile(path)
	// if err != nil {
	// 	return "", err
	// }
	// text := string(file)
	// if limit > 0 && len(file) > limit {
	// 	text = string(text)[:limit]
	// }
	// return text, nil

	return runReadAt(input, config.WORKDIR)
}

func runWrite(input map[string]any) (string, error) {
	// path string, content string
	// path, err := safePath(input)
	// if err != nil {
	// 	return "", err
	// }

	// content := input["content"].(string)
	// err = os.WriteFile(path, []byte(content), 0644)
	// if err != nil {
	// 	return "", err
	// }
	// return fmt.Sprintf("wrote %v bytes to %v\n", len([]byte(content)), path), nil

	return runWriteAt(input, config.WORKDIR)
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

func safePathAt(input map[string]any, cwd string) (string, error) {

	path, ok := input["path"].(string)

	var base string
	if cwd == "" {
		base = config.WORKDIR
	} else {
		base = cwd
	}

	if !ok {
		return "", fmt.Errorf("path is required")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	path = filepath.Clean(path)

	rel, err := filepath.Rel(base, path)
	if err != nil {
		return "", err
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes workspace: %s", path)
	}

	return path, nil

}

func runBashAt(input map[string]any, cwd string) (string, error) {

	command, ok := input["command"].(string)
	if !ok || command == "" {
		return "", fmt.Errorf("command is required.")
	}

	ctx, cancel := context.WithTimeout(context.Background(),
		120*time.Second,
	)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	if cwd == "" {
		cmd.Dir = config.WORKDIR
	} else {
		cmd.Dir = cwd
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}

	lines := string(out)
	lines = strings.TrimSpace(lines)

	rows := []rune(lines)
	result := ""
	if len(rows) > 5000 {
		rows = rows[:5000]
	}
	result = string(rows)

	if ctx.Err() == context.DeadlineExceeded {
		return result, fmt.Errorf("Timeout (120s)")
	}

	return result, nil

}

func runReadAt(input map[string]any, cwd string) (string, error) {

	path, err := safePathAt(input, cwd)
	if err != nil {
		return "", err
	}

	limit := 0

	if value, exists := input["limit"]; exists {
		switch number := value.(type) {
		case float64:
			limit = int(number)
		case int:
			limit = number
		default:
			return "", fmt.Errorf("limit must be a number")
		}

		if limit < 0 {
			return "", fmt.Errorf("limit cannot be negative")
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	scanner := bufio.NewScanner(
		bytes.NewReader(data),
	)
	scanner.Buffer(
		make([]byte, 64*1024),
		10*1024*1024,
	)

	lines := make([]string, 0)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scan file: %w", err)
	}

	if limit > 0 && limit < len(lines) {
		remaining := len(lines) - limit

		lines = append(
			lines[:limit],
			fmt.Sprintf("... (%d more lines)", remaining),
		)
	}

	return strings.Join(lines, "\n"), nil
}

func runWriteAt(input map[string]any, cwd string) (string, error) {

	path, err := safePathAt(input, cwd)
	if err != nil {
		return "", err
	}

	content, ok := input["content"].(string)
	if !ok {
		return "", fmt.Errorf("content is required")
	}

	err = os.WriteFile(
		path,
		[]byte(content),
		0o644,
	)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(
		"wrote %d bytes to %s\n",
		len([]byte(content)),
		path,
	), nil

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
