package tool

import "agent/worktree"

func runCreateWorktree(input map[string]any) (string, error) {

	name, err := requireString(input, "name", "name is required.")
	if err != nil {
		return "", err
	}

	taskID, err := requireString(input, "task_id", "task_id is required.")
	if err != nil {
		return "", err
	}

	return worktree.CreateWorktree(name, taskID)
}

func runRemoveWorktree(input map[string]any) (string, error) {

	name, err := requireString(input, "name", "name is required.")
	if err != nil {
		return "", err
	}

	discardChanges := false
	v, err := requireBool(input, "discard_changes", "discard_changes is required.")
	if err != nil {
		return "", err
	}
	discardChanges = v

	return worktree.RemoveWorktree(name, discardChanges)
}

func runKeepWroktree(input map[string]any) (string, error) {

	name, err := requireString(input, "name", "name is required.")
	if err != nil {
		return "", err
	}

	return worktree.KeepWorktree(name)

}
