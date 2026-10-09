package worktree

import (
	"agent/config"
	"agent/task"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var VALID_WT_NAME = regexp.MustCompile("^[A-Za-z0-9._-]{1,64}$")

type event struct {
	EventType    string    `json:"event_type"`
	WorkTreeName string    `json:"work_tree_name"`
	TaskID       string    `json:"task_id"`
	Ts           time.Time `json:"ts"`
}

const (
	Create = "create"
	Remove = "remove"
	Keep   = "keep"
)

func init() {
	_, err := initWorktreesDir()
	if err != nil {
		fmt.Println(err.Error())
	}
}

func initWorktreesDir() (string, error) {
	err := os.MkdirAll(config.WORKTREES_DIR, 0755)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("init worktressdir: %v", config.WORKTREES_DIR), nil
}

func validateWorktreeName(name string) (string, error) {

	if name == "" {
		return "", fmt.Errorf("worktree dir name is empty")
	}

	if name == "." || name == ".." {
		return "", fmt.Errorf("worktree dir name is . or ..")
	}

	match := VALID_WT_NAME.MatchString(name)

	if match == false {

		return "", fmt.Errorf("name: %v is illegal", name)
	}

	return fmt.Sprintf("name: %v is legal.", name), nil

}

func runGit(args []string) (string, error) {

	ctx, cancel := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = config.WORKDIR

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}

	result := strings.TrimSpace(string(out))

	if result == "" {
		return "no git output", nil
	}

	output := []rune(result)
	if len(output) > 5000 {
		return string(output[:5000]), nil
	}

	if ctx.Err() == context.DeadlineExceeded {
		return result, fmt.Errorf("Timeout (30s)")
	}

	return result, nil
}

func logEvent(eventType string, workTreeName string, taskID string) error {

	event := event{
		EventType:    eventType,
		WorkTreeName: workTreeName,
		TaskID:       taskID,
		Ts:           time.Now(),
	}
	eventsFile := filepath.Join(config.WORKTREES_DIR, "events.jsonl")
	file, err := os.OpenFile(eventsFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	err = json.NewEncoder(file).Encode(event)
	if err != nil {
		return fmt.Errorf("write event: %w", err)
	}

	return nil
}

func CreateWorktree(name string, taskID string) (string, error) {
	_, err := validateWorktreeName(name)
	if err != nil {
		return "valididate failed", err
	}
	path := filepath.Join(config.WORKTREES_DIR, name)

	// 检查路径是否存在
	_, err = os.Stat(path)
	if err == nil {
		return "", fmt.Errorf(
			"worktree %q already exists at %s",
			name,
			path,
		)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("check worktree path: %w", err)
	}

	result, err := runGit([]string{"worktree", "add", path, "-b", fmt.Sprintf("wt/%v", name), "HEAD"})
	if err != nil {
		return "", fmt.Errorf("git error: %v", result)
	}
	if taskID != "" {
		err = bindTaskToWorktree(taskID, name)
		if err != nil {
			return "", fmt.Errorf("bind task to worktree failed: %v", err)
		}
	}
	err = logEvent("create", name, taskID)
	if err != nil {
		return "", fmt.Errorf("log event failed: %v", err)
	}

	fmt.Printf("  \033[33m[worktree] created: %v at %v\033[0m", name, path)

	return fmt.Sprintf("Worktree '%v' created at %v", name, path), nil

}

func bindTaskToWorktree(taskID string, worktreeName string) error {
	t, err := task.LoadTask(taskID)
	if err != nil {
		return err
	}
	t.Worktree = worktreeName
	err = task.SaveTask(t)
	if err != nil {
		return err
	}
	fmt.Printf("  \033[33m[bind] %v → worktree:%v\033[0m", t.Subject, t.Worktree)
	return nil
}

// 计数未提交的文件并在一个worktree中提交

func countWorktreeChanges(path string) (int, int) {

	fileCount := 0
	fileCtx, fileCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer fileCancel()

	fileCmd := exec.CommandContext(fileCtx, "git", "status", "--porcelain")
	fileCmd.Dir = path

	fileOut, fileErr := fileCmd.CombinedOutput()

	if fileCtx.Err() == context.DeadlineExceeded {
		fmt.Println(fileCtx.Err())
		return -1, -1
	}

	if fileErr != nil {
		fmt.Println(fileErr.Error())
		return -1, -1
	}

	fileRows := string(fileOut)
	fileRows = strings.TrimSpace(fileRows)
	fileLines := strings.Split(fileRows, "\n")
	for _, line := range fileLines {
		if line == "" {
			continue
		}
		fileCount++
	}

	commitCtx, commitCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer commitCancel()

	commitCmd := exec.CommandContext(fileCtx, "git", "@{push}..HEAD", "--online")
	commitCmd.Dir = path

	commitOut, commitErr := commitCmd.CombinedOutput()

	if commitCtx.Err() == context.DeadlineExceeded {
		fmt.Println(commitCtx.Err())
		return -1, -1
	}

	if commitErr != nil {
		fmt.Println(commitErr.Error())
		return -1, -1
	}

	commitCount := 0

	commitRows := string(commitOut)
	commitRows = strings.TrimSpace(commitRows)
	commitLines := strings.Split(commitRows, "\n")
	for _, line := range commitLines {
		if line == "" {
			continue
		}
		commitCount++
	}

	return fileCount, commitCount
}

func RemoveWorktree(name string, discardChanges bool) (string, error) {

	_, err := validateWorktreeName(name)
	if err != nil {
		return "", fmt.Errorf("name is illegal: %v", err)
	}

	path := filepath.Join(config.WORKTREES_DIR, name)

	_, err = os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("worktree is not found: %v", err)
		}
		return "", err
	}

	if discardChanges == false {
		fileCount, commitCount := countWorktreeChanges(path)
		if fileCount == -1 || commitCount == -1 {
			return "", fmt.Errorf("Cannot verify worktree '%v' status. "+
				"Use discard_changes=true to force removal.", name)
		}

		if fileCount > 0 || commitCount > 0 {
			return "", fmt.Errorf("Worktree '%s' has %d uncommitted file(s) "+
				"and %d unpushed commit(s). "+
				"Use discard_changes=true to force removal, "+
				"or keep_worktree to preserve for review.", name, fileCount, commitCount)
		}

	}

	removeArgs := []string{"worktree", "remove"}

	removeArgs = append(removeArgs, path)

	if discardChanges {
		removeArgs = append(removeArgs, "--force")
	}
	result, err := runGit(removeArgs)

	if err != nil {
		return "", fmt.Errorf("Failed to remove worktree directory for '%s', output: %s", name, result)
	}

	branchResult, err := runGit([]string{"branch", "-D", fmt.Sprintf("wt/%s", name)})
	if err != nil {
		return "", fmt.Errorf("Failed to remove worktree branch for %s: %v, output: %s", name, err, branchResult)
	}

	err = logEvent(Remove, name, "")
	if err != nil {
		return "", fmt.Errorf("log event failed for %s: %v", name, err)
	}

	fmt.Printf("  \033[36m[worktree] removed: %s\033[0m", name)

	return fmt.Sprintf("Worktree '%s' removed", name), nil
}

func KeepWorktree(name string) (string, error) {

	_, err := validateWorktreeName(name)
	if err != nil {
		return "", err
	}

	err = logEvent(Keep, name, "")
	if err != nil {
		return "", err
	}
	print(fmt.Sprintf("  \033[36m[worktree] kept: %s\033[0m", name))
	return fmt.Sprintf("Worktree '%s' kept for review (branch: wt/%s)", name, name), nil
}
