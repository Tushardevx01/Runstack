package scheduler

import (
	"log/slog"
	"sort"
	"sync"

	"github.com/Tushardevx01/runstack/internal/job"
	"github.com/Tushardevx01/runstack/internal/node"
	"time"
)

type Scheduler struct {
	nodeRegistry       *node.Registry
	jobRegistry        *job.Registry
	ExecutionTimeout   time.Duration
	NodeGracePeriod    time.Duration
	mu                 sync.Mutex
	lastAssignedNodeID string
}

func New(nodeRegistry *node.Registry, jobRegistry *job.Registry) *Scheduler {
	return &Scheduler{
		nodeRegistry:     nodeRegistry,
		jobRegistry:      jobRegistry,
		ExecutionTimeout: 2 * time.Hour,
		NodeGracePeriod:  30 * time.Second,
	}
}

func (s *Scheduler) SchedulePendingJobs() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Step 1: Recover jobs exceeding ExecutionTimeout
	if s.ExecutionTimeout > 0 {
		s.jobRegistry.RecoverExecutionTimeouts(s.ExecutionTimeout)
	}

	// Step 2: Read NodeRegistry and recover OFFLINE nodes beyond grace period
	nodes := s.nodeRegistry.List()
	for _, n := range nodes {
		if n.Status == node.StatusOffline && n.OfflineSince != nil {
			if time.Since(*n.OfflineSince) >= s.NodeGracePeriod {
				s.jobRegistry.RecoverNodeJobs(n.ID, "Node offline grace period exceeded")
			}
		}
	}

	// Step 3: Find ONLINE nodes
	var onlineNodes []node.Node
	for _, n := range nodes {
		if n.Status == node.StatusOnline {
			onlineNodes = append(onlineNodes, n)
		}
	}

	if len(onlineNodes) == 0 {
		return nil
	}

	sort.Slice(onlineNodes, func(i, j int) bool {
		return onlineNodes[i].ID < onlineNodes[j].ID
	})

	startIndex := 0
	if s.lastAssignedNodeID != "" {
		for i, n := range onlineNodes {
			if n.ID > s.lastAssignedNodeID {
				startIndex = i
				break
			}
		}
	}

	// Step 4: Find PENDING jobs
	jobs := s.jobRegistry.List()
	var pendingJobs []job.Job
	for _, j := range jobs {
		if j.Status == job.StatusPending {
			pendingJobs = append(pendingJobs, j)
		}
	}
	// Sort jobs deterministically
	sort.Slice(pendingJobs, func(i, j int) bool {
		return pendingJobs[i].ID < pendingJobs[j].ID
	})

	// Step 5: Assign jobs
	currentIndex := startIndex
	for _, j := range pendingJobs {
		status := job.StatusAssigned
		nodeID := onlineNodes[currentIndex].ID

		_, err := s.jobRegistry.Update(j.ID, job.UpdateParams{
			Status:         &status,
			AssignedNodeID: &nodeID,
		})

		if err == nil {
			slog.Info("job assigned", "job_id", j.ID, "node_id", nodeID)
			s.lastAssignedNodeID = nodeID
			currentIndex = (currentIndex + 1) % len(onlineNodes)
		}
	}

	return nil
}
