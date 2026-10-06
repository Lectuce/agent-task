package tool

import (
	"agent/task"
	"fmt"
	"os"
	"strings"
)

func runCreateTask(input map[string]any) (string, error) {
	subject, ok := input["subject"].(string)
	if !ok || subject == "" {
		return "", fmt.Errorf("subject is required")
	}

	description, _ := input["description"].(string)

	blockedBy := make([]string, 0)
	raw, ok := input["blocked_by"].([]any)
	if ok {
		for _, item := range raw {
			s, ok := item.(string)
			if ok {
				blockedBy = append(blockedBy, s)
			}
		}
	}

	task, err := task.CreateTask(subject, description, blockedBy)
	if err != nil {
		return "", err
	}
	deps := ""
	if len(blockedBy) > 0 {
		deps = fmt.Sprintf(" (blockedBy: %s)", strings.Join(blockedBy, ", "))
	}
	fmt.Printf("  \033[34m[create] %v%v\033[0m", task.Subject, deps)
	return fmt.Sprintf("Create %v: %v%v\n", task.ID, task.Subject, deps), nil
}

func runListTasks(input map[string]any) (string, error) {
	tasks, err := task.ListTasks()
	if err != nil {
		return "", err
	}
	if len(tasks) == 0 {
		return "", fmt.Errorf("No tasks. Use create_task to add some.")
	}
	lines := make([]string, 0)
	for _, t := range tasks {
		icon := "?"

		switch t.Status {
		case task.StatusPending:
			icon = "○"

		case task.StatusInProgress:
			icon = "●"

		case task.StatusCompleted:
			icon = "✓"
		}

		deps := ""
		if len(t.BlockedBy) > 0 {
			deps = fmt.Sprintf(
				" (blockedBy: %s)",
				strings.Join(
					t.BlockedBy,
					", ",
				),
			)
		}

		owner := ""
		if t.Owner != nil {
			owner = fmt.Sprintf(" [%s]", *t.Owner)
		}

		line := fmt.Sprintf(
			"  %s %s: %s [%s]%s%s",
			icon,
			t.ID,
			t.Subject,
			t.Status,
			owner,
			deps,
		)

		lines = append(lines, line)
	}

	return strings.Join(lines, "\n"), nil
}

func runGetTask(input map[string]any) (string, error) {
	taskID, ok := input["task_id"].(string)
	if !ok || taskID == "" {
		return "", fmt.Errorf("task_id is required")
	}
	result, err := task.GetTasks(taskID)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("Error: Task %s not found", taskID), nil
		}
		return "", err
	}
	return result, nil

}

func runClaimTask(input map[string]any) (string, error) {
	taskID, ok := input["task_id"].(string)
	if !ok || taskID == "" {
		return "", fmt.Errorf("task_id is required")
	}
	return task.ClaimTask(taskID, "agent")
}

func runCompleteTask(input map[string]any) (string, error) {
	taskID, ok := input["task_id"].(string)
	if !ok || taskID == "" {
		return "", fmt.Errorf("task_id is required")
	}
	return task.CompleteTask(taskID)
}
