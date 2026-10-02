package cron

func (m *Manager) ConsumeQueue() []*CronJob {
	m.mu.Lock()
	defer m.mu.Unlock()

	fired := make([]*CronJob, len(m.queue))

	copy(fired, m.queue)

	m.queue = m.queue[:0]

	return fired
}

func (m *Manager) HasCronQueue() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	return len(m.queue) > 0
}
