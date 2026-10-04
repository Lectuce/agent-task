package cron

import (
	"fmt"
	"math/rand"
	"time"
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

	job, registered := m.jobs[jobID]

	queued := false
	remaining := make([]*CronJob, 0, len(m.queue))
	for _, queueJob := range m.queue {
		if queueJob.ID == jobID {
			queued = true
			continue
		}
		remaining = append(remaining, queueJob)
	}

	m.queue = remaining

	if !registered && !queued {
		return fmt.Sprintf("Job %s not found", jobID), nil
	}
	if registered {
		delete(m.jobs, jobID)
	}

	delete(m.lastFired, jobID)

	if registered && job.Durable {
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

func (m *Manager) processJobs(now time.Time) {
	m.mu.Lock()

	minuteMarker := now.Format("2006-01-02 15:04")

	for id, job := range m.jobs {

		func() {
			defer func() {
				r := recover()
				if r != nil {
					fmt.Printf("[cron error] %s: %v\n", id, r)
				}
			}()

			if !CronMatches(job.Cron, now) {
				return
			}

			if m.lastFired[job.ID] == minuteMarker {
				return
			}

			jobCopy := *job

			m.queue = append(m.queue, &jobCopy)

			m.lastFired[job.ID] = minuteMarker

			fmt.Printf("[cron fire] %s -> %.40s\n", job.ID, job.Prompt)

			if !job.Recurring {
				delete(m.jobs, job.ID)

				if job.Durable {
					if err :=
						m.saveDurableJobsLocked(); err != nil {
						fmt.Printf("[cron save error] %v\n", err)
					}
				}
			}
		}()
	}
	m.mu.Unlock()
}
