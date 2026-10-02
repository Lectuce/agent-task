package session

import (
	"agent/background"
	"agent/logx"
	"fmt"
	"log"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

type Session struct {
	ID              string
	Messages        []anthropic.MessageParam
	RoundsSinceTodo int
	Logger          *log.Logger
	Background      *background.Manager
}

type SessionManager struct {
	Sessions map[string]*Session
	Current  string
}

func NewSessionManager() *SessionManager {
	sessionLogger, err := logx.NewLogger("default")
	if err != nil {
		panic(fmt.Sprintf("failed to create default session logger: %v", err))
	}

	return &SessionManager{
		Sessions: map[string]*Session{
			"default": {
				ID:         "default",
				Messages:   []anthropic.MessageParam{},
				Logger:     sessionLogger,
				Background: background.NewManager(),
			},
		},
		Current: "default",
	}

}

func (sm *SessionManager) NewSession(name string) bool {
	logger, err := logx.NewLogger(name)
	if err != nil {
		return false
	}

	_, exists := sm.Sessions[name]
	if exists {
		return false
	}

	sm.Sessions[name] = &Session{
		ID:         name,
		Messages:   []anthropic.MessageParam{},
		Logger:     logger,
		Background: background.NewManager(),
	}
	sm.Current = name
	return true
}

func (sm *SessionManager) CurrentSession() *Session {
	return sm.Sessions[sm.Current]
}

func (sm *SessionManager) HandleCommand(query string) bool {

	// 新session
	if strings.HasPrefix(query, "/session new") {
		name := strings.TrimSpace(strings.TrimPrefix(query, "/session new"))
		if name == "" {
			fmt.Println("session name is required")
			return true
		}

		if !sm.NewSession(name) {
			fmt.Printf("session already exists: %s\n", name)
			return true
		}

		sm.Current = name
		fmt.Printf("created and switch to session: %s\n", name)
		return true
	}

	// 切换 session
	if strings.HasPrefix(query, "/session switch") {
		name := strings.TrimSpace(
			strings.TrimPrefix(
				query,
				"/session switch ",
			),
		)
		_, ok := sm.Sessions[name]
		if !ok {
			fmt.Printf(
				"session not found: %s\n",
				name,
			)
			return true
		}
		sm.Current = name
		fmt.Printf("switched to session: %s\n", name)
		return true
	}

	// 查看 session
	if query == "/session list" {
		for name := range sm.Sessions {
			if name == sm.Current {
				fmt.Printf("* %s\n", name)
			} else {
				fmt.Printf("  %s\n", name)
			}
		}
		return true
	}

	return false
}
