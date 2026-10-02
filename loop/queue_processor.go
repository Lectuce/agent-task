package loop

import (
	"agent/cron"
	"agent/prompt"
	"agent/session"
	"context"
	"fmt"
	"sync"
	"time"
)

var AgentLock sync.Mutex

func QueueProcessorLoop(cronManager *cron.Manager, ctx context.Context, promptCtx *prompt.PromptContext, getCurrentSession func() *session.Session) {
	ticker := time.NewTicker(200 * time.Millisecond)

	defer ticker.Stop()

	for range ticker.C {

		if !cronManager.HasCronQueue() {
			continue
		}

		if !AgentLock.TryLock() {
			continue
		}

		func() {
			defer AgentLock.Unlock()

			if !cronManager.HasCronQueue() {
				return
			}

			fmt.Println("\n\033[35m" +
				"[queue processor] delivering scheduled work" +
				"\033[0m",
			)

			currentSession := getCurrentSession()

			err := AgentLoop(
				"",
				ctx,
				promptCtx,
				currentSession,
				cronManager,
			)

			if err != nil {
				fmt.Printf(
					"[cron agent error] %v\n",
					err,
				)
			}
		}()
	}
}
