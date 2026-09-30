package loop

import (
	"agent/background"
	"agent/compact"
	"agent/config"
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

func AgentLoop(query string, ctx context.Context, promptCtx *prompt.PromptContext, currentSession *session.Session) error {
	var round = 0
	handlers := make(map[string]tool.ToolHandler, 0)
	for name, handler := range tool.ToolHandlers {
		handlers[name] = handler
	}

	handlers["task"] = func(input map[string]any) (string, error) {
		return subagent.SpawnSubagent(input, ctx)
	}

	// 会话
	currentSession.Messages = append(currentSession.Messages,
		anthropic.NewUserMessage(
			anthropic.NewTextBlock(query),
		),
	)
	messages := currentSession.Messages
	defer func() {
		currentSession.Messages = messages
	}()

	memoriesContent, err := memory.LoadMemories(messages, ctx)
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
		if currentSession.RoundsSinceTodo >= 3 && len(messages) > 0 {
			messages = append(messages, anthropic.NewUserMessage(
				anthropic.NewTextBlock("<reminder>Update your todos.</reminder>"),
			))
			currentSession.RoundsSinceTodo = 0
		}

		// 压缩前快照
		preCompress, err := memory.CloneMessages(messages)
		if err != nil {
			return err
		}

		// compact context
		messages = compact.ToolResultBudget(messages, config.TOOL_RESULT_MAX_BYTES)
		messages = compact.SnipCompact(messages, config.MAX_MESSAGES)
		messages = compact.MicroCompact(messages)

		if compact.EstimateSize(messages) > config.CONTEXT_LIMIT {
			fmt.Println("  \033[33m[auto compact]\033[0m")
			messages, err = compact.CompactHistory(messages, ctx)
			if err != nil {
				return err
			}
		}

		memoryTurn := memory.FindUserTurn(messages, query)

		requestMessages := messages

		// 注入记忆
		if memoriesContent != "" && memoryTurn >= 0 && memoryTurn < len(messages) {
			requestMessages = memory.InjectMemories(messages, memoryTurn, memoriesContent)
		}

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

		// prompt太长
		if err != nil {
			if recovery.IsPromptTooLongError(err) {
				if !state.HasAttemptedReactiveCompact {
					messages, err = compact.ReactiveCompact(messages, ctx)
					if err != nil {
						return err
					}
					state.HasAttemptedReactiveCompact = true
					continue
				}
				fmt.Println("  \033[31m[unrecoverable] still too long after compact\033[0m")
				messages = append(messages, anthropic.NewAssistantMessage(
					anthropic.NewTextBlock("[Error] Context too large, cannot continue."),
				))
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
			messages = append(messages, anthropic.NewAssistantMessage(message.ToParam().Content...))

			if state.RecoveryCount < config.MAX_RECOVERY_RETRIES {
				state.RecoveryCount++
				fmt.Printf("  \033[33m[max_tokens] truncated again, requesting continuation %d/%d (max_tokens=%d)\033[0m\n", state.RecoveryCount, config.MAX_RECOVERY_RETRIES, maxTokens)
				messages = append(messages, anthropic.NewUserMessage(
					anthropic.NewTextBlock("Output token limit hit. Resume directly — no apology, no recap. Pick up mid-thought."),
				))
				continue
			}
			return fmt.Errorf("still truncated after %d continuations", config.MAX_RECOVERY_RETRIES)

		}

		messages = append(messages, anthropic.NewAssistantMessage(message.ToParam().Content...))

		// 没有tool_use，结束
		if message.StopReason != anthropic.StopReasonToolUse {
			_ = hook.TriggerHooks(hook.Stop, &hook.HookContext{
				Messages: messages,
			})

			for _, block := range message.Content {
				if text, ok := block.AsAny().(anthropic.TextBlock); ok {
					fmt.Println(text.Text)
				}
			}

			// 压缩快照提取memory
			err := memory.ExtractMemories(preCompress, ctx)
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
			case anthropic.ServerToolUseBlock:

				if background.IsSlowOperation(string(block.Name), block) {

				}

				currentSession.Logger.Printf("[server_tool_call] tool=%v id=%v", block.Name, block.ID)
				fmt.Printf("[server_tool_call] tool=%v id=%v\n", block.Name, block.ID)

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

				currentSession.Logger.Printf(
					"[tool_call] tool=%v id=%v args=%v",
					block.Name,
					block.ID,
					input,
				)

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
				output, err := handler(input)
				if err != nil {
					currentSession.Logger.Printf(
						"[tool_error] tool=%v error=%v",
						block.Name,
						err,
					)
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
				currentSession.Logger.Printf(
					"[tool_result] tool=%v output=%v",
					block.Name,
					preview,
				)
				fmt.Printf(
					"[tool_result] tool=%v output=%v",
					block.Name,
					preview,
				)
				toolResults = append(toolResults, anthropic.NewToolResultBlock(block.ID, output, false))

			}
		}
		if len(toolResults) == 0 {
			break
		}
		messages = append(messages, anthropic.NewUserMessage(toolResults...))
		promptCtx, err = prompt.UpdateContext(*promptCtx, messages)
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
