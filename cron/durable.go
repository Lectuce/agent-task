package cron

import (
	"encoding/json"
	"fmt"
	"os"
)

func (m *Manager) saveDurableJobsLocked() error {
	var durable []*CronJob

	for _, job := range m.jobs {
		if job.Durable {
			durable = append(durable, job)
		}
	}

	data, err := json.Marshal(durable)
	if err != nil {
		return err
	}

	return os.WriteFile(m.durablePath, data, 0644)
}

func (m *Manager) LoadDurableJobs() error {
	data, err := os.ReadFile(
		m.durablePath,
	)

	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return err
	}

	var jobs []*CronJob

	err = json.Unmarshal(data, &jobs)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	valid := 0

	for _, job := range jobs {
		if err := ValidateCron(job.Cron); err != nil {
			fmt.Printf("[cron] skipping invalid job %s: %v\n", job.ID, err)

			continue
		}

		m.jobs[job.ID] = job
		valid++
	}

	if valid > 0 {
		fmt.Printf("[cron] loaded %d durable job(s)\n", valid)
	}

	return nil
}
