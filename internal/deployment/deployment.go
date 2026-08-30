package deployment

import (
	"time"

	"github.com/Tushardevx01/runstack/internal/application"
)

type DeploymentStatus string

const (
	StatusActive     DeploymentStatus = "ACTIVE"
	StatusSuperseded DeploymentStatus = "SUPERSEDED"
	StatusFailed     DeploymentStatus = "FAILED"
	StatusRolledBack DeploymentStatus = "ROLLED_BACK"
)

type RolloutStatus string

const (
	RolloutPending     RolloutStatus = "PENDING"
	RolloutProgressing RolloutStatus = "PROGRESSING"
	RolloutPaused      RolloutStatus = "PAUSED"
	RolloutFailed      RolloutStatus = "FAILED"
	RolloutCompleted   RolloutStatus = "COMPLETED"
	RolloutRolledBack  RolloutStatus = "ROLLED_BACK"
)

type Deployment struct {
	ID                  string              `json:"id"`
	ApplicationID       string              `json:"application_id"`
	Version             int                 `json:"version"`
	SpecSnapshot        application.AppSpec `json:"spec_snapshot"`
	Hash                string              `json:"hash"`
	Status              DeploymentStatus    `json:"status"` // Overall status (ACTIVE/SUPERSEDED)
	RolloutStatus       RolloutStatus       `json:"rollout_status"`
	DesiredReplicas     int                 `json:"desired_replicas"`
	UpdatedReplicas     int                 `json:"updated_replicas"`
	ReadyReplicas       int                 `json:"ready_replicas"`
	UnavailableReplicas int                 `json:"unavailable_replicas"`
	BlockedReason       string              `json:"blocked_reason,omitempty"`
	ConsecutiveCrashes  int                 `json:"consecutive_crashes"`
	Degraded            bool                `json:"degraded"`
	CreatedAt           time.Time           `json:"created_at"`
}

// DeepCopy creates a complete copy of the Deployment.
func (d *Deployment) DeepCopy() Deployment {
	copy := *d

	// The SpecSnapshot needs to be deep copied.
	// Since AppSpec contains maps and slices, we need to manually copy them.

	if d.SpecSnapshot.Environment != nil {
		copy.SpecSnapshot.Environment = make(map[string]string, len(d.SpecSnapshot.Environment))
		for k, v := range d.SpecSnapshot.Environment {
			copy.SpecSnapshot.Environment[k] = v
		}
	}

	if d.SpecSnapshot.Ports != nil {
		copy.SpecSnapshot.Ports = make([]application.PortMapping, len(d.SpecSnapshot.Ports))
		for i, p := range d.SpecSnapshot.Ports {
			copy.SpecSnapshot.Ports[i] = p
		}
	}

	if d.SpecSnapshot.Command != nil {
		copy.SpecSnapshot.Command = make([]string, len(d.SpecSnapshot.Command))
		for i, v := range d.SpecSnapshot.Command {
			copy.SpecSnapshot.Command[i] = v
		}
	}

	if d.SpecSnapshot.Args != nil {
		copy.SpecSnapshot.Args = make([]string, len(d.SpecSnapshot.Args))
		for i, v := range d.SpecSnapshot.Args {
			copy.SpecSnapshot.Args[i] = v
		}
	}

	return copy
}
