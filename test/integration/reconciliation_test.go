package integration

import (
	"github.com/Tushardevx01/runstack/internal/job"
	"testing"
	"time"

	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/deployment"
	"github.com/Tushardevx01/runstack/internal/instance"
	"github.com/Tushardevx01/runstack/internal/node"
	"github.com/Tushardevx01/runstack/internal/scheduler"
	"github.com/Tushardevx01/runstack/internal/service"
)

func TestEndToEndReconciliation(t *testing.T) {
	appReg := application.NewRegistry()
	depReg := deployment.NewRegistry()
	instReg := instance.NewRegistry()
	nodeReg := node.NewRegistry()

	appService := service.NewAppService(appReg, depReg, application.NewSecretRegistry())
	reconciler := scheduler.NewInstanceReconciler(appReg, depReg, instReg, nodeReg)
	instScheduler := scheduler.NewInstanceScheduler(nodeReg, instReg, scheduler.NewCapacityCalculator(appReg, depReg, instReg, &job.Registry{}))

	// Register 2 nodes
	nodeReg.Register(node.Node{ID: "node-1", Status: node.StatusOnline})
	nodeReg.Register(node.Node{ID: "node-2", Status: node.StatusOnline})

	// 1. Create Application with replicas=2
	spec1 := application.AppSpec{Replicas: 2}
	app, err := appService.CreateApp("test-app", spec1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 2. Verify no Instances are synchronously created
	if len(instReg.List()) != 0 {
		t.Fatalf("expected 0 instances, got %d", len(instReg.List()))
	}

	// 3. Run reconciliation
	scheduler.ConfigInstanceUnknownTimeout = 0
	reconciler.Reconcile()

	// Scheduler assigns instances
	instScheduler.SchedulePendingInstances()

	// 4. Verify exactly 2 PENDING/ASSIGNED Instances exist
	insts := instReg.List()
	if len(insts) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(insts))
	}
	for _, inst := range insts {
		if inst.Status != instance.StatusAssigned {
			t.Fatalf("expected ASSIGNED, got %s", inst.Status)
		}
	}

	// 5. Run reconciliation again
	scheduler.ConfigInstanceUnknownTimeout = 0
	reconciler.Reconcile()

	// 6. Verify still exactly 2, no duplicates
	if len(instReg.List()) != 2 {
		t.Fatalf("expected still 2 instances, got %d", len(instReg.List()))
	}

	// 7. Mark one assigned Node offline
	targetInst := instReg.List()[0]
	var healthyNodeID string
	if targetInst.NodeID == "node-1" {
		healthyNodeID = "node-2"
	} else {
		healthyNodeID = "node-1"
	}

	nodeReg.MarkOfflineNodes(-1 * time.Second) // mark all offline
	nodeReg.Heartbeat(healthyNodeID, nil)      // bring one back online

	// 8. Reconcile
	scheduler.ConfigInstanceUnknownTimeout = 0
	reconciler.Reconcile()
	reconciler.Reconcile()

	// 9. Verify the orphaned Instance becomes CRASHED
	crashedInst, _ := instReg.Get(targetInst.ID)
	if crashedInst.Status != instance.StatusCrashed {
		t.Fatalf("expected orphaned instance to be CRASHED, got %s", crashedInst.Status)
	}

	// 10. Reconcile again (the first reconcile marked it crashed, which lowered the viable count, so it also created a new one in the same pass)
	// Actually, wait, recoverOrphanedInstances runs first in Reconcile(), so it marks it CRASHED, then counts viable, sees 1 < 2, and creates a replacement all in one tick!
	// So we should just verify the replacement was created.

	instScheduler.SchedulePendingInstances()

	// 11. Verify desired capacity is restored without duplicate explosion
	insts = instReg.List()
	if len(insts) != 3 {
		t.Fatalf("expected 3 total instances (1 crashed, 2 assigned), got %d", len(insts))
	}

	viable := 0
	for _, inst := range insts {
		if inst.Status == instance.StatusAssigned {
			viable++
		}
	}
	if viable != 2 {
		t.Fatalf("expected 2 viable assigned instances, got %d", viable)
	}

	// 12. Update Application to a new spec
	spec2 := application.AppSpec{Replicas: 2, Command: []string{"echo", "v2"}}
	oldDepID := app.ActiveDeploymentID

	app, err = appService.UpdateApp(app.ID, spec2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 13. Verify a new Deployment exists
	if app.ActiveDeploymentID == oldDepID {
		t.Fatalf("expected new ActiveDeploymentID")
	}

	// 14. Verify old Deployment snapshot is unchanged
	oldDep, _ := depReg.Get(oldDepID)
	if len(oldDep.SpecSnapshot.Command) != 0 {
		t.Fatalf("expected old deployment snapshot to be unchanged")
	}
	if oldDep.Status != deployment.StatusSuperseded {
		t.Fatalf("expected old deployment to be superseded")
	}

	// 15. Verify reconciliation targets the new active Deployment
	scheduler.ConfigInstanceUnknownTimeout = 0
	reconciler.Reconcile()

	insts = instReg.List()
	// Should have created 2 new instances, stopped the 2 viable old ones.
	// Total instances = 1 crashed (v1) + 2 stopped (v1) + 2 pending (v2) = 5
	if len(insts) != 5 {
		t.Fatalf("expected 5 total instances, got %d", len(insts))
	}

	v2Count := 0
	for _, inst := range insts {
		if inst.DeploymentID == app.ActiveDeploymentID && inst.Status == instance.StatusPending {
			v2Count++
		}
	}
	if v2Count != 2 {
		t.Fatalf("expected 2 new pending instances for v2, got %d", v2Count)
	}
}
