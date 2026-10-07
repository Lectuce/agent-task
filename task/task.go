package task

import (
	"agent/config"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type Task struct {
	ID          string
	Subject     string
	Description string
	Status      string
	Owner       *string
	BlockedBy   []string // 当前任务依赖哪些其他任务
}

// Status
const (
	StatusPending    = "pending"
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
)

func init() {
	_ = os.MkdirAll(config.TASKS_DIR, 0755)
}

func taskPath(taskID string) string {
	return filepath.Join(config.TASKS_DIR, taskID+".json")
}

func CreateTask(subject string, description string, blockedBy []string) (*Task, error) {
	task := &Task{
		ID:          fmt.Sprintf("task_%d", time.Now().UnixNano()),
		Subject:     subject,
		Description: description,
		Status:      "pending",
		Owner:       nil,
		BlockedBy:   blockedBy,
	}
	err := SaveTask(task)
	if err != nil {
		return nil, err
	}

	return task, nil

}

func SaveTask(task *Task) error {
	err := os.MkdirAll(config.TASKS_DIR, 0755)
	if err != nil {
		return err
	}

	data, err := json.Marshal(task)
	if err != nil {
		return err
	}

	return os.WriteFile(taskPath(task.ID), data, 0644)
}

func LoadTask(taskID string) (*Task, error) {
	data, err := os.ReadFile(taskPath(taskID))
	if err != nil {
		return nil, err
	}
	var task Task
	err = json.Unmarshal(data, &task)
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func ListTasks() ([]Task, error) {
	files, err := filepath.Glob(
		filepath.Join(
			config.TASKS_DIR,
			"task_*.json",
		),
	)
	if err != nil {
		return nil, err
	}

	sort.Strings(files)

	tasks := make([]Task, 0, len(files))

	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}

		var task Task

		err = json.Unmarshal(data, &task)
		if err != nil {
			return nil, err
		}

		tasks = append(tasks, task)
	}

	return tasks, nil
}

func CanStart(taskID string) (bool, error) {
	task, err := LoadTask(taskID)
	if err != nil {
		return false, err
	}
	for _, dpID := range task.BlockedBy {
		dep, err := LoadTask(dpID)
		if err != nil {
			return false, err
		}
		if dep.Status != StatusCompleted {
			return false, err
		}
	}
	return true, nil
}

func GetTasks(taskID string) (string, error) {
	task, err := LoadTask(taskID)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(task)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func blockedDependencies(task *Task) ([]string, error) {
	blocked := make([]string, 0)
	for _, dpID := range task.BlockedBy {
		dp, err := LoadTask(dpID)
		if err != nil {
			return nil, err
		}
		if dp.Status == StatusInProgress {
			blocked = append(blocked, dpID)
		}
	}
	return blocked, nil

}

// 领取任务
func ClaimTask(taskID string, owner string) (string, error) {
	task, err := LoadTask(taskID)
	if err != nil {
		return "", err
	}

	if task.Status != StatusPending {
		return fmt.Sprintf("Task %v is %v, cannot claim", taskID, task.Status), err
	}

	canStart, err := CanStart(taskID)
	if !canStart {
		deps, err := blockedDependencies(task)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Blocked by: %v", deps), nil
	}

	task.Owner = &owner
	task.Status = StatusInProgress

	err = SaveTask(task)
	if err != nil {
		return "", err
	}

	fmt.Printf("  \033[36m[claim] %s → in_progress (owner: %s)\033[0m\n", task.Subject, owner)

	return fmt.Sprintf("Claimed %s (%s)", task.ID, task.Subject), nil
}

func CompleteTask(taskID string) (string, error) {
	task, err := LoadTask(taskID)
	if err != nil {
		return "", err
	}

	if task.Status != StatusInProgress {
		return fmt.Sprintf("Task %s is %s, cannot complete", taskID, task.Status), nil
	}

	task.Status = StatusCompleted
	err = SaveTask(task)
	if err != nil {
		return "", err
	}

	tasks, err := ListTasks()
	if err != nil {
		return "", err
	}

	// 检查
	var unblocked []string
	for _, t := range tasks {
		if t.Status != StatusPending {
			continue
		}
		if len(t.BlockedBy) == 0 {
			continue
		}

		canStart, err := CanStart(t.ID)
		if err != nil {
			return "", err
		}
		if canStart {
			unblocked = append(unblocked, t.Subject)
		}
	}

	fmt.Printf("  \033[32m[complete] %s ✓\033[0m\n", task.Subject)

	msg := fmt.Sprintf("Completed %s (%s)", task.ID, task.Subject)

	if len(unblocked) > 0 {
		msg += fmt.Sprintf("\nUnblocked: %v", unblocked)

		fmt.Printf("  \033[33m[unblocked] %v\033[0m\n", unblocked)
	}

	return msg, nil

}
