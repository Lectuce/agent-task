package teams

import (
	"agent/config"
	"agent/protocol"
	"agent/recovery"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

var (
	activeMu       sync.Mutex
	activeTeammate = map[string]bool{}
)

func SpawnTeammateThread(name string, role string, prompt string) (string, error) {

	activeMu.Lock()
	_, ok := activeTeammate[name]
	if ok {
		return "", fmt.Errorf("Teammate %v already exists.", name)
	}
	activeTeammate[name] = true
	activeMu.Unlock()

	system := []anthropic.TextBlockParam{
		{
			Text: fmt.Sprintf(
				"You are %v, a %v. \n"+
					"Use tools to complete tasks.\n"+
					"Send results via send_message to 'lead'. ",
				name, role,
			),
		},
	}

	go func() {
		ctx := context.Background()
		messages := []anthropic.MessageParam{
			anthropic.NewUserMessage(
				anthropic.NewTextBlock(
					prompt,
				),
			),
		}
		subTools := []anthropic.ToolParam{
			{
				Name:        "bash",
				Description: anthropic.String("Run a shell command."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Type: "object",
					Properties: map[string]interface{}{
						"command": map[string]interface{}{
							"type": "string",
						},
						"bash": map[string]interface{}{
							"type": "boolean",
						},
					},
					Required: []string{"command"},
				},
			},
			{
				Name:        "read_file",
				Description: anthropic.String("Read file contents."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Type: "object",
					Properties: map[string]interface{}{
						"path": map[string]interface{}{
							"type": "string",
						},
						"limit": map[string]interface{}{
							"type": "integer",
						},
					},
					Required: []string{"path"},
				},
			},
			{
				Name:        "write_file",
				Description: anthropic.String("Write content to a file."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Type: "object",
					Properties: map[string]interface{}{
						"path": map[string]interface{}{
							"type": "string",
						},
						"content": map[string]interface{}{
							"type": "string",
						},
					},
					Required: []string{"path", "content"},
				},
			},
			{
				Name:        "send_message",
				Description: anthropic.String("Send a message to another agent."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Type: "object",
					Properties: map[string]interface{}{
						"to": map[string]interface{}{
							"type": "string",
						},
						"content": map[string]interface{}{
							"type": "string",
						},
					},
					Required: []string{"to", "content"},
				},
			},
			{
				Name:        "submit_plan",
				Description: anthropic.String("Submit a plan for Lead approval."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Type: "object",
					Properties: map[string]interface{}{
						"plan": map[string]interface{}{
							"type": "string",
						},
						"content": map[string]interface{}{
							"type": "string",
						},
					},
					Required: []string{"plan"},
				},
			},
		}

		var subHandlers = map[string]toolHandler{
			"bash":       runBash,
			"read_file":  runRead,
			"write_file": runWrite,
			"send_message": func(input map[string]any) (string, error) {
				to, ok := input["to"].(string)
				if !ok || to == "" {
					return "", fmt.Errorf("to is required.")
				}
				content, ok := input["content"].(string)
				if !ok || content == "" {
					return "", fmt.Errorf("content is required.")
				}
				err := BUS.Send(name, to, content, "message", map[string]any{})
				if err != nil {
					return "", err
				}

				return "Sent", nil
			},
			"submit_plan": func(input map[string]any) (string, error) {
				plan, ok := input["plan"].(string)
				if !ok || plan == "" {
					return "", fmt.Errorf("plan is required.")
				}
				return teammateSubmitPlan(name, plan)
			},
		}
		state := recovery.InitRecoveryState()
		shutDownRequested := false

		for !shutDownRequested {
			inbox, err := BUS.ReadInbox(name)

			shouldStop := false
			nonProtocol := make([]message, 0)
			for _, msg := range inbox {
				if msg.MessageType == protocol.ShutDownRequest || msg.MessageType == protocol.PlanApprovalResponse {
					shouldStop, err = handleInboxMessage(name, msg, &messages)
					if err != nil {
						fmt.Println(err.Error())
						return
					}
					if shouldStop {
						break
					}
				} else {
					nonProtocol = append(nonProtocol, msg)
				}
			}
			if shouldStop {
				shutDownRequested = true
				break
			}
			if len(nonProtocol) > 0 {
				inboxJson, err := json.Marshal(nonProtocol)
				if err != nil {
					fmt.Printf("marshal error: %v\n", err)
					return
				}
				messages = append(messages,
					anthropic.NewUserMessage(
						anthropic.NewTextBlock(
							fmt.Sprintf("<inbox>%v"+
								"</inbox>", inboxJson,
							)),
					),
				)

			}
			// if err != nil {
			// 	fmt.Println(err.Error())
			// 	return
			// }
			// if len(inbox) > 0 {
			// 	rawdata, err := json.Marshal(inbox)
			// 	if err != nil {
			// 		fmt.Println(err.Error())
			// 		return
			// 	}
			// 	data := string(rawdata)
			// 	messages = append(messages,
			// 		anthropic.NewUserMessage(
			// 			anthropic.NewTextBlock(fmt.Sprintf("<inbox>%v</inbox>", data)),
			// 		),
			// 	)
			// }

			requestMessage := messages
			if len(requestMessage) > 20 {
				requestMessage = requestMessage[len(requestMessage)-20:]
			}

			// 调用LLM， retry
			response, err := recovery.WithRetry(
				func() (*anthropic.Message, error) {
					return config.Client.Messages.New(
						ctx,
						anthropic.MessageNewParams{
							MaxTokens: config.DEFAULT_MAX_TOOKENS,
							Messages:  requestMessage,
							Model:     state.CurrentModel,
							Tools:     buildTools(subTools),
							System:    system,
						},
					)
				},
				state,
				10,
			)
			if err != nil {
				fmt.Println(err.Error())
				return
			}

			messages = append(messages, anthropic.NewAssistantMessage(
				response.ToParam().Content...,
			))
			if response.StopReason != anthropic.StopReasonToolUse {
				for !shutDownRequested {
					time.Sleep(1 * time.Second)
					inbox, err := BUS.ReadInbox(name)
					if err != nil {
						fmt.Printf("read inbox error: %v\n", err)
					}
					if len(inbox) == 0 {
						continue
					}
					resumeWork := false
					for _, msg := range inbox {
						if msg.MessageType == protocol.ShutDownRequest || msg.MessageType == protocol.PlanApprovalResponse {
							shouldStop, err := handleInboxMessage(name, msg, &messages)
							if err != nil {
								fmt.Printf("handle inbox error: %v\n", err)
								return
							}
							if shouldStop {
								shutDownRequested = true
								break
							}

							if msg.MessageType == protocol.PlanApprovalResponse {
								resumeWork = true
							}

						} else {
							nonProtocol = append(nonProtocol, msg)
						}
					}
					if shutDownRequested {
						break
					}

					if resumeWork {
						break
					}

					if len(nonProtocol) > 0 {
						inboxJson, err := json.Marshal(nonProtocol)
						if err != nil {
							fmt.Printf("marshall error: %v\n", err)
							return
						}
						messages = append(messages,
							anthropic.NewUserMessage(
								anthropic.NewTextBlock(
									fmt.Sprintf("<inbox>%v"+
										"</inbox>", inboxJson,
									)),
							),
						)
						break
					}

				}
				continue

			}
			results := make([]anthropic.ContentBlockParamUnion, 0)
			for _, block := range response.Content {

				switch block.AsAny().(type) {
				case anthropic.ToolUseBlock:
					handler, ok := subHandlers[block.Name]
					if !ok {
						results = append(results,
							anthropic.NewToolResultBlock(
								block.ID,
								fmt.Sprintf("Unkown tool: %v", block.Name),
								true,
							),
						)
						continue
					}

					var input map[string]any
					err := json.Unmarshal(block.Input, &input)
					if err != nil {
						fmt.Println(err.Error())
						return
					}
					output, err := handler(input)
					if err != nil {
						fmt.Println(err.Error())
						return
					}
					results = append(results, anthropic.NewToolResultBlock(
						block.ID,
						output,
						false,
					))

				}

			}
			messages = append(messages, anthropic.NewUserMessage(results...))
		}

		summary := "Done."
		found := false
		for i := len(messages) - 1; i >= 0; i-- {
			msg := messages[i]
			if msg.Role != anthropic.MessageParamRoleAssistant {
				continue
			}
			for _, b := range msg.Content {
				if b.GetType() == nil || *b.GetType() != "text" || b.OfText == nil {
					continue
				}
				summary = *b.GetText()
				found = true
				break
			}
			if found {
				break
			}
		}
		err := BUS.Send(name, "lead", summary, "result", map[string]any{})
		if err != nil {
			fmt.Println(errors.New("send eroor").Error())
		}

		activeMu.Lock()
		delete(activeTeammate, name)
		activeMu.Unlock()

		fmt.Printf("  \033[32m[teammate] %v finished\033[0m\n", name)
	}()

	fmt.Printf("  \033[36m[teammate] %v spawned as %v\033[0m\n", name, role)

	return fmt.Sprintf("Teammate '%v' spawned as %v\n", name, role), nil

}

func teammateSubmitPlan(fromName string, plan string) (string, error) {

	requestID := protocol.NewRequestID()

	protocol.AddPendingRequests(&protocol.ProtocolState{
		RequestID:      requestID,
		ProtocolType:   protocol.PlanApproval,
		Sender:         fromName,
		Target:         "lead",
		ProtocolStatus: protocol.Pending,
		Payload:        plan,
	})

	err := BUS.Send(fromName, "lead", plan, protocol.PlanApprovalRequest, map[string]any{"request_id": requestID})
	if err != nil {
		return "", fmt.Errorf("send error: %v", err)
	}

	return fmt.Sprintf("Plan submitted (%v). Waiting for approval...\n", requestID), nil

}

func handleInboxMessage(name string, msg message, messages *[]anthropic.MessageParam) (bool, error) {
	msgType := "message"
	msgType = msg.MessageType

	meta := msg.Metadata
	requestID := ""
	if msg.Metadata != nil {
		v, ok := msg.Metadata["request_id"].(string)
		if ok {
			requestID = v
		}
	}

	if msgType == protocol.ShutDownRequest {
		err := BUS.Send(name, "lead", "Shutting down gracefully.",
			protocol.ShutDownResponse, map[string]any{
				"request_id": requestID,
				"approve":    true,
			},
		)
		if err != nil {
			return false, err
		}

		fmt.Printf("  \033[35m[protocol] %v approved shutdown "+
			"(%v)\033[0m",
			name,
			requestID,
		)
		return true, nil
	}

	if msgType == protocol.PlanApprovalResponse {
		approve := false
		approve = meta["approve"].(bool)
		if approve {

			*messages = append(*messages, anthropic.NewUserMessage(
				anthropic.NewTextBlock("[Plan approved] Proceed with the task."),
			))
		} else {
			*messages = append(*messages,
				anthropic.NewUserMessage(
					anthropic.NewTextBlock(fmt.Sprintf("[Plan rejected] Feedback: %v", msg.Content)),
				),
			)
		}
		return false, nil
	}
	return false, nil

}
