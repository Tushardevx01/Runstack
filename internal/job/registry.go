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

func (r *Registry) Create(name, command string, maxRetries int) *Job {
	r.mu.Lock()
	defer r.mu.Unlock()

	j := &Job{
		ID:         "job-" + GenerateID(),
		Name:       name,
		Command:    command,
		Status:     StatusPending,
		CreatedAt:  time.Now().UTC(),
		MaxRetries: maxRetries,
	}

	r.jobs[j.ID] = j
	r.appendEvent(j.ID, EventCreated, "", StatusPending, "", "", 0, "Job created")

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

func (r *Registry) appendEvent(jobID string, eventType JobEventType, from, to Status, nodeID string, executionID string, attempts int, reason string) {
	evt := JobEvent{
		ID:          fmt.Sprintf("%s-evt-%d", jobID, len(r.events[jobID])+1),
		JobID:       jobID,
		Timestamp:   time.Now().UTC(),
		Type:        eventType,
		From:        from,
		To:          to,
		NodeID:      nodeID,
		ExecutionID: executionID,
		Attempts:    attempts,
		Reason:      reason,
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
		if j.Status == StatusRunning && *params.Status == StatusPending {
			return nil, errors.New("cannot manually transition RUNNING to PENDING via Update")
		}
		if j.Status == StatusAssigned && *params.Status == StatusPending {
			return nil, errors.New("cannot manually transition ASSIGNED to PENDING via Update")
		}
		// Block PENDING→FAILED and ASSIGNED→FAILED via Update.
		// These transitions are only valid through internal recovery methods.
		if *params.Status == StatusFailed && (j.Status == StatusPending || j.Status == StatusAssigned) {
			return nil, errors.New("cannot manually transition to FAILED via Update")
		}
		j.Status = *params.Status
	}

	if oldStatus == StatusRunning || oldStatus == StatusSucceeded || oldStatus == StatusFailed {
		if params.AssignedNodeID != nil || params.StartedAt != nil || params.Result != nil {
			return nil, errors.New("cannot modify execution fields on running/terminal job")
		}
	}

	// Block setting execution fields on PENDING jobs when not transitioning to ASSIGNED.
	// A PENDING job with a non-empty AssignedNodeID or StartedAt violates state invariants.
	if oldStatus == StatusPending && j.Status == StatusPending {
		if params.AssignedNodeID != nil || params.StartedAt != nil || params.Result != nil {
			return nil, errors.New("cannot set execution fields on PENDING job without status transition")
		}
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
		r.appendEvent(j.ID, EventAssigned, oldStatus, j.Status, j.AssignedNodeID, "", j.Attempts, "Job assigned by scheduler")
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

	j.Attempts++
	now := time.Now().UTC()
	j.Status = StatusRunning
	j.StartedAt = &now
	j.ExecutionID = "exec-" + GenerateID()

	r.appendEvent(j.ID, EventClaimed, StatusAssigned, StatusRunning, nodeID, j.ExecutionID, j.Attempts, "Job claimed by agent")

	jobCopy := *j
	r.mu.Unlock()

	slog.Info("job claimed", "job_id", id, "node_id", nodeID, "execution_id", jobCopy.ExecutionID)
	return &jobCopy, nil
}

func (r *Registry) ReportResult(id, nodeID string, execID string, res JobResult) (*Job, error) {
	r.mu.Lock()

	j, ok := r.jobs[id]
	if !ok {
		r.mu.Unlock()
		return nil, ErrJobNotFound
	}

	// Idempotency: if already succeeded or failed by this node, accept it without modifying again.
	if j.Status == StatusSucceeded || j.Status == StatusFailed {
		if j.AssignedNodeID != nodeID || j.ExecutionID != execID {
			r.mu.Unlock()
			return nil, errors.New("stale execution result")
		}
		if j.Result != nil && j.Result.ExitCode != res.ExitCode {
			r.mu.Unlock()
			return nil, errors.New("contradictory execution result")
		}
		jobCopy := *j
		r.mu.Unlock()
		return &jobCopy, nil
	}

	if j.Status != StatusRunning {
		r.mu.Unlock()
		return nil, ErrInvalidTransition
	}
	if j.AssignedNodeID != nodeID || j.ExecutionID != execID {
		r.mu.Unlock()
		return nil, errors.New("stale execution result")
	}

	now := time.Now().UTC()
	var evtType JobEventType
	var reason string
	if res.ExitCode == 0 {
		j.Status = StatusSucceeded
		evtType = EventSucceeded
		reason = "Job completed successfully"
		j.CompletedAt = &now
		j.Result = &res
		r.appendEvent(j.ID, evtType, StatusRunning, j.Status, nodeID, execID, j.Attempts, reason)
	} else {
		if j.Attempts <= j.MaxRetries {
			j.Status = StatusPending
			evtType = EventRetried
			reason = fmt.Sprintf("Job failed with exit code %d", res.ExitCode)
			j.AssignedNodeID = ""
			j.StartedAt = nil
			j.ExecutionID = ""
			r.appendEvent(j.ID, evtType, StatusRunning, StatusPending, nodeID, execID, j.Attempts, reason)
		} else {
			j.Status = StatusFailed
			evtType = EventFailed
			reason = fmt.Sprintf("Job failed with exit code %d (Retries exhausted)", res.ExitCode)
			j.CompletedAt = &now
			j.Result = &res
			r.appendEvent(j.ID, evtType, StatusRunning, j.Status, nodeID, execID, j.Attempts, reason)
		}
	}

	jobCopy := *j
	r.mu.Unlock()

	slog.Info("job result reported", "job_id", id, "node_id", nodeID, "execution_id", execID, "status", jobCopy.Status, "exit_code", res.ExitCode)
	return &jobCopy, nil
}

// RecoverExecutionTimeouts finds RUNNING jobs that have exceeded the timeout
// and safely recovers them back to PENDING. Returns the number of jobs recovered.
func (r *Registry) RecoverExecutionTimeouts(timeout time.Duration) int {
	r.mu.Lock()

	recoveredCount := 0
	now := time.Now().UTC()

	for _, j := range r.jobs {
		if j.Status == StatusRunning && j.StartedAt != nil {
			if now.Sub(*j.StartedAt) >= timeout {
				oldNodeID := j.AssignedNodeID
				oldExecutionID := j.ExecutionID

				if j.Attempts <= j.MaxRetries {
					j.Status = StatusPending
					j.AssignedNodeID = ""
					j.StartedAt = nil
					j.ExecutionID = ""

					r.appendEvent(
						j.ID,
						EventRecovered,
						StatusRunning,
						StatusPending,
						oldNodeID,
						oldExecutionID,
						j.Attempts,
						"Execution timeout exceeded",
					)
				} else {
					j.Status = StatusFailed
					j.CompletedAt = &now
					j.Result = &JobResult{ExitCode: -1, Error: "Execution timeout exceeded"}

					r.appendEvent(
						j.ID,
						EventFailed,
						StatusRunning,
						StatusFailed,
						oldNodeID,
						oldExecutionID,
						j.Attempts,
						"Execution timeout exceeded (Retries exhausted)",
					)
				}

				recoveredCount++
			}
		}
	}
	r.mu.Unlock()

	if recoveredCount > 0 {
		slog.Info("recovered execution timeouts", "count", recoveredCount)
	}

	return recoveredCount
}

// RecoverNodeJobs finds RUNNING or ASSIGNED jobs for a specific node
// and safely recovers them back to PENDING. Returns the number of jobs recovered.
func (r *Registry) RecoverNodeJobs(nodeID string, reason string) int {
	r.mu.Lock()

	recoveredCount := 0

	for _, j := range r.jobs {
		if (j.Status == StatusRunning || j.Status == StatusAssigned) && j.AssignedNodeID == nodeID {
			oldStatus := j.Status
			oldExecutionID := j.ExecutionID

			if j.Attempts <= j.MaxRetries {
				j.Status = StatusPending
				j.AssignedNodeID = ""
				j.StartedAt = nil
				j.ExecutionID = ""

				r.appendEvent(
					j.ID,
					EventRecovered,
					oldStatus,
					StatusPending,
					nodeID,
					oldExecutionID,
					j.Attempts,
					reason,
				)
			} else {
				j.Status = StatusFailed
				now := time.Now().UTC()
				j.CompletedAt = &now
				j.Result = &JobResult{ExitCode: -1, Error: reason}

				r.appendEvent(
					j.ID,
					EventFailed,
					oldStatus,
					StatusFailed,
					nodeID,
					oldExecutionID,
					j.Attempts,
					reason+" (Retries exhausted)",
				)
			}

			recoveredCount++
		}
	}
	r.mu.Unlock()

	if recoveredCount > 0 {
		slog.Info("recovered node jobs", "node_id", nodeID, "count", recoveredCount, "reason", reason)
	}

	return recoveredCount
}
