package scheduler

import (
	"log/slog"
	"sort"

	"github.com/Tushardevx01/runstack/internal/job"
	"github.com/Tushardevx01/runstack/internal/node"
	"time"
)

type Scheduler struct {
	nodeRegistry   *node.Registry
	jobRegistry    *job.Registry
	StaleThreshold time.Duration
}

func New(nodeRegistry *node.Registry, jobRegistry *job.Registry) *Scheduler {
	return &Scheduler{
		nodeRegistry:   nodeRegistry,
		jobRegistry:    jobRegistry,
		StaleThreshold: 30 * time.Second, // default configuration
	}
}

func (s *Scheduler) SchedulePendingJobs() error {
	// Step 1: Detect and recover stale RUNNING jobs back to PENDING.
	if s.StaleThreshold > 0 {
		s.jobRegistry.RecoverStaleJobs(s.StaleThreshold)
	}

	// Step 2: Find eligible ONLINE nodes.
	nodes := s.nodeRegistry.List()

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

	selectedNode := onlineNodes[0]

	jobs := s.jobRegistry.List()

	for _, j := range jobs {
		if j.Status == job.StatusPending {
			status := job.StatusAssigned
			nodeID := selectedNode.ID

			_, err := s.jobRegistry.Update(j.ID, job.UpdateParams{
				Status:         &status,
				AssignedNodeID: &nodeID,
			})

			if err == nil {
				slog.Info("job assigned", "job_id", j.ID, "node_id", nodeID)
			}
		}
	}

	return nil
}
