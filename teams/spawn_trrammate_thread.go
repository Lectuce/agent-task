package teams

import (
	"agent/config"
	"agent/recovery"
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
)

var activeTeammate = map[string]bool{}

func SpawnTeammateThread(name string, role string, prompt string) (string, error) {
	_, ok := activeTeammate[name]
	if ok {
		return "", fmt.Errorf("Teammate %v already exists.", name)
	}

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
	activeTeammate[name] = true

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
				BUS.Send(name, to, content, "message")

				return "Sent", nil
			},
		}
		state := recovery.InitRecoveryState()

		for range 10 {
			inbox, err := BUS.ReadInbox(name)
			if err != nil {
				fmt.Println(err.Error())
				return
			}
			if len(inbox) > 0 {
				rawdata, err := json.Marshal(inbox)
				if err != nil {
					fmt.Println(err.Error())
					return
				}
				data := string(rawdata)
				messages = append(messages,
					anthropic.NewUserMessage(
						anthropic.NewTextBlock(fmt.Sprintf("<inbox>%v</inbox>", data)),
					),
				)
			}

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
				break
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
		BUS.Send(name, "lead", summary, "result")
		delete(activeTeammate, name)
		fmt.Printf("  \033[32m[teammate] %v finished\033[0m\n", name)
	}()

	fmt.Printf("  \033[36m[teammate] %v spawned as %v\033[0m\n", name, role)

	return fmt.Sprintf("Teammate '%v' spawned as %v\n", name, role), nil

}
