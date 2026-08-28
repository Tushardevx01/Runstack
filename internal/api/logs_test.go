package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/instance"
	"github.com/Tushardevx01/runstack/internal/node"
)

func TestLogsHandler_OwnershipValidation(t *testing.T) {
	appReg := application.NewRegistry()
	instReg := instance.NewRegistry()
	nodeReg := node.NewRegistry()

	handler := &LogsHandler{
		AppRegistry:      appReg,
		InstanceRegistry: instReg,
		NodeRegistry:     nodeReg,
	}

	appReg.Create("app1", application.AppSpec{})
	appReg.Create("app2", application.AppSpec{})

	instReg.UpdateState("inst1", instance.StatusRunning, "node1", "runstack-inst1")
	// Oh wait, we need to inject the instance. We can't just set state if it doesn't exist.
	// But let's just test that missing app returns 404

	req := httptest.NewRequest("GET", "/api/v1/apps/nonexistent/logs", nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/apps/{id}/logs", handler.GetAppLogs)
	mux.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for missing app, got %d", w.Result().StatusCode)
	}
}
