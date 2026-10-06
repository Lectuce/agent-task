package tool

import (
	"agent/config"
	"agent/protocol"
	"agent/skills"
	"agent/task"
	"agent/teams"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knetic/govaluate"
)

func runBash(input map[string]any) (string, error) {
	// command string
	command, ok := input["command"].(string)
	if !ok || command == "" {
		return "", fmt.Errorf("command is required")
	}
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

func runEdit(input map[string]any) (string, error) {
	// path string, old_text string, new_text string
	path, err := safePath(input)
	if err != nil {
		return "", err
	}
	oldText := input["old_text"].(string)
	newText := input["new_text"].(string)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	text := string(data)
	if !strings.Contains(text, oldText) {
		return "", fmt.Errorf("text not found")
	}
	text = strings.Replace(text, oldText, newText, 1)
	err = os.WriteFile(path, []byte(text), 0644)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Edited %v\n", path), nil
}

func runGlob(input map[string]any) (string, error) {
	// pattern string
	pattern := input["pattern"].(string)
	path := filepath.Join(config.WORKDIR, pattern)
	matches, err := filepath.Glob(path)
	if err != nil {
		return "", err
	}
	for i, m := range matches {
		rel, err := filepath.Rel(config.WORKDIR, m)
		if err == nil {
			matches[i] = rel
		}
	}

	return strings.Join(matches, "\n"), nil
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

func runloadSkill(input map[string]any) (string, error) {
	name, ok := input["name"].(string)
	if !ok || name == "" {
		return "", fmt.Errorf("name is required")
	}
	skill, ok := skills.SkillRegistry[name]
	if !ok {
		return "", fmt.Errorf("Skill not found: %v\n", skill)
	}
	return skill.Content, nil

}

func runCalculator(input map[string]any) (string, error) {
	expression, ok := input["expression"].(string)
	if !ok || expression == "" {
		return "", fmt.Errorf("expression is required")
	}

	expr, err := govaluate.NewEvaluableExpression(expression)
	if err != nil {
		return "", fmt.Errorf("invalid expression: %w", err)
	}

	result, err := expr.Evaluate(nil)
	if err != nil {
		return "", fmt.Errorf("calculate expression: %w", err)
	}

	return fmt.Sprintf("%v", result), nil
}

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

func runScheduleCron(input map[string]any) (string, error) {
	cronExpr, ok := input["cron_expr"].(string)
	if !ok || cronExpr == "" {
		return "", fmt.Errorf("cron_expr is required")
	}
	prompt, ok := input["prompt"].(string)
	if !ok || prompt == "" {
		return "", fmt.Errorf("prompt is required")
	}
	recurring := true
	v, ok := input["recurring"].(bool)
	if ok {
		recurring = v
	}
	durable := false
	u, ok := input["durable"].(bool)
	if ok {
		durable = u
	}
	cronJob, err := CronManager.ScheduleJob(cronExpr, prompt, recurring, durable)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Scheduled %s: '%s' -> %s", cronJob.ID, cronJob.Cron, cronJob.Prompt), nil
}

func runListCrons(input map[string]any) (string, error) {
	cronJobs := CronManager.ListJobs()
	if len(cronJobs) == 0 {
		return "", fmt.Errorf("No cron jobs. Use schedule_cron to add one")
	}

	lines := make([]string, 0)

	for _, job := range cronJobs {
		tag := "one-shot"
		if job.Recurring {
			tag = "recurring"
		}

		dur := "session"
		if job.Durable {
			dur = "durable"
		}

		preview := job.Prompt
		if len(preview) > 40 {
			preview = preview[:40]
		}

		lines = append(lines, fmt.Sprintf("  %v: '%v' → %v [%v, %v]", job.ID, job.Cron, preview, tag, dur))

	}

	result := strings.Join(lines, "\n")
	return result, nil
}

func runCancleCron(input map[string]any) (string, error) {
	jobID, ok := input["job_id"].(string)
	if !ok || jobID == "" {
		return "", fmt.Errorf("job_id is required")
	}
	return CronManager.CancelJob(jobID)

}

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
	msgs, err := MessageBus.ReadInbox("lead")
	if err != nil {
		return "", err
	}
	if len(msgs) == 0 {
		return "", fmt.Errorf("(inbox empty)")
	}
	lines := make([]string, 0)

	for _, m := range msgs {
		preview := m.Content
		if len(preview) > 200 {
			preview = preview[:200]
		}
		lines = append(lines, fmt.Sprintf("  [%v] %v", m.From, preview))
	}
	result := strings.Join(lines, "\n")
	return result, nil

}

func runRequestShudown(input map[string]any) (string, error) {
	teammate, ok := input["teammate"].(string)
	if !ok || teammate == "" {
		return "", fmt.Errorf("teammate is required")
	}

	requestID := protocol.NewRequestID()

	protocol.AddPendingRequests(&protocol.ProtocolState{
		RequestID:      requestID,
		ProtocolType:   protocol.ShutDown,
		Sender:         "lead",
		Target:         teammate,
		ProtocolStatus: protocol.Pending,
		Payload:        "",
	})

	err := MessageBus.Send("lead", teammate, "Please shut down gracefully.", "shutdown_request", map[string]any{"request_id": requestID})
	if err != nil {
		return "", fmt.Errorf("send error: %v", err)
	}

	fmt.Printf("  \033[35m[protocol] shutdown_request → %v "+
		"(%v)\033[0m", teammate, requestID)

	return fmt.Sprintf("Shutdown request sent to %v (req: %v)\n", teammate, requestID), nil
}

func runRequestPlan(input map[string]any) (string, error) {

	teammate, ok := input["teammate"].(string)
	if !ok || teammate == "" {
		return "", fmt.Errorf("teammate is required.")
	}

	task, ok := input["task"].(string)
	if !ok || task == "" {
		return "", fmt.Errorf("task is required.")
	}

	err := MessageBus.Send("lead",
		teammate,
		fmt.Sprintf("Please submit a plan for: %v", task),
		"message",
		map[string]any{},
	)
	if err != nil {
		return "", fmt.Errorf("send error: %v", err)
	}

	return fmt.Sprintf("Asked %v to submit a plan", teammate), nil

}

func runReviewPlan(input map[string]any) (string, error) {

	requestID, ok := input["request_id"].(string)
	if !ok || requestID == "" {
		return "", fmt.Errorf("request_id is required.")
	}
	approve := false
	approve, ok = input["approve"].(bool)
	if !ok {
		return "", fmt.Errorf("approve is required.")
	}

	feedback := ""
	v, ok := input["feedback"].(string)
	if ok {
		feedback = v
	}

	state, ok := protocol.GetPendingRequest(requestID)
	if !ok || state == nil {
		return "", fmt.Errorf("request %v not found", requestID)
	}
	state.Mu.Lock()
	defer state.Mu.Unlock()

	if state.ProtocolStatus != protocol.Pending {
		status := state.ProtocolStatus
		return "", fmt.Errorf("Request %v already %v", requestID, status)
	}

	if approve {
		state.ProtocolStatus = protocol.Approved
	} else {
		state.ProtocolStatus = protocol.Rejected
	}

	content := feedback
	if content == "" {
		if approve {
			content = protocol.Approved
		} else {
			content = protocol.Rejected
		}
	}

	sender := state.Sender
	err := MessageBus.Send(
		"lead",
		sender,
		content,
		protocol.PlanApprovalResponse,
		map[string]any{
			"request_id": requestID,
			"approve":    approve,
		},
	)
	if err != nil {
		return "", fmt.Errorf("send error: %v", err)
	}

	icon := "✗"
	if approve {
		icon = "✓"
	}

	status := protocol.Rejected
	if approve {
		status = protocol.Approved
	}

	fmt.Printf("  \033[32m[protocol] plan %v (%v)\033[0m\n", icon, requestID)
	return fmt.Sprintf("Plan %v (%v)", status, requestID), nil

}
