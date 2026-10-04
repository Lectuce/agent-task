package cron

import (
	"context"
	"time"
)

func (m *Manager) SchedulerLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			m.processJobs(now)
		}
	}

}
