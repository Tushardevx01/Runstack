package executor_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tushardevx01/runstack/internal/api"
	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/deployment"
	"github.com/Tushardevx01/runstack/internal/executor"
	"github.com/Tushardevx01/runstack/internal/instance"
	"github.com/Tushardevx01/runstack/internal/runtime/fake"
)

func TestProber_Integration(t *testing.T) {
	// Setup Control Plane internals
	instRegistry := instance.NewRegistry()
	depRegistry := deployment.NewRegistry()
	appRegistry := application.NewRegistry()

	// Setup API server
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

	// Start Agent
	exec := executor.NewInstanceExecutor("node-1", client, fakeRuntime)
	exec.Start()
	defer exec.Stop()

	// Start real test target server on random port
	var currentStatus atomic.Int32
	currentStatus.Store(200)
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(int(currentStatus.Load()))
	}))
	defer targetServer.Close()
	targetPort, _ := strconv.Atoi(strings.TrimPrefix(targetServer.URL, "http://127.0.0.1:"))

	// Helper to create and claim instance
	createTestInstance := func(spec application.AppSpec) instance.Instance {
		app, _ := appRegistry.Create("test-app", spec)
		dep, _ := depRegistry.Create(app.ID, spec)
		inst, _ := instRegistry.Create(app.ID, dep.ID)
		instRegistry.UpdateState(inst.ID, instance.StatusAssigned, "node-1", "")
		return inst
	}

	waitForHealth := func(instID string, expectedHealth instance.InstanceHealth, timeout time.Duration) instance.Instance {
		start := time.Now()
		for time.Since(start) < timeout {
			inst, _ := instRegistry.Get(instID)
			if inst.Health == expectedHealth {
				return inst
			}
			time.Sleep(10 * time.Millisecond)
		}
		last, _ := instRegistry.Get(instID)
		t.Fatalf("timeout waiting for health %s, got %s", expectedHealth, last.Health)
		return instance.Instance{}
	}

	waitForStatus := func(instID string, expectedStatus instance.InstanceStatus, timeout time.Duration) instance.Instance {
		start := time.Now()
		for time.Since(start) < timeout {
			inst, _ := instRegistry.Get(instID)
			if inst.Status == expectedStatus {
				return inst
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("timeout waiting for status %s", expectedStatus)
		return instance.Instance{}
	}

	t.Run("TEST A: Configured readiness probe reaches HEALTHY", func(t *testing.T) {
		currentStatus.Store(200)
		spec := application.AppSpec{
			Image: "test-img",
			Ports: []application.PortMapping{{ContainerPort: 8080, HostPort: targetPort, Protocol: "tcp"}},
			ReadinessProbe: &application.Probe{
				Type: "HTTP", Path: "/", PeriodSecs: 1, TimeoutSecs: 1, SuccessThreshold: 1, FailureThreshold: 1,
			},
		}
		inst := createTestInstance(spec)
		waitForHealth(inst.ID, instance.HealthHealthy, 3*time.Second)
	})

	t.Run("TEST B: Readiness failure removes endpoint", func(t *testing.T) {
		currentStatus.Store(200)
		spec := application.AppSpec{
			Image: "test-img",
			Ports: []application.PortMapping{{ContainerPort: 8080, HostPort: targetPort, Protocol: "tcp"}},
			ReadinessProbe: &application.Probe{
				Type: "HTTP", Path: "/", PeriodSecs: 1, TimeoutSecs: 1, SuccessThreshold: 1, FailureThreshold: 1,
			},
		}
		inst := createTestInstance(spec)
		waitForHealth(inst.ID, instance.HealthHealthy, 3*time.Second)

		// Cause failure
		currentStatus.Store(500)
		waitForHealth(inst.ID, instance.HealthUnhealthy, 3*time.Second)

		// Check consecutive crashes are unchanged
		dep, _ := depRegistry.Get(inst.DeploymentID)
		if dep.ConsecutiveCrashes != 0 {
			t.Fatalf("Readiness failure should not increment ConsecutiveCrashes")
		}

		// Verify container still running
		finalInst, _ := instRegistry.Get(inst.ID)
		if finalInst.Status != instance.StatusRunning {
			t.Fatalf("Readiness failure should not terminate container")
		}
	})

	t.Run("TEST C: Readiness recovery", func(t *testing.T) {
		currentStatus.Store(500)
		spec := application.AppSpec{
			Image: "test-img",
			Ports: []application.PortMapping{{ContainerPort: 8080, HostPort: targetPort, Protocol: "tcp"}},
			ReadinessProbe: &application.Probe{
				Type: "HTTP", Path: "/", PeriodSecs: 1, TimeoutSecs: 1, SuccessThreshold: 1, FailureThreshold: 1,
			},
		}
		inst := createTestInstance(spec)
		waitForHealth(inst.ID, instance.HealthUnhealthy, 3*time.Second)

		// Recover
		currentStatus.Store(200)
		waitForHealth(inst.ID, instance.HealthHealthy, 3*time.Second)
	})

	t.Run("TEST D: Liveness failure terminates container", func(t *testing.T) {
		currentStatus.Store(200)
		spec := application.AppSpec{
			Image: "test-img",
			Ports: []application.PortMapping{{ContainerPort: 8080, HostPort: targetPort, Protocol: "tcp"}},
			LivenessProbe: &application.Probe{
				Type: "HTTP", Path: "/", PeriodSecs: 1, TimeoutSecs: 1, SuccessThreshold: 1, FailureThreshold: 1,
			},
		}
		inst := createTestInstance(spec)

		// Wait for running
		waitForStatus(inst.ID, instance.StatusRunning, 3*time.Second)

		// Cause failure
		currentStatus.Store(500)

		// Status should become CRASHED
		waitForStatus(inst.ID, instance.StatusCrashed, 3*time.Second)

		// Check consecutive crashes incremented
		dep, _ := depRegistry.Get(inst.DeploymentID)
		if dep.ConsecutiveCrashes == 0 {
			t.Fatalf("Liveness failure MUST increment ConsecutiveCrashes")
		}
	})

	t.Run("TEST E: Verify readiness failure alone NEVER terminates container", func(t *testing.T) {
		currentStatus.Store(500)
		spec := application.AppSpec{
			Image: "test-img",
			Ports: []application.PortMapping{{ContainerPort: 8080, HostPort: targetPort, Protocol: "tcp"}},
			ReadinessProbe: &application.Probe{
				Type: "HTTP", Path: "/", PeriodSecs: 1, TimeoutSecs: 1, SuccessThreshold: 1, FailureThreshold: 1,
			},
		}
		inst := createTestInstance(spec)

		// Wait for running and unhealthy
		waitForHealth(inst.ID, instance.HealthUnhealthy, 3*time.Second)

		time.Sleep(1 * time.Second)
		finalInst, _ := instRegistry.Get(inst.ID)
		if finalInst.Status != instance.StatusRunning {
			t.Fatalf("Readiness failure terminated container")
		}
	})
}
