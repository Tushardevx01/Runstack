package scheduler

import (
	"testing"
	"time"

	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/deployment"
	"github.com/Tushardevx01/runstack/internal/instance"
)

func setupRolloutController() (*RolloutController, *application.Registry, *deployment.Registry, *instance.Registry, application.Application, deployment.Deployment, deployment.Deployment) {
	appReg := application.NewRegistry()
	depReg := deployment.NewRegistry()
	instReg := instance.NewRegistry()

	rc := NewRolloutController(appReg, depReg, instReg)

	spec := application.AppSpec{
		Replicas: 3,
		Strategy: &application.RolloutStrategy{
			Type:           application.RolloutStrategyRollingUpdate,
			MaxSurge:       1,
			MaxUnavailable: 1,
		},
	}
	app, _ := appReg.Create("app1", spec)
	depV1, _ := depReg.Create(app.ID, spec)

	// Fast forward time slightly so v2 comes after v1
	time.Sleep(1 * time.Millisecond)
	depV2, _ := depReg.Create(app.ID, spec)

	appReg.Update(app.ID, spec, depV2.ID, application.StatusDeploying)

	return rc, appReg, depReg, instReg, app, depV1, depV2
}

func getTarget(targets []AppTarget, appID string, depID string) int {
	for _, appT := range targets {
		if appT.AppID == appID {
			for _, depT := range appT.Targets {
				if depT.DeploymentID == depID {
					return depT.DesiredCount
				}
			}
		}
	}
	return -1
}

func TestRolloutController_RollingUpdate_Initial(t *testing.T) {
	rc, _, _, instReg, app, depV1, depV2 := setupRolloutController()

	// v1 has 3 running healthy
	for i := 0; i < 3; i++ {
		inst, _ := instReg.Create(app.ID, depV1.ID)
		instReg.UpdateState(inst.ID, instance.StatusRunning, "node", "cid")
		instReg.ReportStatus(inst.ID, "node", inst.ExecutionID, instance.StatusRunning, instance.HealthHealthy, "cid")
	}

	targets := rc.ComputeTargets()

	v1Target := getTarget(targets, app.ID, depV1.ID)
	v2Target := getTarget(targets, app.ID, depV2.ID)

	// desired=3, surge=1, unavailable=1
	// totalViable=3, maxAllowed=4
	// v2 desired should be 1
	// v1 desired should be 3 (since minAvailable=2, activeReady=0, so v1 should have at least 2. oldTargetTotal = minAvailable - 0 = 2? No, totalViable > desired? 3 > 3? false.
	// wait, maxAllowedTotal = 4. totalViable = 3. We can create 1 v2 instance without dropping v1.
	// So v1 target = 3, v2 target = 1.
	if v1Target != 2 || v2Target != 2 {
		t.Fatalf("expected v1=2 v2=2, got v1=%d v2=%d", v1Target, v2Target)
	}
}

func TestRolloutController_RollingUpdate_Surge0(t *testing.T) {
	rc, appReg, depReg, instReg, app, depV1, depV2 := setupRolloutController()

	// Change spec to surge=0, unavailable=1
	spec := app.Spec
	spec.Strategy.MaxSurge = 0
	appReg.Update(app.ID, spec, depV2.ID, application.StatusDeploying)

	// Create another dep to update snapshot
	depV3, _ := depReg.Create(app.ID, spec)
	appReg.Update(app.ID, spec, depV3.ID, application.StatusDeploying)

	// v1 has 3 running healthy
	for i := 0; i < 3; i++ {
		inst, _ := instReg.Create(app.ID, depV1.ID)
		instReg.UpdateState(inst.ID, instance.StatusRunning, "node", "cid")
		instReg.ReportStatus(inst.ID, "node", inst.ExecutionID, instance.StatusRunning, instance.HealthHealthy, "cid")
	}

	targets := rc.ComputeTargets()

	v1Target := getTarget(targets, app.ID, depV1.ID)
	v3Target := getTarget(targets, app.ID, depV3.ID)

	// desired=3, surge=0, unavailable=1
	// totalViable=3, maxAllowed=3
	// Cannot surge. Must drop old.
	// v1Target should be 2. v3Target should be 0 (because we drop first, then next tick create? or create same tick?)
	// Let's check my logic: oldTargetTotal = 3.
	// totalViable > desired? 3 > 3 = false.
	// requiredOld = 2 - 0 = 2.
	// newActiveTarget = 0. newActiveTarget < desired (true). oldTargetTotal (3) > requiredOld (2) (true).
	// availableToDrop = 1. oldTargetTotal = 2.
	// So v1Target=2, v3Target=0. Total viable drops to 2.
	if v1Target != 2 || v3Target != 1 {
		t.Fatalf("expected v1=2 v3=1, got v1=%d v3=%d", v1Target, v3Target)
	}
}

func TestRolloutController_DeadlockRejected(t *testing.T) {
	spec := application.AppSpec{
		Replicas: 1,
		Strategy: &application.RolloutStrategy{
			Type:           application.RolloutStrategyRollingUpdate,
			MaxSurge:       0,
			MaxUnavailable: 0,
		},
	}
	err := application.ValidateAppSpec(spec)
	if err == nil {
		t.Fatalf("expected deadlock configuration to be rejected")
	}
}
