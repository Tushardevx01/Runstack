package executor_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Tushardevx01/runstack/internal/api"
	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/deployment"
	"github.com/Tushardevx01/runstack/internal/executor"
	"github.com/Tushardevx01/runstack/internal/instance"
	"github.com/Tushardevx01/runstack/internal/runtime"
	"github.com/Tushardevx01/runstack/internal/runtime/fake"
)

func TestInstanceExecutor_Lifecycle(t *testing.T) {
	instRegistry := instance.NewRegistry()
	depRegistry := deployment.NewRegistry()
	appRegistry := application.NewRegistry()

	// Setup data
	spec := application.AppSpec{Image: "test-image"}
	app, _ := appRegistry.Create("test-app", spec)
	dep, _ := depRegistry.Create(app.ID, spec)
	inst, _ := instRegistry.Create(app.ID, dep.ID)
	instRegistry.UpdateState(inst.ID, instance.StatusAssigned, "node-1", "")

	// Setup API
	handler := &api.InstanceHandler{
		InstanceRegistry:   instRegistry,
		DeploymentRegistry: depRegistry,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/instances", handler.List)
	mux.HandleFunc("POST /api/v1/instances/{id}/claim", handler.Claim)
	mux.HandleFunc("POST /api/v1/instances/{id}/status", handler.ReportStatus)

	server := httptest.NewServer(mux)
	defer server.Close()

	client := api.NewClient(server.URL)
	fakeRuntime := fake.New()

	exec := executor.NewInstanceExecutor("node-1", client, fakeRuntime)
	exec.Start()
	defer exec.Stop()

	// Wait for agent to claim and start
	time.Sleep(200 * time.Millisecond)

	updatedInst, _ := instRegistry.Get(inst.ID)
	if updatedInst.Status != instance.StatusRunning {
		t.Errorf("expected RUNNING, got %s", updatedInst.Status)
	}
	if updatedInst.ExecutionID == "" {
		t.Error("expected ExecutionID to be set")
	}

	// Fake runtime stop to simulate crash
	fakeRuntime.Stop(context.Background(), "runstack-"+inst.ID)

	// Wait for agent to detect and report
	time.Sleep(200 * time.Millisecond)

	crashedInst, _ := instRegistry.Get(inst.ID)
	if crashedInst.Status != instance.StatusCrashed {
		t.Errorf("expected CRASHED, got %s", crashedInst.Status)
	}

	// Re-assign a new instance, scale down (STOPPING)
	inst2, _ := instRegistry.Create(app.ID, dep.ID)
	instRegistry.UpdateState(inst2.ID, instance.StatusAssigned, "node-1", "")

	time.Sleep(200 * time.Millisecond)
	updatedInst2, _ := instRegistry.Get(inst2.ID)
	if updatedInst2.Status != instance.StatusRunning {
		t.Errorf("expected RUNNING, got %s", updatedInst2.Status)
	}

	// Trigger scale down
	instRegistry.UpdateState(inst2.ID, instance.StatusStopping, "node-1", "")

	time.Sleep(200 * time.Millisecond)
	stoppedInst2, _ := instRegistry.Get(inst2.ID)
	if stoppedInst2.Status != instance.StatusStopped {
		t.Errorf("expected STOPPED, got %s", stoppedInst2.Status)
	}

	state, err := fakeRuntime.Status(context.Background(), "runstack-"+inst2.ID)
	if err == nil && state != runtime.StateExited { // fake rm just deletes it, so error expected
		t.Errorf("expected fake container to be removed or exited")
	}
}
