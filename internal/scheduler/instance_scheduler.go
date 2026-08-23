package scheduler

import (
	"log/slog"
	"sort"
	"sync"

	"github.com/Tushardevx01/runstack/internal/instance"
	"github.com/Tushardevx01/runstack/internal/node"
)

type InstanceScheduler struct {
	nodeRegistry       *node.Registry
	instanceRegistry   *instance.Registry
	mu                 sync.Mutex
	lastAssignedNodeID string
}

func NewInstanceScheduler(nodeRegistry *node.Registry, instanceRegistry *instance.Registry) *InstanceScheduler {
	return &InstanceScheduler{
		nodeRegistry:     nodeRegistry,
		instanceRegistry: instanceRegistry,
	}
}

func (s *InstanceScheduler) SchedulePendingInstances() error {
	s.mu.Lock()
	defer s.mu.Unlock()

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

	startIndex := 0
	if s.lastAssignedNodeID != "" {
		for i, n := range onlineNodes {
			if n.ID > s.lastAssignedNodeID {
				startIndex = i
				break
			}
		}
	}

	instances := s.instanceRegistry.List()
	var pendingInstances []instance.Instance
	for _, inst := range instances {
		if inst.Status == instance.StatusPending {
			pendingInstances = append(pendingInstances, inst)
		}
	}

	sort.Slice(pendingInstances, func(i, j int) bool {
		return pendingInstances[i].ID < pendingInstances[j].ID
	})

	currentIndex := startIndex
	for _, inst := range pendingInstances {
		nodeID := onlineNodes[currentIndex].ID

		_, err := s.instanceRegistry.UpdateState(inst.ID, instance.StatusAssigned, nodeID, "")
		if err == nil {
			slog.Info("instance assigned", "instance_id", inst.ID, "node_id", nodeID)
			s.lastAssignedNodeID = nodeID
			currentIndex = (currentIndex + 1) % len(onlineNodes)
		}
	}

	return nil
}
