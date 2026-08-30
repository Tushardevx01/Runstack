package scheduler

import (
	"testing"
	"time"

	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/deployment"
	"github.com/Tushardevx01/runstack/internal/instance"
	"github.com/Tushardevx01/runstack/internal/node"
)

func setupReconciler() (*InstanceReconciler, *application.Registry, *deployment.Registry, *instance.Registry, *node.Registry, instance.Instance) {
	appReg := application.NewRegistry()
	depReg := deployment.NewRegistry()
	instReg := instance.NewRegistry()
	nodeReg := node.NewRegistry()

	r := NewInstanceReconciler(appReg, depReg, instReg, nodeReg)
	r.DrainTimeout = 0

	spec := application.AppSpec{Replicas: 1}
	app, _ := appReg.Create("app1", spec)
	dep, _ := depReg.Create(app.ID, spec)
	appReg.Update(app.ID, spec, dep.ID, application.StatusPending)

	inst, _ := instReg.Create(app.ID, dep.ID)
	instReg.UpdateState(inst.ID, instance.StatusRunning, "node-1", "cid")

	nodeReg.Register(node.Node{ID: "node-1"}, "")
	nodeReg.Heartbeat("node-1", nil)

	return r, appReg, depReg, instReg, nodeReg, inst
}

func TestReconciler_HealthUnhealthy(t *testing.T) {
	r, _, depReg, instReg, _, inst := setupReconciler()

	// Report unhealthy
	instReg.ReportStatus(inst.ID, "node-1", inst.ExecutionID, instance.StatusRunning, instance.HealthUnhealthy, "cid", nil)

	r.Reconcile()

	updatedInst, _ := instReg.Get(inst.ID)
	if updatedInst.Status != instance.StatusCrashed {
		t.Fatalf("expected UNHEALTHY to become CRASHED, got %v", updatedInst.Status)
	}

	dep, _ := depReg.Get(inst.DeploymentID)
	if dep.ConsecutiveCrashes != 1 {
		t.Fatalf("expected crash counter 1, got %d", dep.ConsecutiveCrashes)
	}
}

func TestReconciler_HealthyRecoveryReset(t *testing.T) {
	r, _, depReg, instReg, _, inst := setupReconciler()

	// Add a crash to deployment manually
	depReg.RecordCrash(inst.DeploymentID, 5)

	// Make it healthy
	instReg.ReportStatus(inst.ID, "node-1", inst.ExecutionID, instance.StatusRunning, instance.HealthHealthy, "cid", nil)

	// Make it look like it started a long time ago
	oldTime := time.Now().UTC().Add(-2 * time.Minute)
	instCopy, _ := instReg.Get(inst.ID)
	instCopy.StartedAt = &oldTime
	// Need to use internal map for this test hack, or just modify test
	// Actually we can't easily modify StartedAt because it's set by ReportStatus.
	// But we can override ConfigHealthyRecoveryWindow
	ConfigHealthyRecoveryWindow = 0

	r.Reconcile()

	dep, _ := depReg.Get(inst.DeploymentID)
	if dep.ConsecutiveCrashes != 0 {
		t.Fatalf("expected crash counter reset to 0, got %d", dep.ConsecutiveCrashes)
	}
}

func TestReconciler_CrashLoopBreaker(t *testing.T) {
	r, _, depReg, instReg, _, inst := setupReconciler()

	// Crash 5 times
	for i := 0; i < deployment.MaxCrashLoopThreshold; i++ {
		instReg.ReportStatus(inst.ID, "node-1", inst.ExecutionID, instance.StatusRunning, instance.HealthUnhealthy, "cid", nil)
		r.Reconcile()

		insts := instReg.List()
		for _, idxInst := range insts {
			if idxInst.Status == instance.StatusPending {
				instReg.UpdateState(idxInst.ID, instance.StatusRunning, "node-1", "cid-next")
				inst = idxInst
			}
		}
	}

	dep, _ := depReg.Get(inst.DeploymentID)
	if !dep.Degraded {
		t.Fatalf("expected deployment to be degraded")
	}

	// It should pause replacement. Desired = 1, viable = 0.
	insts := instReg.List()
	viable := 0
	for _, i := range insts {
		if r.isViable(i.Status) {
			viable++
		}
	}
	if viable != 0 {
		t.Fatalf("expected 0 viable instances due to DEGRADED pause, got %d", viable)
	}
}

func TestReconciler_UnknownTimeout(t *testing.T) {
	r, _, depReg, instReg, nodeReg, inst := setupReconciler()

	nodeReg.MarkOfflineNodes(-1 * time.Second)

	ConfigInstanceUnknownTimeout = 5 * time.Minute

	// Tick 1: marks UNKNOWN
	r.Reconcile()

	updated, _ := instReg.Get(inst.ID)
	if updated.Status != instance.StatusUnknown {
		t.Fatalf("expected UNKNOWN, got %v", updated.Status)
	}
	if updated.UnknownSince == nil {
		t.Fatalf("expected UnknownSince populated")
	}

	depBefore, _ := depReg.Get(inst.DeploymentID)

	// Tick 2: still UNKNOWN because timeout hasn't passed
	r.Reconcile()
	updated, _ = instReg.Get(inst.ID)
	if updated.Status != instance.StatusUnknown {
		t.Fatalf("expected still UNKNOWN, got %v", updated.Status)
	}

	// Fast forward time
	ConfigInstanceUnknownTimeout = 0
	r.Reconcile()

	updated, _ = instReg.Get(inst.ID)
	if updated.Status != instance.StatusCrashed {
		t.Fatalf("expected CRASHED, got %v", updated.Status)
	}

	depAfter, _ := depReg.Get(inst.DeploymentID)
	if depAfter.ConsecutiveCrashes != depBefore.ConsecutiveCrashes {
		t.Fatalf("node loss should NOT increment ConsecutiveCrashes")
	}
}

func TestReconciler_ObservationReturns(t *testing.T) {
	r, _, _, instReg, nodeReg, inst := setupReconciler()

	nodeReg.MarkOfflineNodes(-1 * time.Second)
	r.Reconcile() // UNKNOWN

	updated, _ := instReg.Get(inst.ID)
	if updated.UnknownSince == nil {
		t.Fatalf("expected UnknownSince")
	}

	// Observation returns
	nodeReg.Heartbeat("node-1", nil)
	instReg.ReportStatus(inst.ID, "node-1", inst.ExecutionID, instance.StatusRunning, instance.HealthHealthy, "cid", nil)

	updated, _ = instReg.Get(inst.ID)
	if updated.UnknownSince != nil {
		t.Fatalf("expected UnknownSince cleared, got %v", updated.UnknownSince)
	}
}
