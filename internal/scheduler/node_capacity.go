package scheduler

import (
	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/deployment"
	"github.com/Tushardevx01/runstack/internal/instance"
	"github.com/Tushardevx01/runstack/internal/job"
	"github.com/Tushardevx01/runstack/internal/node"
)

type CapacityCalculator struct {
	AppRegistry        *application.Registry
	DeploymentRegistry *deployment.Registry
	InstanceRegistry   *instance.Registry
	JobRegistry        *job.Registry
}

func NewCapacityCalculator(appReg *application.Registry, depReg *deployment.Registry, instReg *instance.Registry, jobReg *job.Registry) *CapacityCalculator {
	return &CapacityCalculator{
		AppRegistry:        appReg,
		DeploymentRegistry: depReg,
		InstanceRegistry:   instReg,
		JobRegistry:        jobReg,
	}
}

type NodeCapacity struct {
	AvailableCPU      float64
	AvailableMemoryMB int
}

func (c *CapacityCalculator) CalculateAll(nodes []node.Node) map[string]NodeCapacity {
	caps := make(map[string]NodeCapacity)
	for _, n := range nodes {
		caps[n.ID] = NodeCapacity{
			AvailableCPU:      float64(n.CPUCores),
			AvailableMemoryMB: int(n.Capabilities.TotalMemoryBytes / 1024 / 1024),
		}
	}

	instances := c.InstanceRegistry.List()
	// Get all deployments to find resource reqs
	deps := c.DeploymentRegistry.List()
	depMap := make(map[string]deployment.Deployment)
	for _, d := range deps {
		depMap[d.ID] = d
	}

	for _, inst := range instances {
		if !c.isInstanceConsumingCapacity(inst.Status) {
			continue
		}

		dep, exists := depMap[inst.DeploymentID]
		if !exists {
			continue
		}

		if dep.SpecSnapshot.Resources != nil {
			cap := caps[inst.NodeID]
			cap.AvailableCPU -= dep.SpecSnapshot.Resources.CPU
			cap.AvailableMemoryMB -= dep.SpecSnapshot.Resources.MemoryMB
			caps[inst.NodeID] = cap
		}
	}

	jobs := c.JobRegistry.List()
	for _, j := range jobs {
		if !c.isJobConsumingCapacity(j.Status) {
			continue
		}
		if j.CPU > 0 || j.MemoryMB > 0 {
			cap := caps[j.AssignedNodeID]
			cap.AvailableCPU -= j.CPU
			cap.AvailableMemoryMB -= j.MemoryMB
			caps[j.AssignedNodeID] = cap
		}
	}

	return caps
}

func (c *CapacityCalculator) isInstanceConsumingCapacity(status instance.InstanceStatus) bool {
	switch status {
	case instance.StatusAssigned, instance.StatusStarting, instance.StatusRunning, instance.StatusUnknown, instance.StatusStopping:
		return true
	default:
		// PENDING, STOPPED, CRASHED do not consume capacity
		return false
	}
}

func (c *CapacityCalculator) isJobConsumingCapacity(status job.Status) bool {
	switch status {
	case job.StatusAssigned, job.StatusRunning:
		return true
	default:
		return false
	}
}
