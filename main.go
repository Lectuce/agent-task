package main

import (
	"agent/config"
	"agent/cron"
	"agent/hook"
	"agent/loop"
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
	go cronManager.SchedulerLoop(ctx)
	go loop.QueueProcessorLoop(cronManager, ctx, promptContext, func() *session.Session {
		return sessionManager.CurrentSession()
	})
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
			return loop.AgentLoop(query, ctx, promptContext, sessionManager.CurrentSession(), cronManager)
		}()
	}

}
