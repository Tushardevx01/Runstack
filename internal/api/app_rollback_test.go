package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tushardevx01/runstack/internal/api"
	"github.com/Tushardevx01/runstack/internal/application"
	"github.com/Tushardevx01/runstack/internal/deployment"
	"github.com/Tushardevx01/runstack/internal/service"
)

func TestAppAPI_Rollback_IDOR(t *testing.T) {
	appReg := application.NewRegistry()
	depReg := deployment.NewRegistry()
	secReg := application.NewSecretRegistry()
	svc := service.NewAppService(appReg, depReg, secReg)

	// App A
	specA := application.AppSpec{Image: "image-a", Replicas: 1}
	appA, _ := svc.CreateApp("AppA", specA)
	appA, _ = svc.DeployApp(appA.ID, specA)

	// App B
	specB := application.AppSpec{Image: "image-b", Replicas: 1}
	appB, _ := svc.CreateApp("AppB", specB)
	appB, _ = svc.DeployApp(appB.ID, specB)
	depB := appB.ActiveDeploymentID

	handler := &api.AppHandler{Service: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/apps/{id}/rollback", handler.Rollback)

	req, _ := http.NewRequest("POST", "/api/v1/apps/"+appA.ID+"/rollback?target="+depB, nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	if !strings.Contains(rr.Body.String(), "forbidden") {
		t.Errorf("Expected 'forbidden' in error message")
	}

	// Verify no mutation occurred
	appAVerify, _ := appReg.Get(appA.ID)
	if appAVerify.ActiveDeploymentID == depB {
		t.Errorf("Rollback IDOR mutation occurred!")
	}
}
