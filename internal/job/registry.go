package job

import (
	"errors"
	"log/slog"
	"sync"
	"time"
)

type Registry struct {
	mu   sync.RWMutex
	jobs map[string]*Job
}

func NewRegistry() *Registry {
	return &Registry{
		jobs: make(map[string]*Job),
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
	if res.ExitCode == 0 {
		j.Status = StatusSucceeded
	} else {
		j.Status = StatusFailed
	}
	j.CompletedAt = &now
	j.Result = &res

	jobCopy := *j
	r.mu.Unlock()

	slog.Info("job result reported", "job_id", id, "status", j.Status, "exit_code", res.ExitCode)
	return &jobCopy, nil
}
