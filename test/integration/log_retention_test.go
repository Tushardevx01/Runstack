package integration

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/Tushardevx01/runstack/internal/api"
	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/executor"
	"github.com/Tushardevx01/runstack/internal/instance"
	"github.com/Tushardevx01/runstack/internal/node"
	"github.com/Tushardevx01/runstack/internal/runtime"
)

type mockRuntimeLog struct {
	runtime.ContainerRuntime
	logs map[string]string
}

func (m *mockRuntimeLog) Logs(ctx context.Context, containerID string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(m.logs[containerID])), nil
}

func TestCrashLogRetentionEndToEnd(t *testing.T) {
	appRegistry := application.NewRegistry()
	instRegistry := instance.NewRegistry()
	nodeRegistry := node.NewRegistry()

	nodeRegistry.Register(node.Node{ID: "node-1", IPAddress: "127.0.0.1"}, "tok")

	logsHandler := &api.LogsHandler{
		AppRegistry:      appRegistry,
		InstanceRegistry: instRegistry,
		NodeRegistry:     nodeRegistry,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/apps/{id}/logs", logsHandler.GetAppLogs)

	cpServer := httptest.NewServer(mux)
	defer cpServer.Close()
	cpClient := api.NewClient(cpServer.URL)

	cr := &mockRuntimeLog{logs: make(map[string]string)}
	exec := executor.NewInstanceExecutor("node-1", cpClient, cr)

	agentMux := http.NewServeMux()
	agentMux.HandleFunc("/api/v1/logs", func(w http.ResponseWriter, r *http.Request) {
		instID := r.URL.Query().Get("instance_id")
		appID := r.URL.Query().Get("app_id")
		execID := r.URL.Query().Get("exec_id")
		if instID != "" && appID != "" {
			lines, ok := exec.CrashLogs.Get(instID, appID, execID)
			if ok {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(strings.Join(lines, "\n") + "\n"))
				return
			}
		}
		http.Error(w, "Not found", 404)
	})

	agentServer := httptest.NewServer(agentMux)
	defer agentServer.Close()

	u, _ := url.Parse(agentServer.URL)
	portStr := u.Port()
	port, _ := strconv.Atoi(portStr)
	logsHandler.AgentPort = port

	app, _ := appRegistry.Create("my-app", application.AppSpec{})
	inst, _ := instRegistry.Create(app.ID, "dep-1")
	instRegistry.UpdateState(inst.ID, instance.StatusAssigned, "node-1", "")
	inst, _ = instRegistry.Claim(inst.ID, "node-1")
	inst, _ = instRegistry.ReportStatus(inst.ID, "node-1", inst.ExecutionID, instance.StatusRunning, instance.HealthHealthy, "runstack-"+inst.ID, nil)

	cr.logs["runstack-"+inst.ID] = "started\nprocessing\ncrashed"
	exec.CrashLogs.CaptureAndFreeze(context.Background(), app.ID, "dep-1", inst.ID, inst.ExecutionID, "node-1", "runstack-"+inst.ID)

	instRegistry.ReportStatus(inst.ID, "node-1", inst.ExecutionID, instance.StatusCrashed, instance.HealthUnknown, "runstack-"+inst.ID, nil)

	inst2, _ := instRegistry.Create(app.ID, "dep-1")
	inst2, _ = instRegistry.Claim(inst2.ID, "node-1")
	instRegistry.ReportStatus(inst2.ID, "node-1", inst2.ExecutionID, instance.StatusRunning, instance.HealthHealthy, "runstack-"+inst2.ID, nil)

	req, _ := http.NewRequest("GET", cpServer.URL+"/api/v1/apps/"+app.ID+"/logs?instance="+inst.ID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to request logs: %v", err)
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(b), "crashed") {
		t.Errorf("Expected logs to contain 'crashed', got: %s", string(b))
	}
}
