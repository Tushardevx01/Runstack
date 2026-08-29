package job

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusAssigned  Status = "assigned"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

var (
	ErrInvalidTransition = errors.New("invalid job status transition")
	ErrJobNotFound       = errors.New("job not found")
)

type JobResult struct {
	ExitCode int    `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Error    string `json:"error,omitempty"`
}

type Job struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Command        string     `json:"command"`
	Status         Status     `json:"status"`
	CreatedAt      time.Time  `json:"createdAt"`
	StartedAt      *time.Time `json:"startedAt,omitempty"`
	CompletedAt    *time.Time `json:"completedAt,omitempty"`
	AssignedNodeID string     `json:"assignedNodeId,omitempty"`
	ExecutionID    string     `json:"executionId,omitempty"`
	CPU            float64    `json:"cpu,omitempty"`
	MemoryMB       int        `json:"memory_mb,omitempty"`
	MaxRetries     int        `json:"maxRetries"`
	Attempts       int        `json:"attempts"`
	Result         *JobResult `json:"result,omitempty"`
}

type JobEventType string

const (
	EventCreated   JobEventType = "CREATED"
	EventAssigned  JobEventType = "ASSIGNED"
	EventClaimed   JobEventType = "CLAIMED"
	EventSucceeded JobEventType = "SUCCEEDED"
	EventFailed    JobEventType = "FAILED"
	EventRecovered JobEventType = "RECOVERED"
	EventRetried   JobEventType = "RETRIED"
)

type JobEvent struct {
	ID          string       `json:"id"`
	JobID       string       `json:"jobId"`
	Timestamp   time.Time    `json:"timestamp"`
	Type        JobEventType `json:"type"`
	From        Status       `json:"from"`
	To          Status       `json:"to"`
	NodeID      string       `json:"nodeId,omitempty"`
	ExecutionID string       `json:"executionId,omitempty"`
	Attempts    int          `json:"attempts,omitempty"`
	Reason      string       `json:"reason,omitempty"`
}

func isValidTransition(current, next Status) bool {
	if current == next {
		return true // No-op transition is safely ignored
	}

	switch current {
	case StatusPending:
		return next == StatusAssigned || next == StatusFailed
	case StatusAssigned:
		return next == StatusRunning || next == StatusFailed || next == StatusPending
	case StatusRunning:
		return next == StatusSucceeded || next == StatusFailed || next == StatusPending
	case StatusSucceeded, StatusFailed:
		return false
	default:
		return false
	}
}

func GenerateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
