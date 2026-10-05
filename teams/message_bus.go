package teams

import (
	"agent/config"
	"agent/protocol"
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
	From        string         `json:"from"`
	To          string         `json:"to"`
	Content     string         `json:"content"`
	MessageType string         `json:"message_type"`
	Ts          time.Time      `json:"ts"`
	Metadata    map[string]any `json:"meta_data"`
}

func NewMessageBus() (*MessageBus, error) {
	err := os.MkdirAll(config.MAILBOX_DIR, 0755)
	if err != nil {
		return nil, err
	}
	return &MessageBus{}, err
}

func (m *MessageBus) Send(fromAgent string, toAgent string, content string, messageType string, metadata map[string]any) error {
	message := message{
		From:        fromAgent,
		To:          toAgent,
		Content:     content,
		MessageType: messageType,
		Ts:          time.Now(),
		Metadata:    metadata,
	}

	inbox := filepath.Join(config.MAILBOX_DIR, toAgent+".jsonl")
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	f, err := os.OpenFile(inbox, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
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

func (m *MessageBus) DrainInboxText(agent string) (string, int, error) {
	msgs, err := m.ReadInbox(agent)
	if err != nil {
		return "", 0, err
	}
	if len(msgs) == 0 {
		return "", 0, nil
	}

	lines := make([]string, 0, len(msgs))

	for _, msg := range msgs {
		preview := msg.Content

		if len(preview) > 200 {
			preview = preview[:200]
		}

		lines = append(lines, fmt.Sprintf("From %v: %v", msg.From, preview))

	}
	return "[Inbox]\n" + strings.Join(lines, "\n"), len(msgs), nil

}

func ConsumeLeadInbox(routeProtocol bool) ([]message, error) {

	msgs, err := BUS.ReadInbox("lead")
	if err != nil {
		return nil, err
	}

	if len(msgs) == 0 {
		return nil, nil
	}

	if routeProtocol {
		for _, msg := range msgs {

			requestID := ""

			if msg.Metadata != nil {
				v, ok := msg.Metadata["request_id"].(string)
				if ok {
					requestID = v
				}
			}

			if requestID == "" {
				continue
			}

			if !strings.HasSuffix(msg.MessageType, "_response") {
				continue
			}

			approve := false

			if msg.Metadata != nil {
				v, ok := msg.Metadata["approve"].(bool)
				if ok {
					approve = v
				}
			}
			protocol.MatchResponse(msg.MessageType, requestID, approve)
		}
	}

	return msgs, nil
}
