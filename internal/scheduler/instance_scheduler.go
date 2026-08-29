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
	capacityCalculator *CapacityCalculator
	mu                 sync.Mutex
	lastAssignedNodeID string
}

func NewInstanceScheduler(nodeRegistry *node.Registry, instanceRegistry *instance.Registry, capCalc *CapacityCalculator) *InstanceScheduler {
	return &InstanceScheduler{
		nodeRegistry:       nodeRegistry,
		instanceRegistry:   instanceRegistry,
		capacityCalculator: capCalc,
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

	caps := s.capacityCalculator.CalculateAll(onlineNodes)

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

	for _, inst := range pendingInstances {
		dep, err := s.capacityCalculator.DeploymentRegistry.Get(inst.DeploymentID)
		if err != nil {
			continue
		}

		reqCPU := 0.0
		reqMem := 0
		if dep.SpecSnapshot.Resources != nil {
			reqCPU = dep.SpecSnapshot.Resources.CPU
			reqMem = dep.SpecSnapshot.Resources.MemoryMB
		}

		var eligibleNodes []node.Node
		for _, n := range onlineNodes {
			c := caps[n.ID]
			if c.AvailableCPU >= reqCPU && c.AvailableMemoryMB >= reqMem {
				eligibleNodes = append(eligibleNodes, n)
			}
		}

		if len(eligibleNodes) == 0 {
			slog.Warn("Insufficient capacity for instance", "instance_id", inst.ID, "cpu_requested", reqCPU, "memory_requested", reqMem)
			s.capacityCalculator.DeploymentRegistry.UpdateRolloutStatusOnly(inst.DeploymentID, dep.RolloutStatus, "insufficient_capacity")
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

		_, err = s.instanceRegistry.UpdateState(inst.ID, instance.StatusAssigned, selectedNode.ID, "")
		if err == nil {
			s.capacityCalculator.DeploymentRegistry.UpdateRolloutStatusOnly(inst.DeploymentID, dep.RolloutStatus, "")
			slog.Info("Instance scheduled", "instance_id", inst.ID, "node_id", selectedNode.ID)
			s.lastAssignedNodeID = selectedNode.ID

			// Deduct from local caps cache
			cap := caps[selectedNode.ID]
			cap.AvailableCPU -= reqCPU
			cap.AvailableMemoryMB -= reqMem
			caps[selectedNode.ID] = cap
		}
	}

	return nil
}
