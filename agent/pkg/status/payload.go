package status

import "time"

// AgentStatus represents the payload sent by the agent to the central server.
type AgentStatusPayload struct {
	LastAppliedCommit string    `json:"lastAppliedCommit"`
	IsDrifted         bool      `json:"isDrifted"`
	Status            string    `json:"status"` 
	ErrorMessage      string    `json:"errorMessage,omitempty"` 
	Timestamp         time.Time `json:"timestamp"`
} 