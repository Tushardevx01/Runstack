package job

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type Registry struct {
	mu     sync.RWMutex
	jobs   map[string]*Job
	events map[string][]JobEvent
}

func NewRegistry() *Registry {
	return &Registry{
		jobs:   make(map[string]*Job),
		events: make(map[string][]JobEvent),
	}
}

func (r *Registry) Create(name, command string) *Job {
	r.mu.Lock()
	defer r.mu.Unlock()

	j := &Job{
		ID:        "job-" + GenerateID(),
		Name:      name,
		Command:   command,
		Status:    StatusPending,
		CreatedAt: time.Now().UTC(),
	}

	r.jobs[j.ID] = j
	r.appendEvent(j.ID, EventCreated, "", StatusPending, "", "Job created")

	jobCopy := *j
	return &jobCopy
}

func (r *Registry) Get(id string) (*Job, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if j, ok := r.jobs[id]; ok {
		jobCopy := *j
		return &jobCopy, nil
	}
	return nil, ErrJobNotFound
}

func (r *Registry) List() []Job {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Job, 0, len(r.jobs))
	for _, j := range r.jobs {
		result = append(result, *j)
	}
	return result
}

func (r *Registry) appendEvent(jobID string, eventType JobEventType, from, to Status, nodeID string, reason string) {
	evt := JobEvent{
		ID:        fmt.Sprintf("%s-evt-%d", jobID, len(r.events[jobID])+1),
		JobID:     jobID,
		Timestamp: time.Now().UTC(),
		Type:      eventType,
		From:      from,
		To:        to,
		NodeID:    nodeID,
		Reason:    reason,
	}
	r.events[jobID] = append(r.events[jobID], evt)
}

func (r *Registry) GetEvents(id string) ([]JobEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if _, ok := r.jobs[id]; !ok {
		return nil, ErrJobNotFound
	}

	events := r.events[id]
	eventsCopy := make([]JobEvent, len(events))
	copy(eventsCopy, events)
	return eventsCopy, nil
}

type UpdateParams struct {
	Status         *Status
	AssignedNodeID *string
	StartedAt      *time.Time
	CompletedAt    *time.Time
	Result         *JobResult
}

func (r *Registry) Update(id string, params UpdateParams) (*Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	j, ok := r.jobs[id]
	if !ok {
		return nil, ErrJobNotFound
	}

	oldStatus := j.Status

	if params.Status != nil {
		if !isValidTransition(j.Status, *params.Status) {
			return nil, ErrInvalidTransition
		}
		j.Status = *params.Status
	}

	if params.AssignedNodeID != nil {
		j.AssignedNodeID = *params.AssignedNodeID
	}
	if params.StartedAt != nil {
		j.StartedAt = params.StartedAt
	}
	if params.CompletedAt != nil {
		j.CompletedAt = params.CompletedAt
	}
	if params.Result != nil {
		j.Result = params.Result
	}

	if oldStatus != j.Status && j.Status == StatusAssigned {
		r.appendEvent(j.ID, EventAssigned, oldStatus, j.Status, j.AssignedNodeID, "Job assigned by scheduler")
	}

	jobCopy := *j
	return &jobCopy, nil
}

func (r *Registry) Claim(id, nodeID string) (*Job, error) {
	r.mu.Lock()

	j, ok := r.jobs[id]
	if !ok {
		r.mu.Unlock()
		return nil, ErrJobNotFound
	}

	if j.Status != StatusAssigned {
		r.mu.Unlock()
		return nil, ErrInvalidTransition
	}
	if j.AssignedNodeID != nodeID {
		r.mu.Unlock()
		return nil, errors.New("job assigned to another node")
	}

	now := time.Now().UTC()
	j.Status = StatusRunning
	j.StartedAt = &now

	r.appendEvent(j.ID, EventClaimed, StatusAssigned, StatusRunning, nodeID, "Job claimed by agent")

	jobCopy := *j
	r.mu.Unlock()

	slog.Info("job claimed", "job_id", id, "node_id", nodeID)
	return &jobCopy, nil
}

func (r *Registry) ReportResult(id, nodeID string, res JobResult) (*Job, error) {
	r.mu.Lock()

	j, ok := r.jobs[id]
	if !ok {
		r.mu.Unlock()
		return nil, ErrJobNotFound
	}

	// Idempotency: if already succeeded or failed by this node, accept it without modifying again.
	if j.Status == StatusSucceeded || j.Status == StatusFailed {
		if j.AssignedNodeID != nodeID {
			r.mu.Unlock()
			return nil, errors.New("job assigned to another node")
		}
		jobCopy := *j
		r.mu.Unlock()
		return &jobCopy, nil
	}

	if j.Status != StatusRunning {
		r.mu.Unlock()
		return nil, ErrInvalidTransition
	}
	if j.AssignedNodeID != nodeID {
		r.mu.Unlock()
		return nil, errors.New("job assigned to another node")
	}

	now := time.Now().UTC()
	var evtType JobEventType
	var reason string
	if res.ExitCode == 0 {
		j.Status = StatusSucceeded
		evtType = EventSucceeded
		reason = "Job completed successfully"
	} else {
		j.Status = StatusFailed
		evtType = EventFailed
		reason = fmt.Sprintf("Job failed with exit code %d", res.ExitCode)
	}
	j.CompletedAt = &now
	j.Result = &res

	r.appendEvent(j.ID, evtType, StatusRunning, j.Status, nodeID, reason)

	jobCopy := *j
	r.mu.Unlock()

	slog.Info("job result reported", "job_id", id, "status", j.Status, "exit_code", res.ExitCode)
	return &jobCopy, nil
}
