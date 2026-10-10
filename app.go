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

	"github.com/chzyer/readline"
)

type App struct {
	session   *session.SessionManager
	promptCtx *prompt.PromptContext
	cron      *cron.Manager
	mcp       *mcpclient.Manager
	reader    *readline.Instance
}

func newApp(ctx context.Context) (*App, error) {
	err := skills.ScanSkill()
	if err != nil {
		return nil, err
	}

	hook.Register()

	sessionManager := session.NewSessionManager()
	promptContext, err := prompt.UpdateContext(prompt.PromptContext{}, nil)
	if err != nil {
		return nil, err
	}

	bus, err := teams.NewMessageBus()
	if err != nil {
		return nil, err
	}
	teams.SetMessageBus(bus)
	tool.SetMessageBus(bus)

	cronManager := cron.NewManager(config.DURABLE_PATH)
	err = cronManager.LoadDurableJobs()
	if err != nil {
		return nil, err
	}
	tool.SetCronManager(cronManager)

	mcpManager, err := setupMCP(ctx)
	if err != nil {
		_ = mcpManager.Close()
		return nil, err
	}

	r, err := readline.New("agent[default] >> ")
	if err != nil {
		return nil, err
	}

	return &App{
		session:   sessionManager,
		promptCtx: promptContext,
		mcp:       mcpManager,
		reader:    r,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	fmt.Println("输入问题，回车发送。输入 q 退出。")

	go a.cron.SchedulerLoop(ctx)

	go loop.QueueProcessorLoop(
		a.cron,
		ctx,
		a.promptCtx,
		a.session.CurrentSession,
		a.mcp,
	)

	return a.runREPL(ctx)
}

func (a *App) Close() {

	if a.reader != nil {
		_ = a.reader.Close()
	}

	if a.mcp != nil {
		err := a.mcp.Close()
		if err != nil {
			fmt.Printf("[MCP close error] %v\n", err)
		}
	}

}
