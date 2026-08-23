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

type Job struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Command        string     `json:"command"`
	Status         Status     `json:"status"`
	CreatedAt      time.Time  `json:"createdAt"`
	StartedAt      *time.Time `json:"startedAt,omitempty"`
	CompletedAt    *time.Time `json:"completedAt,omitempty"`
	AssignedNodeID string     `json:"assignedNodeId,omitempty"`
	Result         string     `json:"result,omitempty"`
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
		return next == StatusSucceeded || next == StatusFailed
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
