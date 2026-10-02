package cron

import (
	"fmt"
	"math/rand"
)

func (m *Manager) ScheduleJob(cronExpr string, prompt string, recurring bool, durable bool) (*CronJob, error) {

	if err := ValidateCron(cronExpr); err != nil {
		return nil, err
	}

	job := &CronJob{
		ID:        fmt.Sprintf("cron_%06d", rand.Intn(1000000)),
		Cron:      cronExpr,
		Prompt:    prompt,
		Recurring: recurring,
		Durable:   durable,
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.jobs[job.ID] = job

	if durable {
		err := m.saveDurableJobsLocked()
		if err != nil {
			return nil, err
		}
	}

	fmt.Printf("[cron register] %s '%s' -> %.40s\n", job.ID, cronExpr, prompt)

	return job, nil
}

func (m *Manager) CancelJob(jobID string) (string, error) {

	m.mu.Lock()
	defer m.mu.Unlock()

	job, ok := m.jobs[jobID]
	if !ok {
		return fmt.Sprintf("Job %s not found", jobID), nil
	}

	delete(m.jobs, jobID)
	delete(m.lastFired, jobID)

	if job.Durable {
		if err := m.saveDurableJobsLocked(); err != nil {
			return "", err
		}
	}

	fmt.Printf("[cron cancel] %s\n", jobID)

	return fmt.Sprintf("Cancelled %s", jobID), nil
}

func (m *Manager) ListJobs() []*CronJob {
	m.mu.Lock()
	defer m.mu.Unlock()

	jobs := make([]*CronJob, 0, len(m.jobs))

	for _, job := range m.jobs {
		jobCopy := *job
		jobs = append(jobs, &jobCopy)
	}

	return jobs
}
