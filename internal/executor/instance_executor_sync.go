package executor

import "github.com/Tushardevx01/runstack/internal/instance"

func (e *InstanceExecutor) syncPorts() {
	instances, err := e.APIClient.ListInstances(e.NodeID, "")
	if err != nil {
		return
	}

	allocated := make(map[int]string)
	for _, inst := range instances {
		if inst.Status == instance.StatusStarting || inst.Status == instance.StatusRunning || inst.Status == instance.StatusStopping {
			for _, p := range inst.Ports {
				allocated[p.External] = inst.ID
			}
		}
	}
	e.Ports.Sync(allocated)
}
