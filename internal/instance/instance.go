package instance

import (
	"time"
)

type InstanceStatus string

const (
	StatusPending  InstanceStatus = "PENDING"
	StatusAssigned InstanceStatus = "ASSIGNED"
	StatusStarting InstanceStatus = "STARTING"
	StatusRunning  InstanceStatus = "RUNNING"
	StatusStopping InstanceStatus = "STOPPING"
	StatusCrashed  InstanceStatus = "CRASHED"
	StatusStopped  InstanceStatus = "STOPPED"
	StatusUnknown  InstanceStatus = "UNKNOWN"
)

type InstanceHealth string

const (
	HealthHealthy   InstanceHealth = "HEALTHY"
	HealthUnhealthy InstanceHealth = "UNHEALTHY"
	HealthUnknown   InstanceHealth = "UNKNOWN"
)

type Instance struct {
	ID            string         `json:"id"`
	ApplicationID string         `json:"application_id"`
	DeploymentID  string         `json:"deployment_id"`
	NodeID        string         `json:"node_id,omitempty"`
	ExecutionID   string         `json:"execution_id,omitempty"`
	Status        InstanceStatus `json:"status"`
	Health        InstanceHealth `json:"health,omitempty"`
	ContainerID   string         `json:"container_id,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	StartedAt     *time.Time     `json:"started_at,omitempty"`
	StoppedAt     *time.Time     `json:"stopped_at,omitempty"`
	UnknownSince  *time.Time     `json:"unknown_since,omitempty"`
}

// DeepCopy creates a complete copy of the Instance.
func (i *Instance) DeepCopy() Instance {
	copy := *i

	if i.StartedAt != nil {
		t := *i.StartedAt
		copy.StartedAt = &t
	}

	if i.StoppedAt != nil {
		t := *i.StoppedAt
		copy.StoppedAt = &t
	}

	if i.UnknownSince != nil {
		t := *i.UnknownSince
		copy.UnknownSince = &t
	}

	return copy
}
