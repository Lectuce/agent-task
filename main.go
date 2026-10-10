package main

import (
	"agent/config"
	"agent/cron"
	"agent/hook"
	"agent/loop"
	"agent/mcpclient"
	"agent/prompt"
	"agent/session"
	"agent/skills"
	"agent/teams"
	"agent/tool"
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/chzyer/readline"
)

func main() {
	err := skills.ScanSkill()
	if err != nil {
		fmt.Println(err.Error())
	}
	hook.Register()
	fmt.Println("输入问题，回车发送。输入 q 退出。")
	sessionManager := session.NewSessionManager()
	promptContext, err := prompt.UpdateContext(prompt.PromptContext{}, nil)
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	bus, err := teams.NewMessageBus()
	if err != nil {
		fmt.Println(err.Error())
	}
	teams.SetMessageBus(bus)
	tool.SetMessageBus(bus)

	cronManager := cron.NewManager(config.DURABLE_PATH)
	err = cronManager.LoadDurableJobs()
	if err != nil {
		fmt.Println(err.Error())
	}
	tool.SetCronManager(cronManager)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	mcpManager := mcpclient.NewManager()
	defer func() {
		err := mcpManager.Close()
		if err != nil {
			fmt.Printf("[MCP close error] %v\n", err)
		}
	}()

	serverConfig := mcpclient.ServerConfig{
		Name:      config.MCP_SERVER_NAME,
		Enabled:   config.MCP_ENABLED,
		Required:  config.MCP_REQUIRED,
		Transport: config.MCP_TRANSPORT,

		URL:     config.MCP_URL,
		Headers: config.MCPHeaders(),

		Command: config.MCP_COMMAND,
		Args:    config.MCP_ARGS,

		Timeout:    config.MCP_TIMEOUT,
		AllowTools: config.MCP_ALLOW_TOOLS,
		DenyTools:  config.MCP_DENY_TOOLS,
	}

	if serverConfig.Enabled {
		var connectErr error

		switch serverConfig.Transport {
		case "http":
			if serverConfig.URL == "" {
				connectErr = fmt.Errorf("MCP_URL is required for HTTP transport")
			}

		case "stdio":
			if serverConfig.Command == "" {
				connectErr = fmt.Errorf("MCP_COMMAND is required for stdio transport")
			}

		default:
			connectErr = fmt.Errorf("unsupported MCP transport: %s", serverConfig.Transport)
		}

		if connectErr == nil {
			connectErr = mcpManager.Connect(ctx, serverConfig)
		}

		if connectErr != nil {
			fmt.Printf("[MCP error] %v\n", connectErr)

			if serverConfig.Required {
				return
			}
		}
	}

	go cronManager.SchedulerLoop(ctx)
	go loop.QueueProcessorLoop(cronManager, ctx, promptContext, func() *session.Session {
		return sessionManager.CurrentSession()
	}, mcpManager)
	r, err := readline.New("agent[default] >> ")
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	defer r.Close()

	for {
		r.SetPrompt(fmt.Sprintf("agent[%s] >> ", sessionManager.Current))

		query, err := r.Readline()
		if err != nil {
			fmt.Println(err.Error())
		}
		if query == "q" || query == "exit" || query == "" {
			return
		}

		if sessionManager.HandleCommand(query) {
			continue
		}

		hookCtx := &hook.HookContext{
			Query: query,
		}
		hook.TriggerHooks(hook.UserPromptSubmit, hookCtx)
		query = hookCtx.Query

		loop.AgentLock.Lock()
		err = func() error {
			defer loop.AgentLock.Unlock()

			currentSession := sessionManager.CurrentSession()

			err := loop.AgentLoop(query, ctx, promptContext, currentSession, cronManager, mcpManager)
			if err != nil {
				return err
			}

			inboxText, count, err := teams.ConsumeLeadInboxText()
			if err != nil {
				return fmt.Errorf("drain lead inbox: %v", err)
			}
			if inboxText != "" {
				currentSession.Messages = append(currentSession.Messages,
					anthropic.NewUserMessage(
						anthropic.NewTextBlock(inboxText),
					),
				)
			}

			fmt.Printf("\n\033[33m[Inbox: %v messages injected]\033[0m\n", count)

			return nil
		}()

		if err != nil {
			fmt.Printf("[agent error] %v\n", err)
		}
	}

}
