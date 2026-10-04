package teams

import (
	"agent/config"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type MessageBus struct {
	message *message
	mu      sync.Mutex
}

type message struct {
	From        string    `json:"from"`
	To          string    `json:"to"`
	Content     string    `json:"content"`
	MessageType string    `json:"message_type"`
	Ts          time.Time `json:"ts"`
}

func NewMessageBus() (*MessageBus, error) {
	err := os.MkdirAll(config.MAILBOX_DIR, 0755)
	if err != nil {
		return nil, err
	}
	return &MessageBus{}, err
}

func (m *MessageBus) Send(fromAgent string, toAgent string, content string, messageType string) error {
	message := message{
		From:        fromAgent,
		To:          toAgent,
		Content:     content,
		MessageType: messageType,
		Ts:          time.Now(),
	}

	inbox := filepath.Join(config.MAILBOX_DIR, toAgent+".jsonl")
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	f, err := os.OpenFile(inbox, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	_, err = f.Write(append(data, '\n'))
	defer f.Close()

	if err != nil {
		return err
	}
	preview := content
	if len(preview) > 50 {
		preview = preview[:50]
	}

	fmt.Printf("  \033[33m[bus] %v → %v: %v\033[0m", fromAgent, toAgent, preview)

	return nil
}

func (m *MessageBus) ReadInbox(agent string) ([]message, error) {
	inbox := config.MAILBOX_DIR + fmt.Sprintf("/%v.jsonl", agent)

	m.mu.Lock()
	defer m.mu.Unlock()
	lines, err := os.ReadFile(inbox)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	msgs := make([]message, 0)
	for _, line := range strings.Split(string(lines), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var msg message
		err = json.Unmarshal([]byte(line), &msg)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, msg)
	}
	err = os.Remove(inbox)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	}
	return msgs, nil
}
