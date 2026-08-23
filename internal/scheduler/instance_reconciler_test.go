package scheduler

import (
	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/deployment"
	"github.com/Tushardevx01/runstack/internal/instance"
	"github.com/Tushardevx01/runstack/internal/node"
	"testing"
)

func TestInstanceReconciler_IdempotentScaleUp(t *testing.T) {
	appReg := application.NewRegistry()
	depReg := deployment.NewRegistry()
	instReg := instance.NewRegistry()
	nodeReg := node.NewRegistry()

	r := NewInstanceReconciler(appReg, depReg, instReg, nodeReg)

	// Create an app with replicas=2
	spec := application.AppSpec{Replicas: 2}
	app, _ := appReg.Create("app1", spec)
	dep, _ := depReg.Create(app.ID, spec)
	appReg.Update(app.ID, spec, dep.ID, application.StatusPending)

	// Tick 1
	r.Reconcile()
	insts := instReg.List()
	if len(insts) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(insts))
	}

	// Tick 2 (Idempotency check)
	r.Reconcile()
	insts = instReg.List()
	if len(insts) != 2 {
		t.Fatalf("expected 2 instances after second tick, got %d", len(insts))
	}
}

func TestInstanceReconciler_OrphanRecovery(t *testing.T) {
	appReg := application.NewRegistry()
	depReg := deployment.NewRegistry()
	instReg := instance.NewRegistry()
	nodeReg := node.NewRegistry()

	r := NewInstanceReconciler(appReg, depReg, instReg, nodeReg)

	spec := application.AppSpec{Replicas: 1}
	app, _ := appReg.Create("app1", spec)
	dep, _ := depReg.Create(app.ID, spec)
	appReg.Update(app.ID, spec, dep.ID, application.StatusPending)

	// Create an instance manually and assign it to an offline node
	inst, _ := instReg.Create(app.ID, dep.ID)
	instReg.UpdateState(inst.ID, instance.StatusRunning, "node-1", "cid-1")

	// node-1 is not registered, so it is implicitly offline
	ConfigInstanceUnknownTimeout = 0
	r.Reconcile() // marks UNKNOWN
	r.Reconcile() // times out to CRASHED, scales up

	// Original instance should be CRASHED
	updatedInst, _ := instReg.Get(inst.ID)
	if updatedInst.Status != instance.StatusCrashed {
		t.Fatalf("expected original instance to be CRASHED, got %s", updatedInst.Status)
	}

	// But a replacement should have been created (total instances = 2)
	insts := instReg.List()
	if len(insts) != 2 {
		t.Fatalf("expected 2 instances (1 crashed, 1 pending), got %d", len(insts))
	}

	viable := 0
	for _, i := range insts {
		if r.isViable(i.Status) {
			viable++
		}
	}
	if viable != 1 {
		t.Fatalf("expected 1 viable instance, got %d", viable)
	}

	// Tick 2 (idempotency check)
	r.Reconcile()
	insts = instReg.List()
	if len(insts) != 2 {
		t.Fatalf("expected instances to remain 2, got %d", len(insts))
	}
}

func TestInstanceReconciler_ScaleDown(t *testing.T) {
	appReg := application.NewRegistry()
	depReg := deployment.NewRegistry()
	instReg := instance.NewRegistry()
	nodeReg := node.NewRegistry()

	r := NewInstanceReconciler(appReg, depReg, instReg, nodeReg)

	spec := application.AppSpec{Replicas: 1}
	app, _ := appReg.Create("app1", spec)
	dep, _ := depReg.Create(app.ID, spec)
	appReg.Update(app.ID, spec, dep.ID, application.StatusPending)

	// Manually create 3 viable instances
	instReg.Create(app.ID, dep.ID)
	instReg.Create(app.ID, dep.ID)
	instReg.Create(app.ID, dep.ID)

	r.Reconcile()

	insts := instReg.List()
	if len(insts) != 3 {
		t.Fatalf("expected 3 total instances, got %d", len(insts))
	}

	viable := 0
	for _, i := range insts {
		if r.isViable(i.Status) {
			viable++
		}
	}
	if viable != 1 {
		t.Fatalf("expected 1 viable instance after scale down, got %d", viable)
	}
}

func TestInstanceReconciler_DeploymentRollout(t *testing.T) {
	appReg := application.NewRegistry()
	depReg := deployment.NewRegistry()
	instReg := instance.NewRegistry()
	nodeReg := node.NewRegistry()

	r := NewInstanceReconciler(appReg, depReg, instReg, nodeReg)

	spec1 := application.AppSpec{Replicas: 1}
	app, _ := appReg.Create("app1", spec1)
	dep1, _ := depReg.Create(app.ID, spec1)
	appReg.Update(app.ID, spec1, dep1.ID, application.StatusPending)

	// Tick 1 creates an instance for dep1
	r.Reconcile()

	// Update Application to spec2 (dep2)
	spec2 := application.AppSpec{Replicas: 1}
	dep2, _ := depReg.Create(app.ID, spec2)
	appReg.Update(app.ID, spec2, dep2.ID, application.StatusPending)

	// Tick 2 should stop dep1 instance and create dep2 instance
	r.Reconcile()

	insts := instReg.List()
	if len(insts) != 2 {
		t.Fatalf("expected 2 total instances, got %d", len(insts))
	}

	for _, i := range insts {
		if i.DeploymentID == dep1.ID && i.Status != instance.StatusStopped {
			t.Fatalf("expected old deployment instance to be STOPPED, got %s", i.Status)
		}
		if i.DeploymentID == dep2.ID && i.Status != instance.StatusPending {
			t.Fatalf("expected new deployment instance to be PENDING, got %s", i.Status)
		}
	}
}
