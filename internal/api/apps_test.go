package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/deployment"
	"github.com/Tushardevx01/runstack/internal/ingress"
	"github.com/Tushardevx01/runstack/internal/instance"
)

func setupAppsHandler() *AppsHandler {
	appReg := application.NewRegistry()
	depReg := deployment.NewRegistry()
	instReg := instance.NewRegistry()
	domainReg := ingress.NewDomainRegistry()
	ingressReg := ingress.NewIngressRegistry()

	return &AppsHandler{
		AppRegistry:      appReg,
		DepRegistry:      depReg,
		InstanceRegistry: instReg,
		DomainRegistry:   domainReg,
		IngressRegistry:  ingressReg,
	}
}

func TestListApps(t *testing.T) {
	h := setupAppsHandler()

	h.AppRegistry.Create("test-app", application.AppSpec{Replicas: 3})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	rr := httptest.NewRecorder()

	h.ListApps(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var res map[string][]AppSummary
	if err := json.NewDecoder(rr.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	apps := res["applications"]
	if len(apps) != 1 {
		t.Fatalf("expected 1 app, got %d", len(apps))
	}
	if apps[0].Name != "test-app" {
		t.Errorf("expected test-app, got %s", apps[0].Name)
	}
	if apps[0].DesiredReplicas != 3 {
		t.Errorf("expected 3 replicas, got %d", apps[0].DesiredReplicas)
	}
}

func TestGetAppStatus(t *testing.T) {
	h := setupAppsHandler()

	app, _ := h.AppRegistry.Create("test-app", application.AppSpec{Replicas: 3})
	dep, _ := h.DepRegistry.Create(app.ID, app.Spec)

	// Simulate assignment
	h.AppRegistry.Update(app.ID, app.Spec, dep.ID, application.StatusReady)
	h.DepRegistry.UpdateRolloutStatus(dep.ID, deployment.RolloutCompleted, 3, 3, 3, 0, "")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/apps/{name}/status", h.GetAppStatus)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/test-app/status", nil)
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var detail AppStatusDetail
	if err := json.NewDecoder(rr.Body).Decode(&detail); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if detail.Application.Name != "test-app" {
		t.Errorf("expected test-app, got %s", detail.Application.Name)
	}
	if detail.ActiveDeployment == nil || detail.ActiveDeployment.RolloutStatus != deployment.RolloutCompleted {
		t.Errorf("expected rollout completed")
	}
}
