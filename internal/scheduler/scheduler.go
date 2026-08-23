package scheduler

import (
	"log/slog"
	"sort"

	"github.com/Tushardevx01/runstack/internal/job"
	"github.com/Tushardevx01/runstack/internal/node"
	"time"
)

type Scheduler struct {
	nodeRegistry     *node.Registry
	jobRegistry      *job.Registry
	ExecutionTimeout time.Duration
	NodeGracePeriod  time.Duration
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

	selectedNode := onlineNodes[0]

	// Step 4: Find PENDING jobs
	jobs := s.jobRegistry.List()

	// Step 5: Assign jobs
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
