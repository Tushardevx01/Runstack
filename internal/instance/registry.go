package instance

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

var (
	ErrNotFound               = errors.New("instance not found")
	ErrInvalidStateTransition = errors.New("invalid instance state transition")
)

type Registry struct {
	mu        sync.RWMutex
	instances map[string]Instance
}

func NewRegistry() *Registry {
	return &Registry{
		instances: make(map[string]Instance),
	}
}

func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (r *Registry) Create(appID, deploymentID string) (Instance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	inst := Instance{
		ID:            generateID(),
		ApplicationID: appID,
		DeploymentID:  deploymentID,
		Status:        StatusPending,
		CreatedAt:     time.Now().UTC(),
	}

	r.instances[inst.ID] = inst.DeepCopy()
	return inst.DeepCopy(), nil
}

func (r *Registry) Get(id string) (Instance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	inst, exists := r.instances[id]
	if !exists {
		return Instance{}, ErrNotFound
	}
	return inst.DeepCopy(), nil
}

func (r *Registry) List() []Instance {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Instance
	for _, inst := range r.instances {
		result = append(result, inst.DeepCopy())
	}
	return result
}

// UpdateState allows updating the lifecycle state of an instance.
// In a full implementation, this would enforce a strict state machine.
func (r *Registry) UpdateState(id string, status InstanceStatus, nodeID string, containerID string) (Instance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	inst, exists := r.instances[id]
	if !exists {
		return Instance{}, ErrNotFound
	}

	// Basic state transition validation could go here, for now we just allow it
	// if design allows it. But let's add some basics:
	if inst.Status == StatusStopped || inst.Status == StatusCrashed {
		// Terminal states cannot be changed. Milestone 7B requires immutability of failed instances.
		// Reconciler creates NEW instances instead.
		return Instance{}, ErrInvalidStateTransition
	}

	inst.Status = status
	if nodeID != "" {
		inst.NodeID = nodeID
	}
	if containerID != "" {
		inst.ContainerID = containerID
	}

	if status == StatusRunning && inst.StartedAt == nil {
		now := time.Now().UTC()
		inst.StartedAt = &now
	}

	if (status == StatusStopped || status == StatusCrashed) && inst.StoppedAt == nil {
		now := time.Now().UTC()
		inst.StoppedAt = &now
	}

	r.instances[id] = inst.DeepCopy()
	return inst.DeepCopy(), nil
}

// Claim atomically assigns an ExecutionID to an ASSIGNED instance.
func (r *Registry) Claim(id string, nodeID string) (Instance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	inst, exists := r.instances[id]
	if !exists {
		return Instance{}, ErrNotFound
	}

	if inst.Status != StatusAssigned {
		return Instance{}, ErrInvalidStateTransition
	}
	if inst.NodeID != nodeID {
		return Instance{}, errors.New("node ID mismatch")
	}

	inst.Status = StatusStarting
	inst.ExecutionID = generateID()

	r.instances[id] = inst.DeepCopy()
	return inst.DeepCopy(), nil
}

// ReportStatus safely updates the instance status from the Agent.
func (r *Registry) ReportStatus(id string, nodeID string, executionID string, status InstanceStatus, health InstanceHealth, containerID string) (Instance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	inst, exists := r.instances[id]
	if !exists {
		return Instance{}, ErrNotFound
	}

	if inst.NodeID != nodeID {
		return Instance{}, errors.New("node ID mismatch")
	}
	if inst.ExecutionID != executionID {
		return Instance{}, errors.New("stale execution ID")
	}

	// Allowed Agent transitions:
	// STARTING -> RUNNING, CRASHED, STOPPED
	// RUNNING -> CRASHED, STOPPED
	// STOPPING -> STOPPED, CRASHED
	// Wait, agent doesn't set UNKNOWN.
	if status == StatusPending || status == StatusAssigned || status == StatusStarting || status == StatusUnknown {
		return Instance{}, ErrInvalidStateTransition
	}

	// Idempotency check
	if inst.Status == status && inst.Health == health {
		if containerID != "" && inst.ContainerID == "" {
			inst.ContainerID = containerID
			r.instances[id] = inst.DeepCopy()
		}
		return inst.DeepCopy(), nil
	}

	if inst.Status == StatusStopped || inst.Status == StatusCrashed {
		return Instance{}, ErrInvalidStateTransition
	}

	inst.Status = status
	if health != "" {
		inst.Health = health
	}

	// Observation returned, clear UnknownSince
	inst.UnknownSince = nil

	if containerID != "" && inst.ContainerID == "" {
		inst.ContainerID = containerID
	}

	now := time.Now().UTC()
	if status == StatusRunning && inst.StartedAt == nil {
		inst.StartedAt = &now
	} else if (status == StatusStopped || status == StatusCrashed) && inst.StoppedAt == nil {
		inst.StoppedAt = &now
	}

	r.instances[id] = inst.DeepCopy()
	return inst.DeepCopy(), nil
}

func (r *Registry) MarkUnknown(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	inst, exists := r.instances[id]
	if !exists {
		return ErrNotFound
	}

	if inst.Status == StatusStopped || inst.Status == StatusCrashed {
		return nil // terminal, do not mark unknown
	}

	inst.Status = StatusUnknown
	if inst.UnknownSince == nil {
		now := time.Now().UTC()
		inst.UnknownSince = &now
	}

	r.instances[id] = inst.DeepCopy()
	return nil
}
