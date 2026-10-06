package tool

import (
	"fmt"
	"strings"
)

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
