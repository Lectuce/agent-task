package protocol

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// ProtocolType
const (
	ShutDown     = "shutdown"
	PlanApproval = "plan_approval"
)

// ResponseType
const (
	ShutDownRequest      = "shutdown_request"
	ShutDownResponse     = "shutdown_response"
	PlanApprovalResponse = "plan_approval_response"
	PlanApprovalRequest  = "plan_approval_request"
)

// ProtocolStatus
const (
	Pending  = "pending"
	Approved = "approved"
	Rejected = "rejected"
)

type ProtocolState struct {
	RequestID      string
	ProtocolType   string
	Sender         string
	Target         string
	ProtocolStatus string
	Payload        string
	CreatedAt      time.Time
	Mu             sync.Mutex
}

var PendingRequests = map[string]*ProtocolState{}

func NewRequestID() string {
	return fmt.Sprintf("req_%06d", rand.Intn(1000000))
}

func MatchResponse(responseType string, requestID string, approve bool) {

	state := PendingRequests[requestID]

	if state == nil {
		fmt.Printf("  \033[31m[protocol] unknown request_id: %v\033[0m\n", requestID)
		return

	}

	if state.ProtocolType == ShutDown && responseType != ShutDownResponse {
		fmt.Printf("  \033[31m[protocol] type mismatch: expected shutdown_response,"+
			"got %v\033[0m\n", responseType)
		return
	}

	if state.ProtocolStatus == PlanApproval && responseType != PlanApprovalResponse {
		fmt.Printf("  \033[31m[protocol] type mismatch: expected plan_approval_response, "+
			"got %v\033[0m", responseType)
	}

	if state.ProtocolStatus != Pending {
		fmt.Printf("  \033[33m[protocol] %v already %v, "+
			"ignoring duplicate\033[0m\n", requestID, state.ProtocolStatus,
		)
		return
	}

	var icon string
	var color string
	if approve {
		state.ProtocolStatus = Approved
		icon = "✓"
		color = "32"
	} else {
		state.ProtocolStatus = Rejected
		icon = "✗"
		color = "31"
	}

	fmt.Printf("  \033[%vm[protocol] %v %v "+
		"(%v: %v)\033[0m\n",
		color, state.ProtocolType, icon, requestID, state.ProtocolStatus,
	)
}
