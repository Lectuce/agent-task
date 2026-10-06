package tool

import (
	"agent/protocol"
	"fmt"
)

func runRequestShudown(input map[string]any) (string, error) {
	teammate, ok := input["teammate"].(string)
	if !ok || teammate == "" {
		return "", fmt.Errorf("teammate is required")
	}

	requestID := protocol.NewRequestID()

	protocol.AddPendingRequests(&protocol.ProtocolState{
		RequestID:      requestID,
		ProtocolType:   protocol.ShutDown,
		Sender:         "lead",
		Target:         teammate,
		ProtocolStatus: protocol.Pending,
		Payload:        "",
	})

	err := MessageBus.Send("lead", teammate, "Please shut down gracefully.", "shutdown_request", map[string]any{"request_id": requestID})
	if err != nil {
		return "", fmt.Errorf("send error: %v", err)
	}

	fmt.Printf("  \033[35m[protocol] shutdown_request → %v "+
		"(%v)\033[0m", teammate, requestID)

	return fmt.Sprintf("Shutdown request sent to %v (req: %v)\n", teammate, requestID), nil
}

func runRequestPlan(input map[string]any) (string, error) {

	teammate, ok := input["teammate"].(string)
	if !ok || teammate == "" {
		return "", fmt.Errorf("teammate is required.")
	}

	task, ok := input["task"].(string)
	if !ok || task == "" {
		return "", fmt.Errorf("task is required.")
	}

	err := MessageBus.Send("lead",
		teammate,
		fmt.Sprintf("Please submit a plan for: %v", task),
		"message",
		map[string]any{},
	)
	if err != nil {
		return "", fmt.Errorf("send error: %v", err)
	}

	return fmt.Sprintf("Asked %v to submit a plan", teammate), nil

}

func runReviewPlan(input map[string]any) (string, error) {

	requestID, ok := input["request_id"].(string)
	if !ok || requestID == "" {
		return "", fmt.Errorf("request_id is required.")
	}
	approve := false
	approve, ok = input["approve"].(bool)
	if !ok {
		return "", fmt.Errorf("approve is required.")
	}

	feedback := ""
	v, ok := input["feedback"].(string)
	if ok {
		feedback = v
	}

	state, ok := protocol.GetPendingRequest(requestID)
	if !ok || state == nil {
		return "", fmt.Errorf("request %v not found", requestID)
	}
	state.Mu.Lock()

	if state.ProtocolStatus != protocol.Pending {
		status := state.ProtocolStatus
		state.Mu.Unlock()
		return "", fmt.Errorf("Request %v already %v", requestID, status)
	}

	status := protocol.Rejected
	if approve {
		status = protocol.Approved
	}
	state.ProtocolStatus = status

	content := feedback
	if content == "" {
		if approve {
			content = protocol.Approved
		} else {
			content = protocol.Rejected
		}
	}

	sender := state.Sender
	state.Mu.Unlock()

	err := MessageBus.Send(
		"lead",
		sender,
		content,
		protocol.PlanApprovalResponse,
		map[string]any{
			"request_id": requestID,
			"approve":    approve,
		},
	)
	if err != nil {
		return "", fmt.Errorf("send error: %v", err)
	}

	icon := "✗"
	if approve {
		icon = "✓"
	}

	fmt.Printf("  \033[32m[protocol] plan %v (%v)\033[0m\n", icon, requestID)
	return fmt.Sprintf("Plan %v (%v)", status, requestID), nil

}
