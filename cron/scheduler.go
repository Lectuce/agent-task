package cron

import (
	"fmt"
	"time"
)

func (m *Manager) SchedulerLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for now := range ticker.C {

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
}
