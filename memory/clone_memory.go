package memory

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
)

func CloneMessages(messages []anthropic.MessageParam) ([]anthropic.MessageParam, error) {

	data, err := json.Marshal(messages)
	if err != nil {
		return nil, err
	}

	var cloned []anthropic.MessageParam

	err = json.Unmarshal(data, &cloned)
	if err != nil {
		return nil, err
	}

	return cloned, nil
}

func AppendMessage(history []anthropic.MessageParam, requestMessages []anthropic.MessageParam, message anthropic.MessageParam) ([]anthropic.MessageParam, []anthropic.MessageParam, error) {

	history = append(history, message)

	cloned, err := CloneMessages(
		[]anthropic.MessageParam{
			message,
		},
	)
	if err != nil {
		return nil, nil, err
	}

	requestMessages = append(
		requestMessages,
		cloned[0],
	)

	return history, requestMessages, nil

}
