package loop

import (
	"agent/background"
	"agent/compact"
	"agent/config"
	"agent/cron"
	"agent/hook"
	"agent/memory"
	"agent/prompt"
	"agent/recovery"
	"agent/session"
	"agent/subagent"
	"agent/tool"
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
)

func AgentLoop(query string, ctx context.Context, promptCtx *prompt.PromptContext, currentSession *session.Session, cronManager *cron.Manager) error {
	var round = 0
	handlers := make(map[string]tool.ToolHandler, 0)
	for name, handler := range tool.ToolHandlers {
		handlers[name] = handler
	}

	handlers["task"] = func(input map[string]any) (string, error) {
		return subagent.SpawnSubagent(input, ctx)
	}

	// messages := currentSession.Messages
	history := currentSession.Messages

	// 上一轮已经完成的后台任务
	if currentSession.Background != nil {
		notifications := currentSession.Background.CollectResults()
		for _, notification := range notifications {
			fmt.Printf("  \033[32m[background notification]\033[0m\n%s\n", notification)
			if currentSession.Logger != nil {
				currentSession.Logger.Printf("[background_result] %v", notification)
			}
			history = append(history, anthropic.NewUserMessage(
				anthropic.NewTextBlock(
					notification,
				),
			))
		}
	}

	triggerQuery := ""
	if query != "" {
		history = append(history, anthropic.NewUserMessage(
			anthropic.NewTextBlock(query),
		))
		triggerQuery = query
	}

	// cron
	fired := cronManager.ConsumeQueue()
	for _, job := range fired {
		scheduledText := "[Scheduled] " + job.Prompt
		fmt.Printf("  \033[35m[inject cron] %s\033[0m\n", job.Prompt)
		history = append(history, anthropic.NewUserMessage(
			anthropic.NewTextBlock(scheduledText),
		))
		triggerQuery = scheduledText
	}

	if triggerQuery == "" {
		return nil
	}

	// // 历史会话
	// history = append(history,
	// 	anthropic.NewUserMessage(
	// 		anthropic.NewTextBlock(query),
	// 	),
	// )

	defer func() {
		currentSession.Messages = history
	}()

	memoriesContent, err := memory.LoadMemories(history, ctx)
	if err != nil {
		fmt.Printf("[memory load error: %v]\n", err)
		memoriesContent = ""
	}

	rawsystem := prompt.GetSystemPrompt(*promptCtx)
	system := []anthropic.TextBlockParam{
		{
			Text: rawsystem,
		},
	}
	state := recovery.InitRecoveryState()
	maxTokens := config.DEFAULT_MAX_TOOKENS

	requestMessages, err := memory.CloneMessages(history)
	if err != nil {
		return err
	}
	// memoryTurn := memory.FindUserTurn(requestMessages, triggerQuery)

	// 注入记忆
	if memoriesContent != "" {
		requestMessages = memory.InjectMemories(requestMessages, len(requestMessages)-1, memoriesContent)
	}

	// loop
	for {
		round++
		if round > config.MAX_AGENT_ROUNDS {
			return fmt.Errorf(
				"maximum agent rounds exceeded: %d",
				config.MAX_AGENT_ROUNDS,
			)
		}
		// todo提醒
		if currentSession.RoundsSinceTodo >= 3 && len(requestMessages) > 0 {
			requestMessages = append(requestMessages, anthropic.NewUserMessage(
				anthropic.NewTextBlock("<reminder>Update your todos.</reminder>"),
			))
			currentSession.RoundsSinceTodo = 0
		}

		// compact context
		requestMessages = compact.ToolResultBudget(requestMessages, config.TOOL_RESULT_MAX_BYTES)
		requestMessages = compact.SnipCompact(requestMessages, config.MAX_MESSAGES)
		requestMessages = compact.MicroCompact(requestMessages)

		if compact.EstimateSize(requestMessages) > config.CONTEXT_LIMIT {
			fmt.Println("  \033[33m[auto compact]\033[0m")
			requestMessages, err = compact.CompactHistory(requestMessages, ctx)
			if err != nil {
				return err
			}
		}

		// memoryTurn := memory.FindUserTurn(requestMessages, query)

		// requestMessages := messages

		// // 注入记忆
		// if memoriesContent != "" && memoryTurn >= 0 && memoryTurn < len(requestMessages) {
		// 	requestMessages = memory.InjectMemories(requestMessages, memoryTurn, memoriesContent)
		// }

		// 工具
		tools := tool.BuildTools()

		// 调用LLM， retry
		message, err := recovery.WithRetry(
			func() (*anthropic.Message, error) {
				return config.Client.Messages.New(
					ctx,
					anthropic.MessageNewParams{
						MaxTokens: maxTokens,
						Messages:  requestMessages,
						Model:     state.CurrentModel,
						Tools:     tools,
						System:    system,
					},
				)
			},
			state,
			10,
		)

		// prompt 太长
		if err != nil {
			if recovery.IsPromptTooLongError(err) {
				if !state.HasAttemptedReactiveCompact {
					requestMessages, err = compact.ReactiveCompact(
						requestMessages,
						ctx,
					)
					if err != nil {
						return err
					}
					state.HasAttemptedReactiveCompact = true
					continue
				}

				fmt.Println("  \033[31m[unrecoverable] still too long after compact\033[0m")

				errorMessage := anthropic.NewAssistantMessage(
					anthropic.NewTextBlock(
						"[Error] Context too large, cannot continue.",
					),
				)

				history = append(history, errorMessage)
				return err
			}
			return err
		}

		state.HasAttemptedReactiveCompact = false

		fmt.Printf("  \033[90m[turn] stop_reason=%s input_tokens=%d output_tokens=%d max_tokens=%d\033[0m\n",
			message.StopReason, message.Usage.InputTokens, message.Usage.OutputTokens, maxTokens)

		if message.StopReason == anthropic.StopReasonMaxTokens {
			if !state.HasEscalated {
				prev := maxTokens
				maxTokens = config.ESCALATED_MAX_TOKENS
				state.HasEscalated = true
				fmt.Printf("  \033[33m[max_tokens] escalating max_tokens %d -> %d (stop_reason=max_tokens)\033[0m\n", prev, maxTokens)
				continue
			}

			assistantMessage := anthropic.NewAssistantMessage(message.ToParam().Content...)
			history, requestMessages, err = memory.AppendMessage(history, requestMessages, assistantMessage)
			if err != nil {
				return err
			}

			if state.RecoveryCount < config.MAX_RECOVERY_RETRIES {
				state.RecoveryCount++
				fmt.Printf("  \033[33m[max_tokens] truncated again, requesting continuation %d/%d (max_tokens=%d)\033[0m\n", state.RecoveryCount, config.MAX_RECOVERY_RETRIES, maxTokens)
				continuationMessage := anthropic.NewUserMessage(
					anthropic.NewTextBlock(
						"Output token limit hit. Resume directly — no apology, no recap. Pick up mid-thought.",
					),
				)
				requestMessages = append(requestMessages, continuationMessage)
				continue
			}
			return fmt.Errorf("still truncated after %d continuations", config.MAX_RECOVERY_RETRIES)

		}

		assistantMessage := anthropic.NewAssistantMessage(message.ToParam().Content...)
		history, requestMessages, err = memory.AppendMessage(history, requestMessages, assistantMessage)
		if err != nil {
			return err
		}

		// 没有tool_use，结束
		if message.StopReason != anthropic.StopReasonToolUse {
			_ = hook.TriggerHooks(hook.Stop, &hook.HookContext{
				Messages: history,
			})

			for _, block := range message.Content {
				text, ok := block.AsAny().(anthropic.TextBlock)
				if ok {
					fmt.Println(text.Text)
				}
			}

			// 提取memory
			err := memory.ExtractMemories(history, ctx)
			if err != nil {
				fmt.Printf("[memory extraction error: %v]\n", err)
			}

			return nil
		}

		// 工具执行
		currentSession.RoundsSinceTodo++
		toolResults := []anthropic.ContentBlockParamUnion{}
		for _, block := range message.Content {
			switch block := block.AsAny().(type) {
			case anthropic.ToolUseBlock:
				var input map[string]any
				err = json.Unmarshal([]byte(block.JSON.Input.Raw()), &input)
				if err != nil {
					toolResults = append(
						toolResults,
						anthropic.NewToolResultBlock(
							block.ID,
							fmt.Sprintf("invalid tool input: %v", err),
							true,
						),
					)
					continue
				}

				if currentSession.Logger != nil {
					currentSession.Logger.Printf(
						"[tool_call] tool=%v id=%v args=%v",
						block.Name,
						block.ID,
						input,
					)
				}

				blocked := hook.TriggerHooks(hook.PreToolUse, &hook.HookContext{
					Block: &block,
					Args:  input,
				})
				if len(blocked) > 0 {
					toolResults = append(toolResults, anthropic.NewToolResultBlock(block.ID, blocked, true))
					continue
				}

				// handler
				handler, ok := handlers[block.Name]
				if !ok {
					toolResults = append(toolResults,
						anthropic.NewToolResultBlock(
							block.ID,
							fmt.Sprintf("unknown tool: %s", block.Name),
							true,
						),
					)
					continue
				}

				// 后台
				if currentSession.Background != nil {
					shouldBg := background.ShouldRunBackground(block.Name, input)
					fmt.Printf("[DEBUG background] tool=%s input=%v should=%v\n",
						block.Name,
						input,
						shouldBg)

					if shouldBg {
						bgBlock := block
						bgID := currentSession.Background.Start(
							block.ID,
							block.Name,
							input,
							func(args map[string]any) (string, error) {
								output, err := handler(args)
								if err != nil {
									return "", err
								}
								hook.TriggerHooks(
									hook.PostToolUse,
									&hook.HookContext{
										Block:  &bgBlock,
										Output: output,
										Args:   args,
									},
								)
								return output, nil
							},
						)

						fmt.Printf(
							"  \033[33m[background] %s started\033[0m\n",
							bgID,
						)

						if currentSession.Logger != nil {
							currentSession.Logger.Printf(
								"[background_start] id=%s tool=%s tool_use_id=%s",
								bgID,
								block.Name,
								block.ID,
							)
						}

						toolResults = append(
							toolResults,
							anthropic.NewToolResultBlock(
								block.ID,
								fmt.Sprintf(
									"[Background task %s started] "+
										"Result will be available when complete.",
									bgID,
								),
								false,
							),
						)

						continue

					}
				}

				// 前台
				output, err := handler(input)
				if err != nil {
					if currentSession.Logger != nil {
						currentSession.Logger.Printf(
							"[tool_error] tool=%v error=%v",
							block.Name,
							err,
						)
					}
					toolResults = append(
						toolResults,
						anthropic.NewToolResultBlock(
							block.ID,
							err.Error(),
							true,
						),
					)

					continue
				}

				// hook
				hook.TriggerHooks(hook.PostToolUse, &hook.HookContext{
					Block:  &block,
					Output: output,
					Args:   input,
				})

				// 日志
				preview := output
				if len(preview) > 200 {
					fmt.Println(preview[:200])
				} else {
					fmt.Println(preview)
				}
				if currentSession.Logger != nil {
					currentSession.Logger.Printf(
						"[tool_result] tool=%v output=%v",
						block.Name,
						preview,
					)
				}

				fmt.Printf(
					"[tool_result] tool=%v output=%v",
					block.Name,
					preview,
				)
				toolResults = append(toolResults, anthropic.NewToolResultBlock(block.ID, output, false))

			}
		}

		// 检查后台
		if currentSession.Background != nil {
			notifications := currentSession.Background.CollectResults()
			for _, notification := range notifications {
				fmt.Printf("  \033[32m[background notification]\033[0m\n%s\n", notification)
				if currentSession.Logger != nil {
					currentSession.Logger.Printf("[background_result] %v", notification)
				}
				toolResults = append(toolResults, anthropic.NewTextBlock(notification))
			}
		}

		if len(toolResults) == 0 {
			break
		}

		toolResultMessage := anthropic.NewUserMessage(toolResults...)
		history, requestMessages, err = memory.AppendMessage(history, requestMessages, toolResultMessage)
		if err != nil {
			return err
		}

		promptCtx, err = prompt.UpdateContext(*promptCtx, history)
		if err != nil {
			return err
		}

		rawsystem = prompt.GetSystemPrompt(*promptCtx)
		system = []anthropic.TextBlockParam{
			{
				Text: rawsystem,
			},
		}

	}
	return nil
}
