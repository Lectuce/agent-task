package teams

import (
	"agent/config"
	"agent/protocol"
	"agent/task"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

func scanUnclaimedTasks() ([]task.Task, error) {

	unclaim := make([]task.Task, 0)

	pattern := filepath.Join(config.TASKS_DIR, "task_*.json")

	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}

		var t task.Task
		err = json.Unmarshal(data, &t)
		if err != nil {
			return nil, err
		}

		owner := t.Owner
		canStart, err := task.CanStart(t.ID)
		if err != nil {
			return nil, err
		}
		if t.Status == task.StatusPending && owner == nil && canStart {
			unclaim = append(unclaim, t)
		}
	}
	return unclaim, nil
}

func idlePoll(agentName string, messages *[]anthropic.MessageParam, name string, role string) (string, error) {

	for range config.IDLE_TIMEOUT / config.IDLE_POLL_INTERVAL {

		time.Sleep(config.IDLE_POLL_INTERVAL)

		inbox, err := BUS.ReadInbox(agentName)
		if err != nil {
			return "", err
		}

		if len(inbox) > 0 {
			for _, msg := range inbox {

				if msg.MessageType == protocol.ShutDownRequest {

					metadata := map[string]any{}
					metadata = msg.Metadata
					requestID := ""
					if metadata != nil {
						v, ok := msg.Metadata["request_id"].(string)
						if ok {
							requestID = v
						}
					}

					err = BUS.Send(name, "lead", "Shtting down gracefully.",
						protocol.ShutDownResponse,
						map[string]any{
							"request_id": requestID,
							"approve":    true,
						},
					)

					fmt.Printf("  \033[35m[protocol] %v approved shutdown "+
						"in idle (%v)\033[0m\n", name, requestID,
					)
					return "shutdown", nil
				}

				data, err := json.Marshal(inbox)
				if err != nil {
					return "", err
				}
				*messages = append(*messages, anthropic.NewUserMessage(
					anthropic.NewTextBlock("<inbox>"+
						string(data)+"</inbox>",
					),
				))
				fmt.Printf("  \033[36m[idle] %v found inbox messages\033[0m\n", name)
				return "work", nil
			}
		}

		unclaim, err := scanUnclaimedTasks()
		if err != nil {
			return "", err
		}
		if len(unclaim) > 0 {

			t := unclaim[0]
			result, err := task.TryClaimTask(t.ID, agentName)
			if err != nil {
				return "", err
			}
			if result.Claimed {
				var worktreeInfo string
				if t.Worktree != "" {
					worktreeDir := filepath.Join(config.WORKTREES_DIR, t.Worktree)
					worktreeInfo = fmt.Sprintf("Work directory: %s", worktreeDir)
				}

				*messages = append(*messages, anthropic.NewUserMessage(
					anthropic.NewTextBlock(
						fmt.Sprintf("<auto-claimed>Task %v: "+
							"%v%v</auto-claimed>", t.ID, t.Subject, worktreeInfo,
						),
					),
				))

				fmt.Printf("  \033[32m[idle] %v auto-claimed: "+
					"%v\033[0m\n", name, t.Subject,
				)

				return "work", nil
			}

			fmt.Printf("  \033[33m[idle] %v claim failed: "+
				"%v\033[0m", name, result)
		}
	}
	fmt.Printf("  \033[31m[idle] %v timeout (%v)\033[0m", name, config.IDLE_TIMEOUT)
	return "timeout", nil
}
