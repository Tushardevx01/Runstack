package scheduler

import (
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/Tushardevx01/runstack/internal/job"
	"github.com/Tushardevx01/runstack/internal/node"
)

type Scheduler struct {
	nodeRegistry       *node.Registry
	jobRegistry        *job.Registry
	capacityCalculator *CapacityCalculator
	ExecutionTimeout   time.Duration
	NodeGracePeriod    time.Duration
	mu                 sync.Mutex
	lastAssignedNodeID string
}

func New(nodeRegistry *node.Registry, jobRegistry *job.Registry, capCalc *CapacityCalculator) *Scheduler {
	return &Scheduler{
		nodeRegistry:       nodeRegistry,
		jobRegistry:        jobRegistry,
		capacityCalculator: capCalc,
		ExecutionTimeout:   2 * time.Hour,
		NodeGracePeriod:    30 * time.Second,
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

	caps := s.capacityCalculator.CalculateAll(onlineNodes)

	// Step 4: Find PENDING jobs
	jobs := s.jobRegistry.List()
	var pendingJobs []job.Job
	for _, j := range jobs {
		if j.Status == job.StatusPending {
			pendingJobs = append(pendingJobs, j)
		}
	}

	sort.Slice(pendingJobs, func(i, j int) bool {
		return pendingJobs[i].CreatedAt.Before(pendingJobs[j].CreatedAt)
	})

	for _, j := range pendingJobs {
		reqCPU := j.CPU
		reqMem := j.MemoryMB

		var eligibleNodes []node.Node
		for _, n := range onlineNodes {
			c := caps[n.ID]
			if c.AvailableCPU >= reqCPU && c.AvailableMemoryMB >= reqMem {
				eligibleNodes = append(eligibleNodes, n)
			}
		}

		if len(eligibleNodes) == 0 {
			slog.Warn("Insufficient capacity for job", "job_id", j.ID, "cpu_requested", reqCPU, "memory_requested", reqMem)
			continue
		}

		sort.Slice(eligibleNodes, func(i, j int) bool {
			return eligibleNodes[i].ID < eligibleNodes[j].ID
		})

		startIndex := 0
		if s.lastAssignedNodeID != "" {
			for i, n := range eligibleNodes {
				if n.ID > s.lastAssignedNodeID {
					startIndex = i
					break
				}
			}
		}

		selectedNode := eligibleNodes[startIndex]
		status := job.StatusAssigned

		_, err := s.jobRegistry.Update(j.ID, job.UpdateParams{
			Status:         &status,
			AssignedNodeID: &selectedNode.ID,
		})

		if err == nil {
			slog.Info("Job scheduled", "job_id", j.ID, "node_id", selectedNode.ID)
			s.lastAssignedNodeID = selectedNode.ID

			cap := caps[selectedNode.ID]
			cap.AvailableCPU -= reqCPU
			cap.AvailableMemoryMB -= reqMem
			caps[selectedNode.ID] = cap
		}
	}

	return nil
}
