package scheduler_test

import (
	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/deployment"
	"github.com/Tushardevx01/runstack/internal/instance"
	"github.com/Tushardevx01/runstack/internal/job"
	"github.com/Tushardevx01/runstack/internal/node"
	"github.com/Tushardevx01/runstack/internal/scheduler"
	"testing"
)

func TestCapacityCalculator(t *testing.T) {
	appReg := application.NewRegistry()
	depReg := deployment.NewRegistry()
	instReg := instance.NewRegistry()
	jobReg := job.Registry{}

	capCalc := scheduler.NewCapacityCalculator(appReg, depReg, instReg, &jobReg)

	nodes := []node.Node{
		{
			ID:       "node1",
			CPUCores: 4,
			Capabilities: node.Capabilities{
				TotalMemoryBytes: 4 * 1024 * 1024,
			},
		},
	}

	app, _ := appReg.Create("app1", application.AppSpec{})
	dep, _ := depReg.Create(app.ID, application.AppSpec{
		Resources: &application.ResourceRequirements{
			CPU:      1.0,
			MemoryMB: 1,
		},
	})

	inst, _ := instReg.Create(app.ID, dep.ID)
	instReg.UpdateState(inst.ID, instance.StatusRunning, "node1", "")

	caps := capCalc.CalculateAll(nodes)

	if caps["node1"].AvailableCPU != 3.0 {
		t.Errorf("Expected 3.0 CPU available, got %f", caps["node1"].AvailableCPU)
	}

	if caps["node1"].AvailableMemoryMB != 3 {
		t.Errorf("Expected 3 MB memory available, got %d", caps["node1"].AvailableMemoryMB)
	}

	// UNKNOWN should consume capacity
	instReg.UpdateState(inst.ID, instance.StatusUnknown, "node1", "")
	caps = capCalc.CalculateAll(nodes)
	if caps["node1"].AvailableCPU != 3.0 {
		t.Errorf("Expected 3.0 CPU available for UNKNOWN, got %f", caps["node1"].AvailableCPU)
	}

	// STOPPED should release capacity
	instReg.UpdateState(inst.ID, instance.StatusStopped, "node1", "")
	caps = capCalc.CalculateAll(nodes)
	if caps["node1"].AvailableCPU != 4.0 {
		t.Errorf("Expected 4.0 CPU available for STOPPED, got %f", caps["node1"].AvailableCPU)
	}
}
