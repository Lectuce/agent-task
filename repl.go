package main

import (
	"agent/hook"
	"agent/loop"
	"agent/teams"
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
)

func (a *App) runREPL(ctx context.Context) error {
	for {
		a.reader.SetPrompt(fmt.Sprintf("agent[%s] >> ", a.session.Current))

		query, err := a.reader.Readline()
		if err != nil {
			return err
		}
		if query == "q" || query == "exit" || query == "" {
			return nil
		}

		if a.session.HandleCommand(query) {
			continue
		}

		err = a.handleQuery(ctx, query)
		if err != nil {
			fmt.Printf("[agent error] %v\n", err)
			continue
		}

	}

}

func (a *App) handleQuery(ctx context.Context, query string) error {

	hookCtx := &hook.HookContext{
		Query: query,
	}
	hook.TriggerHooks(hook.UserPromptSubmit, hookCtx)
	query = hookCtx.Query

	loop.AgentLock.Lock()
	defer loop.AgentLock.Unlock()

	currentSession := a.session.CurrentSession()

	err := loop.AgentLoop(query, ctx, a.promptCtx, currentSession, a.cron, a.mcp)
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

}
